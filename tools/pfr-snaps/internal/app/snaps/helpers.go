package snaps

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	// update these to your module path
	"github.com/tyler180/fantasy-football-backends/internal/pfr"
)

// ------------------ env helpers ------------------

func envStr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}
func envBool(k string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(k))) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return def
	}
}
func envInt(k string, def int) int {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}
func pickInt(ev *int, env int) int {
	if ev != nil {
		return *ev
	}
	return env
}

// IMPORTANT: no os.Setenv on TEAM_LIST; warm lambdas would “stick” to last value.

// ------------------ team code mapping ------------------
// NFLverse <-> PFR

var nflverseToPFR = map[string]string{
	"ARI": "ARI", "ATL": "ATL", "BAL": "BAL", "BUF": "BUF", "CAR": "CAR",
	"CHI": "CHI", "CIN": "CIN", "CLE": "CLE", "DAL": "DAL", "DEN": "DEN",
	"DET": "DET", "GB": "GNB", "HOU": "HOU", "IND": "CLT", "JAX": "JAX",
	"KC": "KAN", "LV": "LVR", "LAC": "LAC", "LAR": "LAR", "MIA": "MIA",
	"MIN": "MIN", "NE": "NWE", "NO": "NOR", "NYG": "NYG", "NYJ": "NYJ",
	"PHI": "PHI", "PIT": "PIT", "SF": "SFO", "SEA": "SEA", "TB": "TAM",
	"TEN": "TEN", "WAS": "WAS",
}
var pfrToNFLverse = map[string]string{
	"ARI": "ARI", "ATL": "ATL", "BAL": "BAL", "BUF": "BUF", "CAR": "CAR",
	"CHI": "CHI", "CIN": "CIN", "CLE": "CLE", "DAL": "DAL", "DEN": "DEN",
	"DET": "DET", "GNB": "GB", "HOU": "HOU", "CLT": "IND", "JAX": "JAX",
	"KAN": "KC", "LVR": "LV", "LAC": "LAC", "LAR": "LAR", "MIA": "MIA",
	"MIN": "MIN", "NWE": "NE", "NOR": "NO", "NYG": "NYG", "NYJ": "NYJ",
	"PHI": "PHI", "PIT": "PIT", "SFO": "SF", "SEA": "SEA", "TAM": "TB",
	"TEN": "TEN", "WAS": "WAS",
}

func buildNFLverseFilter(teamListCSV string) map[string]struct{} {
	teamListCSV = strings.TrimSpace(teamListCSV)
	if teamListCSV == "" {
		return nil // nil => ALL teams
	}
	set := make(map[string]struct{})
	for _, t := range strings.Split(teamListCSV, ",") {
		tu := strings.ToUpper(strings.TrimSpace(t))
		if tu == "" {
			continue
		}
		if nv, ok := pfrToNFLverse[tu]; ok {
			set[nv] = struct{}{}
			continue
		}
		set[tu] = struct{}{}
	}
	return set
}

func logTeamFilter(debug bool, season string, filter map[string]struct{}) {
	if !debug {
		return
	}
	if len(filter) == 0 {
		fmt.Printf("snaps[nflverse]: season=%s filter=ALL\n", season)
		return
	}
	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("snaps[nflverse]: season=%s filter=%v\n", season, keys)
}

// ------------------ positions ------------------

func buildPosSet(csv string) map[string]struct{} {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	m := make(map[string]struct{})
	for _, p := range strings.Split(csv, ",") {
		pp := strings.ToUpper(strings.TrimSpace(p))
		if pp != "" {
			m[pp] = struct{}{}
		}
	}
	return m
}
func isDefensive(pos string, allowed map[string]struct{}) bool {
	if pos == "" || allowed == nil {
		return false
	}
	_, ok := allowed[pos]
	return ok
}

// ------------------ team subset (PFR fallback) ------------------

func teamSubset(all []pfr.Team, teamListCSV string, chunkTotal, chunkIndex int) []pfr.Team {
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
	if chunkTotal <= 1 {
		return cp
	}
	if chunkIndex < 0 || chunkIndex >= chunkTotal {
		return cp
	}
	n := len(cp)
	chunkSize := (n + chunkTotal - 1) / chunkTotal
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
	return time.Duration(envInt("TEAM_DELAY_MS", 500)) * time.Millisecond
}

// ------------------ trends helpers (DDB read + slope) ------------------

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
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v": &types.AttributeValueMemberS{Value: st},
		},
		ProjectionExpression: aws.String("PlayerID, Player"),
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
	prefix := season + "#"
	out, err := ddb.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(snapTable),
		IndexName:              aws.String("PlayerGames"),
		KeyConditionExpression: aws.String("#pid = :pid AND begins_with(#sw, :pref)"),
		ExpressionAttributeNames: map[string]string{
			"#pid": "PlayerID",
			"#sw":  "SeasonWeek",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pid":  &types.AttributeValueMemberS{Value: playerID},
			":pref": &types.AttributeValueMemberS{Value: prefix},
		},
		ProjectionExpression: aws.String("SeasonWeek, DefSnapPct"),
	})
	if err != nil {
		return nil, err
	}
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

func getStr(m map[string]types.AttributeValue, key string) string {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case *types.AttributeValueMemberS:
			return t.Value
		case *types.AttributeValueMemberN:
			return t.Value
		}
	}
	return ""
}
func getFloat(m map[string]types.AttributeValue, key string) float64 {
	s := getStr(m, key)
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func slope(vals []float64) float64 {
	n := float64(len(vals))
	if n < 2 {
		return 0
	}
	var sx, sy, sxx, sxy float64
	for i, y := range vals {
		x := float64(i + 1)
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	if den == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / den
}
