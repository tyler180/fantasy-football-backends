package snaps

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	// update to your module path
	"github.com/tyler180/fantasy-football-backends/internal/pfr" // if you keep PFR fallback; otherwise remove
	"github.com/tyler180/fantasy-football-backends/internal/snaps"
	"github.com/tyler180/fantasy-football-backends/internal/store"
)

// LambdaEntrypoint is the single Lambda handler exported from this package.
func LambdaEntrypoint(ctx context.Context, raw Raw) (string, error) {
	var e Event
	_ = json.Unmarshal(raw, &e)

	// Apply non-sticky overrides (season/chunking only)
	if e.Season != "" {
		os.Setenv("SEASON", e.Season)
	}
	if e.TeamChunkTotal != nil {
		os.Setenv("TEAM_CHUNK_TOTAL", fmt.Sprintf("%d", *e.TeamChunkTotal))
	}
	if e.TeamChunkIndex != nil {
		os.Setenv("TEAM_CHUNK_INDEX", fmt.Sprintf("%d", *e.TeamChunkIndex))
	}

	seasonStr := envStr("SEASON", "2024")
	mode := strings.TrimSpace(e.Mode)
	if mode == "" {
		mode = envStr("MODE", "_ingest_snaps_by_game") // default; note the underscore to catch unset bugs
		mode = "ingest_snaps_by_game"
	}
	debug := envBool("DEBUG", false)

	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("aws config: %w", err)
	}
	ddb := dynamodb.NewFromConfig(awsCfg)

	switch mode {
	case "ingest_snaps_by_game":
		return runIngestSnapsByGame(ctx, ddb, e, seasonStr, debug)

	case "materialize_snap_trends":
		return runMaterializeTrends(ctx, ddb, seasonStr, debug)

	default:
		return "", fmt.Errorf("unknown mode %q", mode)
	}
}

// ------------------ ingest (NFLverse first, PFR fallback optional) ------------------

func runIngestSnapsByGame(ctx context.Context, ddb *dynamodb.Client, e Event, seasonStr string, debug bool) (string, error) {
	seasonInt := 0
	fmt.Sscanf(seasonStr, "%d", &seasonInt)

	snapTable := envStr("SNAP_TABLE_NAME", "defensive_snaps_by_game")
	source := strings.ToLower(envStr("SNAP_SOURCE", "nflverse"))

	if source == "nflverse" {
		// Build team filter from event; fallback to config TEAM_LIST (static) only
		teamListCSV := strings.TrimSpace(e.TeamList)
		if teamListCSV == "" {
			teamListCSV = strings.TrimSpace(os.Getenv("TEAM_LIST"))
		}
		filter := buildNFLverseFilter(teamListCSV)
		logTeamFilter(debug, seasonStr, filter)

		// Build PFR team list for backfill from defensive_players_by_team
		pfrTeams := make([]string, 0, 32)
		if len(filter) == 0 {
			for _, t := range pfr.AllTeams() {
				pfrTeams = append(pfrTeams, t.Abbr)
			}
		} else {
			for nv := range filter {
				if p, ok := nflverseToPFR[nv]; ok {
					pfrTeams = append(pfrTeams, p)
				}
			}
		}

		playersTable := envStr("TABLE_NAME", "defensive_players_by_team")
		fillFromPlayers := envBool("FILL_POS_FROM_PLAYERS", true)

		posBackfill := map[string]string{}
		if fillFromPlayers {
			if m, err := store.LoadPlayerPositions(ctx, ddb, playersTable, seasonStr, pfrTeams); err == nil {
				posBackfill = m
			} else if debug {
				log.Printf("snaps[nflverse]: WARN could not backfill positions: %v", err)
			}
		}

		// Fetch NFLverse snap counts
		rows, err := snaps.FetchNflverseSnapCounts(ctx, seasonInt, filter)
		if err != nil {
			return "", fmt.Errorf("fetch nflverse: %w", err)
		}

		defPosSet := buildPosSet(envStr("DEF_POSITIONS",
			envStr("POSITIONS", "DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB")))
		keepAll := envBool("KEEP_ALL_POS", false)

		kept, dropped, filled, missing := 0, 0, 0, 0
		missingIDs := make([]string, 0, 12)
		out := make([]pfr.SnapGameRow, 0, len(rows))

		for _, r := range rows {
			pfrTeam := r.Team
			if x, ok := nflverseToPFR[r.Team]; ok {
				pfrTeam = x
			}

			csvPos := strings.ToUpper(strings.TrimSpace(r.Position))
			// Prefer canonical position from players table when available
			backfillPos, have := posBackfill[r.PlayerID]
			pos := csvPos
			if have && strings.TrimSpace(backfillPos) != "" {
				pos = strings.ToUpper(strings.TrimSpace(backfillPos))
				if csvPos == "" {
					filled++
				}
			} else if csvPos == "" {
				missing++
				if len(missingIDs) < 12 {
					missingIDs = append(missingIDs, r.PlayerID)
				}
			}

			if !keepAll && !isDefensive(pos, defPosSet) {
				dropped++
				continue
			}

			out = append(out, pfr.SnapGameRow{
				Season:     seasonStr,
				Team:       pfrTeam,
				Week:       r.Week,
				PlayerID:   r.PlayerID,
				Player:     r.Player,
				Pos:        pos,
				DefSnapPct: r.DefensePct,
			})
			kept++
		}

		if debug {
			if missing > 0 {
				log.Printf("snaps[nflverse]: WARNING missing pos for %d kept rows; sample PlayerIDs=%v", missing, missingIDs)
			}
			log.Printf("snaps[nflverse]: kept=%d filled_pos=%d dropped_non_def=%d", kept, filled, dropped)
		}

		if len(out) > 0 {
			if err := store.PutSnapGameRows(ctx, ddb, snapTable, out); err != nil {
				return "", fmt.Errorf("write snap rows: %w", err)
			}
		}
		log.Printf("OK snaps[nflverse]: wrote %d rows to %s for %s", len(out), snapTable, seasonStr)
		return fmt.Sprintf("snaps=%d", len(out)), nil
	}

	// -------- PFR fallback (per-team HTML) --------
	all := pfr.AllTeams()
	subset := teamSubset(all, envStr("TEAM_LIST", e.TeamList),
		pickInt(e.TeamChunkTotal, envInt("TEAM_CHUNK_TOTAL", 0)),
		pickInt(e.TeamChunkIndex, envInt("TEAM_CHUNK_INDEX", 0)),
	)
	if debug {
		abbrs := make([]string, len(subset))
		for i, t := range subset {
			abbrs[i] = t.Abbr
		}
		log.Printf("snaps[pfr]: season=%s teams=%d: %s", seasonStr, len(subset), strings.Join(abbrs, ","))
	}

	referer := fmt.Sprintf("https://www.pro-football-reference.com/years/%s/", seasonStr)
	total := 0
	for _, t := range subset {
		rows, err := pfr.FetchTeamDefSnapPctsByGame(ctx, t.Path, t.Abbr, seasonStr, referer)
		if err != nil {
			if debug {
				log.Printf("snaps[pfr]: %s failed: %v", t.Abbr, err)
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
	log.Printf("OK snaps[pfr]: wrote %d rows to %s for %s", total, snapTable, seasonStr)
	return fmt.Sprintf("snaps=%d", total), nil
}

// ------------------ trends materialization ------------------

func runMaterializeTrends(ctx context.Context, ddb *dynamodb.Client, seasonStr string, debug bool) (string, error) {
	snapTable := envStr("SNAP_TABLE_NAME", "defensive_snaps_by_game")
	playersTable := envStr("TABLE_NAME", "defensive_players_by_team")

	allTeams := pfr.AllTeams()
	updated := 0

	for _, t := range allTeams {
		players, err := listPlayersForSeasonTeam(ctx, ddb, playersTable, seasonStr, t.Abbr)
		if err != nil {
			if debug {
				log.Printf("trends: list %s err: %v", t.Abbr, err)
			}
			continue
		}
		if len(players) == 0 {
			continue
		}

		for _, pk := range players {
			vals, err := queryPlayerSnaps(ctx, ddb, snapTable, pk.PlayerID, seasonStr)
			if err != nil {
				if debug {
					log.Printf("trends: query %s %s err: %v", t.Abbr, pk.PlayerID, err)
				}
				continue
			}
			if len(vals) == 0 {
				_ = store.UpdatePlayerTrends(ctx, ddb, playersTable, seasonStr, t.Abbr, pk.PlayerID, 0, 0, 0, 0)
				continue
			}

			last := vals[len(vals)-1]
			var s3, s5, c3 float64
			if len(vals) >= 3 {
				s3 = slope(vals[len(vals)-3:])
				base := (vals[len(vals)-3] + vals[len(vals)-2]) / 2.0
				c3 = last - base
			}
			if len(vals) >= 5 {
				s5 = slope(vals[len(vals)-5:])
			}

			if err := store.UpdatePlayerTrends(ctx, ddb, playersTable, seasonStr, t.Abbr, pk.PlayerID, last, s3, s5, c3); err != nil {
				if debug {
					log.Printf("trends: update %s %s err: %v", t.Abbr, pk.PlayerID, err)
				}
				continue
			}
			updated++
		}
		time.Sleep(150 * time.Millisecond)
	}

	log.Printf("OK trends: updated %d players in %s for %s", updated, playersTable, seasonStr)
	return fmt.Sprintf("trends_updated=%d", updated), nil
}
