package materializer

import "fmt"

const TableName = "defensive_starters_allgames"

// BuildDrop returns a DROP TABLE IF EXISTS for the materialized table.
func BuildDrop(db string) string {
	return fmt.Sprintf(`DROP TABLE IF EXISTS %s.%s`, db, TableName)
}

// BuildCTAS returns the CTAS used to materialize starters for a season.
// NOTE: season is injected literally (no ${...} placeholders).
func BuildCTAS(db string, season int) string {
	return fmt.Sprintf(`
CREATE TABLE %s.%s
WITH (
  format = 'PARQUET',
  partitioned_by = ARRAY['season','team']
) AS
WITH sc AS (
  SELECT
    CAST(season AS INTEGER)  AS season,
    CAST(week   AS INTEGER)  AS week,
    UPPER(TRIM(team))        AS team,
    defense_pct              AS def_pct,
    REGEXP_REPLACE(
      REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(player)), '\\.', ''),
        '[^A-Z0-9 ]',''
      ),
      '\\s+(JR|SR|II|III|IV)\\s*$',''
    ) AS norm_name
  FROM %s.snap_counts
  WHERE season='%d' AND defense_pct IS NOT NULL
),
rw AS (
  SELECT
    CAST(season AS INTEGER)  AS season,
    CAST(week   AS INTEGER)  AS week,
    UPPER(TRIM(team))        AS team,
    player_id,
    full_name,
    position,
    REGEXP_REPLACE(
      REGEXP_REPLACE(
        REGEXP_REPLACE(UPPER(TRIM(full_name)), '\\.', ''),
        '[^A-Z0-9 ]',''
      ),
      '\\s+(JR|SR|II|III|IV)\\s*$',''
    ) AS norm_name
  FROM %s.rosters_weekly
  WHERE season='%d'
)
SELECT
  rw.player_id                          AS player_id,
  MAX_BY(rw.full_name,  sc.week)        AS player_name,
  MAX_BY(rw.position,   sc.week)        AS position,
  COUNT(*)                               AS games,
  ROUND(AVG(sc.def_pct), 2)              AS avg_def_pct,
  %d                                     AS season,
  sc.team                                AS team
FROM sc
JOIN rw
  ON sc.season = rw.season
 AND sc.week   = rw.week
 AND sc.team   = rw.team
 AND sc.norm_name = rw.norm_name
GROUP BY rw.player_id, sc.team
`, db, TableName, db, season, db, season, season)
}

// Some light sanity/QA queries you can log after CTAS finishes.
func BuildCount(db string, season int) string {
	return fmt.Sprintf(`SELECT COUNT(*) AS rows FROM %s.%s WHERE season=%d`, db, TableName, season)
}

func BuildPerTeamCounts(db string, season int) string {
	return fmt.Sprintf(`
SELECT team, COUNT(*) AS players
FROM %s.%s
WHERE season=%d
GROUP BY team
ORDER BY team`, db, TableName, season)
}

func BuildSample(db string, season int) string {
	return fmt.Sprintf(`
SELECT team, player_name, position, games, avg_def_pct
FROM %s.%s
WHERE season=%d
ORDER BY team, player_name
LIMIT 25`, db, TableName, season)
}
