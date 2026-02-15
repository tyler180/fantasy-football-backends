package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"

	// "tools/athena-materializer/internal/materializer"
	"github.com/tyler180/fantasy-football-backends/tools/athena-materializer/internal/materializer"
)

type Event struct {
	Season int `json:"season"` // optional; falls back to env
}

type Response struct {
	OK       bool     `json:"ok"`
	Season   int      `json:"season"`
	Table    string   `json:"table"`
	QueryIDs []string `json:"query_ids"`
	Message  string   `json:"message,omitempty"`
	RowCount int64    `json:"row_count,omitempty"`
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}

func main() { lambda.Start(handler) }

func handler(ctx context.Context, e json.RawMessage) (*Response, error) {
	// --- Inputs ---
	var evt Event
	_ = json.Unmarshal(e, &evt)
	db := mustEnv("ATHENA_DB")
	wg := mustEnv("ATHENA_WORKGROUP")
	season := evt.Season
	if season == 0 {
		// fall back to env if not in the event
		if s := os.Getenv("SEASON"); s != "" {
			_, _ = fmt.Sscanf(s, "%d", &season)
		}
	}
	if season == 0 {
		season = time.Now().Year() // last resort
	}

	// --- Client ---
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	ath := athena.NewFromConfig(cfg)

	qids := make([]string, 0, 4)

	// 1) DROP (best effort)
	drop := materializer.BuildDrop(db)
	if qid, err := runAthena(ctx, ath, wg, db, drop); err != nil {
		log.Printf("WARN drop table failed: %v", err)
	} else {
		qids = append(qids, qid)
	}

	// 2) CTAS (required)
	ctas := materializer.BuildCTAS(db, season)
	qid, err := runAthena(ctx, ath, wg, db, ctas)
	if err != nil {
		return nil, fmt.Errorf("create CTAS: %w", err)
	}
	qids = append(qids, qid)

	// 3) Sanity: count rows
	countSQL := materializer.BuildCount(db, season)
	count, qid, err := fetchSingleInt(ctx, ath, wg, db, countSQL)
	if err != nil {
		return nil, fmt.Errorf("count rows: %w", err)
	}
	qids = append(qids, qid)

	// Optional: log per-team counts & a small sample (doesn’t fail the run)
	if qid, err := runAthena(ctx, ath, wg, db, materializer.BuildPerTeamCounts(db, season)); err == nil {
		qids = append(qids, qid)
	}
	if qid, err := runAthena(ctx, ath, wg, db, materializer.BuildSample(db, season)); err == nil {
		qids = append(qids, qid)
	}

	return &Response{
		OK:       true,
		Season:   season,
		Table:    fmt.Sprintf("%s.%s", db, materializer.TableName),
		QueryIDs: qids,
		RowCount: count,
	}, nil
}

// runAthena submits a query and waits for SUCCEEDED; logs useful stats.
func runAthena(ctx context.Context, c *athena.Client, workgroup, database, sql string) (string, error) {
	start, err := c.StartQueryExecution(ctx, &athena.StartQueryExecutionInput{
		QueryString: aws.String(sql),
		QueryExecutionContext: &types.QueryExecutionContext{
			Database: aws.String(database),
		},
		WorkGroup: aws.String(workgroup),
	})
	if err != nil {
		return "", err
	}

	qid := aws.ToString(start.QueryExecutionId)
	for {
		time.Sleep(800 * time.Millisecond)
		ge, err := c.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{
			QueryExecutionId: aws.String(qid),
		})
		if err != nil {
			return qid, err
		}

		state := ge.QueryExecution.Status.State
		switch state {
		case types.QueryExecutionStateSucceeded:
			// Stats are pointers; guard nils
			var scannedMB float64
			if ge.QueryExecution.Statistics != nil && ge.QueryExecution.Statistics.DataScannedInBytes != nil {
				scannedMB = float64(aws.ToInt64(ge.QueryExecution.Statistics.DataScannedInBytes)) / (1024 * 1024)
			}
			var durMs float64
			if ge.QueryExecution.Statistics != nil && ge.QueryExecution.Statistics.EngineExecutionTimeInMillis != nil {
				durMs = float64(aws.ToInt64(ge.QueryExecution.Statistics.EngineExecutionTimeInMillis))
			}
			log.Printf("athena: OK qid=%s scanned=%.1fMB time=%.0fms", qid, scannedMB, durMs)
			return qid, nil

		case types.QueryExecutionStateFailed, types.QueryExecutionStateCancelled:
			msg := "unknown error"
			if ge.QueryExecution.Status.AthenaError != nil && ge.QueryExecution.Status.AthenaError.ErrorMessage != nil {
				msg = aws.ToString(ge.QueryExecution.Status.AthenaError.ErrorMessage)
			} else if ge.QueryExecution.Status.StateChangeReason != nil {
				msg = aws.ToString(ge.QueryExecution.Status.StateChangeReason)
			}
			return qid, fmt.Errorf("athena failed: %s", msg)
		default:
			// running/queued: continue
		}
	}
}

// fetchSingleInt runs a query that returns a single BIGINT (e.g., COUNT(*)).
func fetchSingleInt(ctx context.Context, c *athena.Client, wg, db, sql string) (int64, string, error) {
	qid, err := runAthena(ctx, c, wg, db, sql)
	if err != nil {
		return 0, qid, err
	}

	res, err := c.GetQueryResults(ctx, &athena.GetQueryResultsInput{
		QueryExecutionId: aws.String(qid),
	})
	if err != nil {
		return 0, qid, err
	}
	// row 0 is header; row 1 is value
	if len(res.ResultSet.Rows) < 2 || len(res.ResultSet.Rows[1].Data) < 1 {
		return 0, qid, fmt.Errorf("no rows returned")
	}
	var n int64
	if _, err := fmt.Sscan(aws.ToString(res.ResultSet.Rows[1].Data[0].VarCharValue), &n); err != nil {
		return 0, qid, fmt.Errorf("parse result: %w", err)
	}
	log.Printf("athena: count=%d", n)
	return n, qid, nil
}
