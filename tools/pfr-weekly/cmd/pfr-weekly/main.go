package main

import (
	"context"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"pfr-weekly/internal/pfr"
	"pfr-weekly/internal/store"
)

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func mustenv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env %s", k)
	}
	return v
}

func handler(ctx context.Context) error {
	mode := strings.ToLower(getenv("MODE", "all")) // ingest_roster | materialize_defense | all
	season := getenv("SEASON", "2024")

	rosterTable := mustenv("ROSTER_TABLE_NAME") // e.g. nfl_roster_rows
	outTable := mustenv("TABLE_NAME")           // your defensive players table

	maxAge := pfr.Atoi(getenv("MAX_AGE", "24"), 24)
	posTokens := pfr.ParsePositions(getenv("POSITIONS", "DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB"))

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	ddbc := ddb.NewFromConfig(cfg)

	// 1) Ingest rosters (raw rows)
	if mode == "ingest_roster" || mode == "all" {
		rows, err := pfr.FetchSeasonRosterRows(ctx, season)
		if err != nil {
			return err
		}
		if err := store.PutRosterRows(ctx, ddbc, rosterTable, rows); err != nil {
			return err
		}
		log.Printf("OK ingest: %d roster rows into %s for season %s", len(rows), rosterTable, season)
	}

	// 2) Materialize filtered defensive view from Dynamo
	if mode == "materialize_defense" || mode == "all" {
		rows, err := store.MaterializeDefenseFromRoster(ctx, ddbc, rosterTable, season, posTokens, maxAge)
		if err != nil {
			return err
		}
		if err := store.PutRows(ctx, ddbc, outTable, season, rows); err != nil {
			return err
		}
		log.Printf("OK materialize: %d rows into %s for season %s", len(rows), outTable, season)
	}

	return nil
}

func main() {
	lambda.Start(handler)
}
