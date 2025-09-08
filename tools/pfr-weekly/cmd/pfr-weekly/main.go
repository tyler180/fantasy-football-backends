package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/tyler1808/fantasy-football-backends/internal/pfr"
	"github.com/tyler1808/fantasy-football-backends/internal/store"
)

type Event struct {
	Mode           string `json:"mode"`   // "ingest_roster", "materialize_defense"
	Season         string `json:"season"` // optional override
	TeamChunkTotal *int   `json:"team_chunk_total"`
	TeamChunkIndex *int   `json:"team_chunk_index"`
	TeamList       string `json:"team_list"`
	SnapCounts     *bool  `json:"snap_counts"`
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
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
	if e.SnapCounts != nil {
		if *e.SnapCounts {
			os.Setenv("SNAP_COUNTS", "1")
		} else {
			os.Setenv("SNAP_COUNTS", "0")
		}
	}
}

func handler(ctx context.Context, raw json.RawMessage) (string, error) {
	// Parse event (safe if empty)
	var e Event
	_ = json.Unmarshal(raw, &e)
	applyEventOverrides(e)

	season := strings.TrimSpace(os.Getenv("SEASON"))
	if season == "" {
		season = "2024"
	}
	mode := strings.TrimSpace(e.Mode)
	if mode == "" {
		mode = strings.TrimSpace(os.Getenv("MODE"))
	}
	if mode == "" {
		// default to materialize-only, so we never hit PFR unless explicitly requested
		mode = "materialize_defense"
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("aws config: %w", err)
	}
	ddb := dynamodb.NewFromConfig(cfg)

	switch mode {
	case "materialize_defense":
		// Read roster from DynamoDB → aggregate → write defensive table (NO PFR calls)
		rosterTable := strings.TrimSpace(os.Getenv("ROSTER_TABLE_NAME"))
		if rosterTable == "" {
			rosterTable = "nfl_roster_rows"
		}
		outTable := strings.TrimSpace(os.Getenv("TABLE_NAME"))
		if outTable == "" {
			outTable = "defensive_players_" + season
		}

		defPos := pfr.ParsePositions(os.Getenv("POSITIONS"))
		maxAge := envInt("MAX_AGE", 24) // not used in filtering anymore, but signature needs it

		rows, err := store.MaterializeDefenseFromRoster(ctx, ddb, rosterTable, season, defPos, maxAge)
		if err != nil {
			return "", fmt.Errorf("materialize from roster: %w", err)
		}

		if err := store.PutRows(ctx, ddb, outTable, season, rows); err != nil {
			return "", fmt.Errorf("write defensive rows: %w", err)
		}
		log.Printf("OK materialize: %d defensive rows into %s for season %s", len(rows), outTable, season)
		return fmt.Sprintf("materialized %d rows", len(rows)), nil

	case "ingest_roster":
		// (your existing ingest path that hits PFR goes here)
		// left unchanged; not shown to keep this focused
		return "ingest skipped in this snippet", nil

	default:
		return "", fmt.Errorf("unknown mode %q", mode)
	}
}

func main() { lambda.Start(handler) }
