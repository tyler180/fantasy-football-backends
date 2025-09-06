package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"

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
	// ---- Env ----
	season := getenv("SEASON", "2025")
	tableName := mustenv("TABLE_NAME")
	maxAge := pfr.Atoi(getenv("MAX_AGE", "24"), 24)
	positionsCSV := getenv("POSITIONS", "DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB")
	posTokens := pfr.ParsePositions(positionsCSV)

	// ---- AWS clients ----
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	ddbc := ddb.NewFromConfig(cfg)

	var s3c *s3.Client
	s3Bucket := os.Getenv("S3_BUCKET") // optional
	s3Prefix := strings.TrimSuffix(getenv("S3_PREFIX", "pfr"), "/")
	if s3Bucket != "" {
		s3c = s3.NewFromConfig(cfg)
	}

	// ---- Fetch & parse: CSV → League HTML → Team pages ----
	csvText, csvErr := pfr.FetchDefenseCSV(ctx, season)

	var rows []pfr.PlayerRow
	if csvErr == nil {
		// Optional S3 archive of raw CSV when CSV path succeeds
		if s3c != nil && csvText != "" {
			date := time.Now().UTC().Format("20060102")
			baseKey := s3Prefix + "/defense_" + season + ".csv"
			snapKey := s3Prefix + "/snapshots/defense_" + season + "_" + date + ".csv"
			for _, key := range []string{baseKey, snapKey} {
				_, err := s3c.PutObject(ctx, &s3.PutObjectInput{
					Bucket:      aws.String(s3Bucket),
					Key:         aws.String(key),
					ContentType: aws.String("text/csv"),
					Body:        strings.NewReader(csvText),
				})
				if err != nil {
					return err
				}
			}
		}

		parsed, err := pfr.ParseAndFilterCSV(strings.NewReader(csvText), posTokens, maxAge)
		if err != nil {
			return err
		}
		rows = parsed
	} else {
		// League HTML fallback
		var leagueErr error
		if html, _, e := pfr.FetchDefensePage(ctx, season); e == nil {
			if parsed, err := pfr.ParseDefenseHTML(html, posTokens, maxAge); err == nil && len(parsed) > 0 {
				rows = parsed
			} else {
				leagueErr = err
			}
		} else {
			leagueErr = e
		}

		// Per-team pages fallback (most robust) if league HTML path didn’t yield rows
		if len(rows) == 0 {
			teamRows, teamErr := pfr.FetchSeasonDefenseViaTeams(ctx, season, posTokens, maxAge)
			if teamErr != nil {
				return fmt.Errorf("csv path failed (%v); league html failed (%v); team pages failed: %w", csvErr, leagueErr, teamErr)
			}
			rows = teamRows
		}
	}

	// ---- Upsert to DynamoDB ----
	if err := store.PutRows(ctx, ddbc, tableName, season, rows); err != nil {
		return err
	}

	log.Printf("OK: %d rows into %s for season %s", len(rows), tableName, season)
	return nil
}

func main() {
	lambda.Start(handler)
}
