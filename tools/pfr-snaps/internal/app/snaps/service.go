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
	"github.com/tyler180/fantasy-football-backends/internal/pfr"
	"github.com/tyler180/fantasy-football-backends/internal/snaps"
	"github.com/tyler180/fantasy-football-backends/internal/store"
)

func LambdaEntrypoint(ctx context.Context, raw Raw) (string, error) {
	var e Event
	_ = json.Unmarshal(raw, &e)

	// Non-sticky env overrides
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
		mode = envStr("MODE", "ingest_snaps_by_game")
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

func runIngestSnapsByGame(ctx context.Context, ddb *dynamodb.Client, e Event, seasonStr string, debug bool) (string, error) {
	var seasonInt int
	fmt.Sscanf(seasonStr, "%d", &seasonInt)

	snapTable := envStr("SNAP_TABLE_NAME", "defensive_snaps_by_game")
	source := strings.ToLower(envStr("SNAP_SOURCE", "nflverse"))

	// Only showing the nflverse path, since that’s what you’re using.
	if source != "nflverse" {
		return runIngestSnapsByGamePFR(ctx, ddb, e, seasonStr, debug)
	}

	// Build team filter from event first; fallback to env TEAM_LIST if set (static)
	teamListCSV := strings.TrimSpace(e.TeamList)
	if teamListCSV == "" {
		teamListCSV = strings.TrimSpace(os.Getenv("TEAM_LIST"))
	}
	filter := buildNFLverseFilter(teamListCSV)
	logTeamFilter(debug, seasonStr, filter)

	// PFR teams list for lookups/backfills
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

	// 1) Load players table pos maps (ID + normalized name)
	playersTable := envStr("TABLE_NAME", "defensive_players_by_team")
	fillFromPlayers := envBool("FILL_POS_FROM_PLAYERS", true)
	var idPos map[string]string
	var namePos map[string]string
	if fillFromPlayers {
		if mID, mName, err := store.LoadPlayerPositions(ctx, ddb, playersTable, seasonStr, pfrTeams); err == nil {
			idPos, namePos = mID, mName
		} else if debug {
			log.Printf("snaps[nflverse]: WARN could not backfill positions from players table: %v", err)
		}
	}

	// 2) Optional: add roster table fallback into namePos
	if envBool("BACKFILL_FROM_ROSTER", true) {
		rosterTable := envStr("ROSTER_TABLE_NAME", "nfl_roster_rows")
		if mRoster, err := store.LoadRosterPositions(ctx, ddb, rosterTable, seasonStr, pfrTeams); err == nil {
			if namePos == nil {
				namePos = mRoster
			} else {
				for k, v := range mRoster {
					if _, exists := namePos[k]; !exists {
						namePos[k] = v
					}
				}
			}
		} else if debug {
			log.Printf("snaps[nflverse]: WARN could not backfill positions from roster: %v", err)
		}
	}

	// 3) Optionally translate nflverse ids → PFR ids (so PlayerID matches your players table)
	useIDs := envBool("SNAP_IDS_ENABLE", true)
	idsURL := envStr("IDS_URL", "")
	var gsis2pfr, name2pfr map[string]string
	if useIDs {
		if g2p, n2p, err := snaps.FetchNflversePlayerIDs(ctx, idsURL); err == nil {
			gsis2pfr, name2pfr = g2p, n2p
		} else if debug {
			log.Printf("snaps[nflverse]: WARN could not fetch player_ids.csv: %v", err)
		}
	}

	// 4) Fetch NFLverse snap counts (CSV)
	rows, err := snaps.FetchNflverseSnapCounts(ctx, seasonInt, filter)
	if err != nil {
		return "", fmt.Errorf("fetch nflverse: %w", err)
	}

	// 5) Build defensive filter and defaults
	defPosSet := buildPosSet(envStr("DEF_POSITIONS",
		envStr("POSITIONS", "DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB")))
	keepAll := envBool("KEEP_ALL_POS", false)
	defaultDef := strings.ToUpper(strings.TrimSpace(envStr("DEFAULT_DEF_POS", "DB")))

	kept, dropped := 0, 0
	filledEmpty, canonicalized, filledByName, filledDefault, missing := 0, 0, 0, 0, 0
	missingIDs := make([]string, 0, 10)
	canonSamples := make([]string, 0, 10)

	out := make([]pfr.SnapGameRow, 0, len(rows))

	for _, r := range rows {
		// Map team to PFR code
		pfrTeam := r.Team
		if x, ok := nflverseToPFR[r.Team]; ok {
			pfrTeam = x
		}

		// Derive best PlayerID for storage/backfill:
		playerID := r.PlayerID // nflverse "player_id" (often GSIS ID)
		if useIDs {
			if pfrID, ok := gsis2pfr[playerID]; ok && pfrID != "" {
				playerID = pfrID
			} else if pfrID, ok := name2pfr[normName(r.Player)]; ok && pfrID != "" {
				playerID = pfrID
			}
		}

		csvPos := strings.ToUpper(strings.TrimSpace(r.Position))
		pos := csvPos

		// Prefer canonical position from players table by PlayerID
		if fillFromPlayers {
			if bp, ok := idPos[playerID]; ok && strings.TrimSpace(bp) != "" {
				bp = strings.ToUpper(strings.TrimSpace(bp))
				if csvPos == "" {
					filledEmpty++
				} else if bp != csvPos {
					canonicalized++
					if len(canonSamples) < 10 {
						canonSamples = append(canonSamples, fmt.Sprintf("%s: %s->%s", playerID, csvPos, bp))
					}
				}
				pos = bp
			} else if csvPos == "" {
				// Fallback by normalized name (players or roster maps)
				if bp, ok := namePos[normName(r.Player)]; ok && bp != "" {
					pos = bp
					filledByName++
				}
			}
		}

		// Filtering logic:
		if !keepAll {
			// Treat any row with DefensePct > 0 as defensive participation
			if r.DefensePct <= 0 {
				dropped++
				continue
			}
			// If still no pos, assign default defensive position
			if pos == "" {
				pos = defaultDef
				filledDefault++
			}
			// If pos still somehow not in allow list, force-allow because DefensePct>0 proves defense
			if !isDefensive(pos, defPosSet) {
				// Optionally map generic DL/DB synonyms here
				// For now we let it through when DefensePct>0
			}
		}

		// Track missing diagnostics (should be rare now)
		if pos == "" {
			missing++
			if len(missingIDs) < 10 {
				missingIDs = append(missingIDs, playerID)
			}
		}

		out = append(out, pfr.SnapGameRow{
			Season:     seasonStr,
			Team:       pfrTeam,
			Week:       r.Week,
			PlayerID:   playerID,
			Player:     r.Player,
			Pos:        pos,
			DefSnapPct: r.DefensePct,
		})
		kept++
	}

	if debug {
		logTeamFilter(debug, seasonStr, filter)
		if missing > 0 {
			log.Printf("snaps[nflverse]: WARNING missing pos for %d kept rows; sample IDs=%v", missing, missingIDs)
		}
		if canonicalized > 0 {
			log.Printf("snaps[nflverse]: canonicalized %d rows (csv->canonical); samples=%v", canonicalized, canonSamples)
		}
		log.Printf("snaps[nflverse]: kept=%d filled_empty=%d filled_by_name=%d filled_default=%d canonicalized=%d dropped_non_def=%d",
			kept, filledEmpty, filledByName, filledDefault, canonicalized, dropped)
	}

	if len(out) > 0 {
		if err := store.PutSnapGameRows(ctx, ddb, snapTable, out); err != nil {
			return "", fmt.Errorf("write snap rows: %w", err)
		}
	}
	log.Printf("OK snaps[nflverse]: wrote %d rows to %s for %s", len(out), snapTable, seasonStr)
	return fmt.Sprintf("snaps=%d", len(out)), nil
}

// ---- PFR fallback kept for completeness (unchanged) ----

func runIngestSnapsByGamePFR(ctx context.Context, ddb *dynamodb.Client, e Event, seasonStr string, debug bool) (string, error) {
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

	snapTable := envStr("SNAP_TABLE_NAME", "defensive_snaps_by_game")
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
