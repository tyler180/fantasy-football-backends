package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	// shared code in your repo
	"github.com/tyler180/fantasy-football-backends/internal/pfr"
	"github.com/tyler180/fantasy-football-backends/internal/store"
)

// ==== Event & env helpers ====

type Event struct {
	Mode           string `json:"mode"`             // "ingest_snaps_by_game" | "materialize_snap_trends"
	Season         string `json:"season"`           // optional override
	TeamChunkTotal *int   `json:"team_chunk_total"` // optional chunking
	TeamChunkIndex *int   `json:"team_chunk_index"` // optional chunking
	TeamList       string `json:"team_list"`        // optional CSV of team abbrs, e.g. "SEA,TAM,TEN"
}

func envStr(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}
func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return def
	}
}

func applyEventOverrides(e Event) {
	if e.Season != "" {
		os.Setenv("SEASON", e.Season)
	}
	if e.TeamChunkTotal != nil {
		os.Setenv("TEAM_CHUNK_TOTAL", strconv.Itoa(*e.TeamChunkTotal))
	}
	if e.TeamChunkIndex != nil {
		os.Setenv("TEAM_CHUNK_INDEX", strconv.Itoa(*e.TeamChunkIndex))
	}
	if strings.TrimSpace(e.TeamList) != "" {
		os.Setenv("TEAM_LIST", e.TeamList)
	}
}

// ==== Team subset / chunking ====

func teamSubset(all []pfr.Team, teamListCSV string, chunkTotal, chunkIndex int, shuffle bool) []pfr.Team {
	// explicit list wins
	if strings.TrimSpace(teamListCSV) != "" {
		want := map[string]struct{}{}
		for _, t := range strings.Split(teamListCSV, ",") {
			want[strings.ToUpper(strings.TrimSpace(t))] = struct{}{}
		}
		out := make([]pfr.Team, 0, len(want))
		for _, tm := range all {
			if _, ok := want[tm.Abbr]; ok {
				out = append(out, tm)
			}
		}
		return out
	}

	cp := append([]pfr.Team(nil), all...)
	// stable order unless shuffle=true
	if shuffle {
		rand.Seed(time.Now().UnixNano())
		rand.Shuffle(len(cp), func(i, j int) { cp[i], cp[j] = cp[j], cp[i] })
	}

	if chunkTotal <= 1 {
		return cp
	}
	if chunkIndex < 0 || chunkIndex >= chunkTotal {
		return cp
	}
	// split into chunkTotal roughly-even chunks, return chunkIndex
	n := len(cp)
	chunkSize := (n + chunkTotal - 1) / chunkTotal // ceil
	start := chunkIndex * chunkSize
	if start >= n {
		return nil
	}
	end := start + chunkSize
	if end > n {
		end = n
	}
	return cp[start:end]
}

func teamDelay() time.Duration {
	ms := envInt("TEAM_DELAY_MS", 500)
	jitter := rand.Intn(ms / 3) // ~30% jitter
	return time.Duration(ms+jitter) * time.Millisecond
}

// ==== DDB helpers (read-side; write/update use store package) ====

type playerKey struct {
	PlayerID string
	Player   string
}

func listPlayersForSeasonTeam(ctx context.Context, ddb *dynamodb.Client, playersTable, season, team string) ([]playerKey, error) {
	st := season + "#" + team
	out, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(playersTable),
		KeyConditionExpression: aws.String("#pk = :v"),
		ExpressionAttributeNames: map[string]string{
			"#pk": "SeasonTeam",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":v": &ddbtypes.AttributeValueMemberS{Value: st},
		},
	})
	if err != nil {
		return nil, err
	}
	res := make([]playerKey, 0, len(out.Items))
	for _, it := range out.Items {
		pid := getStr(it, "PlayerID")
		nm := getStr(it, "Player")
		if pid != "" {
			res = append(res, playerKey{PlayerID: pid, Player: nm})
		}
	}
	return res, nil
}

func queryPlayerSnaps(ctx context.Context, ddb *dynamodb.Client, snapTable, playerID, season string) ([]float64, error) {
	// GSI PlayerGames: PK=PlayerID, SK=SeasonWeek ("2024#01", ...)
	prefix := season + "#"
	out, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(snapTable),
		IndexName:              aws.String("PlayerGames"),
		KeyConditionExpression: aws.String("#pid = :pid AND begins_with(#sw, :pref)"),
		ExpressionAttributeNames: map[string]string{
			"#pid": "PlayerID",
			"#sw":  "SeasonWeek",
		},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":pid":  &ddbtypes.AttributeValueMemberS{Value: playerID},
			":pref": &ddbtypes.AttributeValueMemberS{Value: prefix},
		},
	})
	if err != nil {
		return nil, err
	}

	// Ensure sorted by SeasonWeek ascending (Query should already do this)
	type row struct {
		sw  string
		pct float64
	}
	tmp := make([]row, 0, len(out.Items))
	for _, it := range out.Items {
		sw := getStr(it, "SeasonWeek")
		pct := getFloat(it, "DefSnapPct")
		tmp = append(tmp, row{sw: sw, pct: pct})
	}
	sort.Slice(tmp, func(i, j int) bool { return tmp[i].sw < tmp[j].sw })

	vals := make([]float64, 0, len(tmp))
	for _, r := range tmp {
		vals = append(vals, r.pct)
	}
	return vals, nil
}

func getStr(m map[string]ddbtypes.AttributeValue, key string) string {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case *ddbtypes.AttributeValueMemberS:
			return t.Value
		case *ddbtypes.AttributeValueMemberN:
			return t.Value
		}
	}
	return ""
}
func getFloat(m map[string]ddbtypes.AttributeValue, key string) float64 {
	s := getStr(m, key)
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ==== Trend math ====

func slope(vals []float64) float64 {
	n := float64(len(vals))
	if n < 2 {
		return 0
	}
	var sx, sy, sxx, sxy float64
	for i, y := range vals {
		x := float64(i + 1) // 1..n
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den // pct points per game
}

// ==== Handler ====

func handler(ctx context.Context, raw json.RawMessage) (string, error) {
	// Parse event & allow overrides
	var e Event
	_ = json.Unmarshal(raw, &e)
	applyEventOverrides(e)

	season := envStr("SEASON", "2024")
	mode := strings.TrimSpace(e.Mode)
	if mode == "" {
		mode = envStr("MODE", "ingest_snaps_by_game")
	}
	debug := envBool("DEBUG", false)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("aws config: %w", err)
	}
	ddb := dynamodb.NewFromConfig(cfg)

	switch mode {

	case "ingest_snaps_by_game":
		snapTable := envStr("SNAP_TABLE_NAME", "defensive_snaps_by_game")
		// teams list from internal package
		allTeams := pfr.AllTeams() // []pfr.Team{ Abbr: "SEA", Path: "sea", ...}
		chunkTotal := envInt("TEAM_CHUNK_TOTAL", 0)
		chunkIndex := envInt("TEAM_CHUNK_INDEX", 0)
		subset := teamSubset(allTeams, envStr("TEAM_LIST", e.TeamList), pickInt(e.TeamChunkTotal, chunkTotal), pickInt(e.TeamChunkIndex, chunkIndex), envBool("SHUFFLE_TEAMS", true))

		if debug {
			abbrs := make([]string, len(subset))
			for i, t := range subset {
				abbrs[i] = t.Abbr
			}
			log.Printf("snaps: season=%s teams=%d: %s", season, len(subset), strings.Join(abbrs, ","))
		}

		referer := fmt.Sprintf("https://www.pro-football-reference.com/years/%s/", season)
		total := 0
		for _, t := range subset {
			rows, err := pfr.FetchTeamDefSnapPctsByGame(ctx, t.Path, t.Abbr, season, referer)
			if err != nil {
				if debug {
					log.Printf("snaps: %s failed: %v", t.Abbr, err)
				}
				time.Sleep(teamDelay())
				continue
			}
			if len(rows) > 0 {
				if err := store.PutSnapGameRows(ctx, ddb, snapTable, rows); err != nil {
					return "", fmt.Errorf("write snap rows: %w", err)
				}
				total += len(rows)
			}
			time.Sleep(teamDelay())
		}
		log.Printf("OK snaps: wrote %d rows to %s for %s", total, snapTable, season)
		return fmt.Sprintf("snaps=%d", total), nil

	case "materialize_snap_trends":
		snapTable := envStr("SNAP_TABLE_NAME", "defensive_snaps_by_game")
		playersTable := envStr("TABLE_NAME", "defensive_players_by_team")

		allTeams := pfr.AllTeams()
		updated := 0
		for _, t := range allTeams {
			players, err := listPlayersForSeasonTeam(ctx, ddb, playersTable, season, t.Abbr)
			if err != nil {
				if debug {
					log.Printf("trends: list players %s error: %v", t.Abbr, err)
				}
				continue
			}
			if len(players) == 0 {
				continue
			}
			for _, pk := range players {
				vals, err := queryPlayerSnaps(ctx, ddb, snapTable, pk.PlayerID, season)
				if err != nil {
					if debug {
						log.Printf("trends: query snaps %s %s error: %v", t.Abbr, pk.PlayerID, err)
					}
					continue
				}
				if len(vals) == 0 {
					// clear/update with zeros so we always have the attribute?
					_ = store.UpdatePlayerTrends(ctx, ddb, playersTable, season, t.Abbr, pk.PlayerID, 0, 0, 0, 0)
					continue
				}
				last := vals[len(vals)-1]
				var s3, s5, c3 float64
				if len(vals) >= 3 {
					s3 = slope(vals[len(vals)-3:])
					if len(vals) >= 2 {
						base := 0.0
						for _, v := range vals[len(vals)-3 : len(vals)-1] {
							base += v
						}
						base /= 2.0
						c3 = last - base
					}
				}
				if len(vals) >= 5 {
					s5 = slope(vals[len(vals)-5:])
				}
				if err := store.UpdatePlayerTrends(ctx, ddb, playersTable, season, t.Abbr, pk.PlayerID, last, s3, s5, c3); err != nil {
					if debug {
						log.Printf("trends: update %s %s error: %v", t.Abbr, pk.PlayerID, err)
					}
					continue
				}
				updated++
			}
			// light pacing between teams
			time.Sleep(150 * time.Millisecond)
		}
		log.Printf("OK trends: updated %d player summaries in %s for %s", updated, playersTable, season)
		return fmt.Sprintf("trends_updated=%d", updated), nil

	default:
		return "", fmt.Errorf("unknown mode %q", mode)
	}
}

func pickInt(ev *int, env int) int {
	if ev != nil {
		return *ev
	}
	return env
}

func main() {
	// More deterministic logging timestamps in CloudWatch
	log.SetFlags(0)
	lambda.Start(handler)
}
