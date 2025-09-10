package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	athena "github.com/aws/aws-sdk-go-v2/service/athena"
	athenatypes "github.com/aws/aws-sdk-go-v2/service/athena/types"
)

type Event struct {
	Season     int `json:"season"`
	MaxAge     int `json:"max_age"`     // optional; 0 = ignore
	StarterPct int `json:"starter_pct"` // e.g., 50
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func mustIntEnv(k string, def int) int {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func mustNonEmptyEnv(k string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		log.Fatalf("%s env var is required", k)
	}
	return v
}

func buildCTAS(db, table string, season, starterPct, maxAge int, outputLocation string) (dropStmt, ctasStmt string) {
	seasonDate := fmt.Sprintf("%04d-09-01", season)

	ageFilter := ""
	if maxAge > 0 {
		ageFilter = fmt.Sprintf("  AND age_yrs <= %d\n", maxAge)
	}

	queryBody := fmt.Sprintf(`
WITH snaps AS (
  SELECT
    TRY_CAST(sc.season AS INTEGER)        AS season,     -- INT
    sc.team                                AS team,      -- VARCHAR
    TRY_CAST(sc.week   AS INTEGER)        AS week,       -- INT
    COALESCE(sc.player_id, '')            AS player_id,  -- GSIS (VARCHAR)
    sc.player                              AS player_name,
    sc.defense_pct
  FROM %s.snap_counts sc
  WHERE TRY_CAST(sc.season AS INTEGER) = %d
),
roster AS (
  SELECT
    TRY_CAST(rw.season AS INTEGER)        AS season,     -- INT
    rw.team                                AS team,      -- VARCHAR
    TRY_CAST(rw.week   AS INTEGER)        AS week,       -- INT
    COALESCE(rw.player_id, '')            AS player_id,  -- GSIS (VARCHAR)
    rw.pfr_id                              AS pfr_id,
    rw.full_name                           AS full_name,
    UPPER(COALESCE(rw.position,''))       AS position
  FROM %s.rosters_weekly rw
  WHERE TRY_CAST(rw.season AS INTEGER) = %d
    AND UPPER(COALESCE(rw.position,'')) IN ('DL','DE','DT','NT','EDGE','LB','ILB','OLB','MLB','CB','DB','S','FS','SS','SAF','NB')
),
joined AS (
  SELECT
    s.season, s.team, s.week,
    COALESCE(s.player_id, r.player_id)       AS player_id,   -- GSIS
    COALESCE(r.pfr_id, p.pfr_id)             AS pfr_id,      -- prefer roster PFR, else players via GSIS
    COALESCE(s.player_name, r.full_name)     AS player_name,
    r.position,
    s.defense_pct,
    CASE
      WHEN p.birth_date IS NOT NULL AND TRY(date_parse(p.birth_date, '%%Y-%%m-%%d')) IS NOT NULL
        THEN CAST(date_diff('year', TRY(date_parse(p.birth_date, '%%Y-%%m-%%d')), DATE '%s') AS integer)
      ELSE NULL
    END AS age_yrs
  FROM snaps s
  LEFT JOIN roster r
    ON  CAST(s.season AS VARCHAR) = CAST(r.season AS VARCHAR)   -- force VARCHAR = VARCHAR
    AND s.team = r.team                                         -- VARCHAR = VARCHAR
    AND CAST(s.week   AS VARCHAR) = CAST(r.week   AS VARCHAR)   -- force VARCHAR = VARCHAR
    AND (
         (s.player_id <> '' AND s.player_id = r.player_id)
      OR (s.player_id = ''  AND LOWER(s.player_name) = LOWER(r.full_name))
    )
  LEFT JOIN %s.players p
    ON COALESCE(s.player_id, r.player_id) = p.gsis_id
),
agg AS (
  SELECT
    season, team, player_id, pfr_id, player_name,
    MAX(position)                           AS position,
    MAX(age_yrs)                            AS age_yrs,
    COUNT_IF(defense_pct IS NOT NULL)       AS games_with_snap,
    COUNT(*)                                AS games_total,
    AVG(defense_pct)                        AS avg_def_pct,
    MIN(defense_pct)                        AS min_def_pct,
    MAX(defense_pct)                        AS max_def_pct
  FROM joined
  GROUP BY season, team, player_id, pfr_id, player_name
)
SELECT
  -- non-partition columns FIRST:
  player_id,
  pfr_id,
  player_name,
  position,
  age_yrs,
  games_with_snap,
  games_total,
  avg_def_pct,
  min_def_pct,
  max_def_pct,
  -- partition columns LAST in the same order as partitioned_by:
  CAST(season AS INTEGER) AS season,
  CAST(team   AS VARCHAR) AS team
FROM agg
WHERE games_with_snap = games_total
  AND avg_def_pct >= CAST(%d AS DOUBLE)
%s`, db, season, db, season, seasonDate, db, starterPct, ageFilter)

	dropStmt = fmt.Sprintf(`DROP TABLE IF EXISTS %s.%s`, db, table)

	ctasStmt = fmt.Sprintf(`
CREATE TABLE %s.%s
WITH (
  format = 'PARQUET',
  external_location = '%s',
  partitioned_by = ARRAY['season','team']
) AS
%s
`, db, table, strings.TrimRight(outputLocation, "/"), queryBody)

	if strings.TrimSpace(os.Getenv("DEBUG")) == "1" {
		log.Printf("DEBUG CTAS SQL:\n%s\n", ctasStmt)
	}

	return dropStmt, ctasStmt
}

type JobResult struct {
	QueryExecutionID string `json:"query_execution_id"`
	State            string `json:"state"`
}

func runAthena(ctx context.Context, cl *athena.Client, database, workgroup, output, sql string) (JobResult, error) {
	start, err := cl.StartQueryExecution(ctx, &athena.StartQueryExecutionInput{
		QueryString: aws.String(sql),
		QueryExecutionContext: &athenatypes.QueryExecutionContext{
			Database: aws.String(database), // or omit by setting this field nil
			// Catalog: aws.String("AwsDataCatalog"), // optional
		},
		ResultConfiguration: &athenatypes.ResultConfiguration{
			OutputLocation: aws.String(output),
		},
		WorkGroup: aws.String(workgroup),
	})
	if err != nil {
		return JobResult{}, err
	}

	qid := aws.ToString(start.QueryExecutionId)

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		desc, err := cl.GetQueryExecution(ctx, &athena.GetQueryExecutionInput{
			QueryExecutionId: &qid,
		})
		if err != nil {
			return JobResult{QueryExecutionID: qid}, err
		}
		state := desc.QueryExecution.Status.State
		switch state {
		case athenatypes.QueryExecutionStateSucceeded:
			return JobResult{QueryExecutionID: qid, State: string(state)}, nil
		case athenatypes.QueryExecutionStateFailed, athenatypes.QueryExecutionStateCancelled:
			return JobResult{QueryExecutionID: qid, State: string(state)},
				fmt.Errorf("athena failed: %s: %s",
					aws.ToString(desc.QueryExecution.Status.StateChangeReason),
					aws.ToString(desc.QueryExecution.Status.AthenaError.ErrorMessage))
		default:
			// keep polling
		}
	}
	return JobResult{QueryExecutionID: qid}, fmt.Errorf("athena timeout waiting for query to finish")
}

func handler(ctx context.Context, e Event) (any, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	cl := athena.NewFromConfig(awsCfg)

	db := getenv("ATHENA_DB", "nflverse_curated")
	wg := getenv("ATHENA_WORKGROUP", "primary")
	out := mustNonEmptyEnv("ATHENA_OUTPUT") // e.g., s3://nflverse-athena-query-results/results/

	serveTable := getenv("SERVE_TABLE", "defensive_starters_allgames")

	season := e.Season
	if season == 0 {
		season = mustIntEnv("SEASON", 2024)
	}
	starterPct := e.StarterPct
	if starterPct == 0 {
		starterPct = mustIntEnv("STARTER_PCT", 50)
	}
	maxAge := e.MaxAge
	if maxAge == 0 {
		maxAge = mustIntEnv("MAX_AGE", 0) // 0 = ignore
	}

	// Write serving data to a friendly sub-prefix (optional but cleaner).
	// If you prefer a separate bucket/prefix, set ATHENA_OUTPUT to that exact location.
	servePrefix := strings.TrimRight(out, "/") + fmt.Sprintf("/serve/%s/season=%d/", serveTable, season)

	dropSQL, ctasSQL := buildCTAS(db, serveTable, season, starterPct, maxAge, servePrefix)

	log.Printf("materializer: dropping table %s.%s (if exists)", db, serveTable)
	if _, err := runAthena(ctx, cl, db, wg, out, dropSQL); err != nil {
		// Not fatal if it fails because the table doesn't exist; Athena handles it, but keep message.
		log.Printf("WARN drop table: %v", err)
	}

	log.Printf("materializer: creating table %s.%s via CTAS", db, serveTable)
	res, err := runAthena(ctx, cl, db, wg, out, ctasSQL)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"ok":               true,
		"season":           season,
		"starter_pct":      starterPct,
		"max_age":          maxAge,
		"athena_workgroup": wg,
		"athena_output":    out,
		"serve_table":      fmt.Sprintf("%s.%s", db, serveTable),
		"query_id":         res.QueryExecutionID,
	}, nil
}

func main() {
	lambda.Start(handler)
	// (No local branch—this binary is for the Lambda runtime.)
}
