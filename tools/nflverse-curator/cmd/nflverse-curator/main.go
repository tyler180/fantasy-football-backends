package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	parquet "github.com/parquet-go/parquet-go"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	// update this import path to your module path
	"github.com/tyler180/fantasy-football-backends/tools/nflverse-curator/internal/nflverse"
)

/* ---------- Types & util ---------- */

type Event struct {
	Datasets []string `json:"datasets"`
	Season   int      `json:"season"`
}

type Handler struct {
	S3     *s3.Client
	Bucket string
	Prefix string
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}
func nowStamp() string { return time.Now().UTC().Format("20060102T150405Z") }

/* ---------- S3 uploader ---------- */

type s3uploader struct {
	cl     *s3.Client
	bucket string
}

func (u *s3uploader) put(ctx context.Context, key string, body []byte) error {
	_, err := u.cl.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(u.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(body),
	})
	return err
}

/* ---------- CSV helpers ---------- */

func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "nflverse-curator-go/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch %s: status %d body=%q", url, resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

func readCSV(b []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(b))
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

func idxOf(hdr []string, name string) int {
	name = strings.ToLower(name)
	for i, h := range hdr {
		if strings.ToLower(strings.TrimSpace(h)) == name {
			return i
		}
	}
	return -1
}

func get(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return strings.TrimSpace(rec[i])
}

func parseFloat(rec []string, i int) *float64 {
	s := get(rec, i)
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

/* ---------- Parquet row types ---------- */

type PlayersRow struct {
	PfrID     *string `parquet:"pfr_id,optional"`
	GsisID    *string `parquet:"gsis_id,optional"`
	FullName  *string `parquet:"full_name,optional"`
	BirthDate *string `parquet:"birth_date,optional"`
	Position  *string `parquet:"position,optional"`
}

type RostersWeeklyRow struct {
	Season   string  `parquet:"season"`
	Week     string  `parquet:"week"`
	Team     string  `parquet:"team"`
	PlayerID *string `parquet:"player_id,optional"`
	PfrID    *string `parquet:"pfr_id,optional"`
	FullName *string `parquet:"full_name,optional"`
	Position *string `parquet:"position,optional"`
	Status   *string `parquet:"status,optional"`
}

type SnapCountsRow struct {
	Season     string   `parquet:"season"`
	Week       string   `parquet:"week"`
	Team       string   `parquet:"team"`
	Player     *string  `parquet:"player,optional"`
	PlayerID   *string  `parquet:"player_id,optional"`
	OffensePct *float64 `parquet:"offense_pct,optional"`
	DefensePct *float64 `parquet:"defense_pct,optional"`
	STPct      *float64 `parquet:"st_pct,optional"`
}

/* ---------- Parquet writer ---------- */

func writeParquetAndUpload[T any](ctx context.Context, rows []T, key string, schema *parquet.Schema, up *s3uploader) error {
	if len(rows) == 0 {
		return nil
	}
	tmp := filepath.Join("/tmp", "parq-"+nowStamp()+"-"+filepath.Base(key))
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := parquet.NewWriter(f, schema, parquet.Compression(&parquet.Snappy))
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			_ = w.Close()
			_ = f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	b, err := os.ReadFile(tmp)
	if err != nil {
		return err
	}
	if err := up.put(ctx, key, b); err != nil {
		return err
	}
	_ = os.Remove(tmp)
	return nil
}

/* ---------- Dataset ingesters (CSV ➜ partitioned parquet) ---------- */

func ingestPlayers(ctx context.Context, up *s3uploader, prefix, url string) (int, error) {
	b, err := httpGet(ctx, url)
	if err != nil {
		return 0, err
	}
	rows, err := readCSV(b)
	if err != nil || len(rows) == 0 {
		return 0, errors.New("players csv empty")
	}
	h := rows[0]
	iPfr := idxOf(h, "pfr_id")
	if iPfr < 0 {
		iPfr = idxOf(h, "pfr_player_id")
	}
	iGsis := idxOf(h, "gsis_id")
	iName := idxOf(h, "full_name")
	iBirth := idxOf(h, "birth_date")
	iPos := idxOf(h, "position")

	out := make([]PlayersRow, 0, len(rows)-1)
	for _, rec := range rows[1:] {
		out = append(out, PlayersRow{
			PfrID:     strPtr(get(rec, iPfr)),
			GsisID:    strPtr(get(rec, iGsis)),
			FullName:  strPtr(get(rec, iName)),
			BirthDate: strPtr(get(rec, iBirth)),
			Position:  strPtr(get(rec, iPos)),
		})
	}
	key := fmt.Sprintf("%s/players/players-%s.parquet", prefix, nowStamp())
	return len(out), writeParquetAndUpload(ctx, out, key, parquet.SchemaOf(new(PlayersRow)), up)
}

func ingestRostersWeekly(ctx context.Context, up *s3uploader, prefix, url string) (int, error) {
	b, err := httpGet(ctx, url)
	if err != nil {
		return 0, err
	}
	rows, err := readCSV(b)
	if err != nil || len(rows) == 0 {
		return 0, errors.New("rosters_weekly csv empty")
	}
	h := rows[0]
	is := func(n string) int { return idxOf(h, n) }
	iSeason := is("season")
	iWeek := is("week")
	iTeam := is("team")
	iGsis := is("gsis_id")
	iPfr := idxOf(h, "pfr_id")
	if iPfr < 0 {
		iPfr = idxOf(h, "pfr_player_id")
	}
	iName := is("full_name")
	iPos := is("position")
	iStatus := is("status")

	// partition: season/week
	type key struct{ season, week string }
	buckets := map[key][]RostersWeeklyRow{}

	for _, rec := range rows[1:] {
		season := get(rec, iSeason)
		week := get(rec, iWeek)
		if season == "" || week == "" {
			continue
		}
		week = fmt.Sprintf("%02s", week)
		k := key{season, week}
		row := RostersWeeklyRow{
			Season:   season,
			Week:     week,
			Team:     strings.ToUpper(get(rec, iTeam)),
			PlayerID: strPtr(get(rec, iGsis)),
			PfrID:    strPtr(get(rec, iPfr)),
			FullName: strPtr(get(rec, iName)),
			Position: strPtr(get(rec, iPos)),
			Status:   strPtr(get(rec, iStatus)),
		}
		buckets[k] = append(buckets[k], row)
	}

	schema := parquet.SchemaOf(new(RostersWeeklyRow))
	total := 0
	for k, part := range buckets {
		key := fmt.Sprintf("%s/rosters_weekly/season=%s/week=%s/part-%s.parquet", prefix, k.season, k.week, nowStamp())
		if err := writeParquetAndUpload(ctx, part, key, schema, up); err != nil {
			return total, err
		}
		total += len(part)
	}
	return total, nil
}

func ingestSnapCounts(ctx context.Context, up *s3uploader, prefix, url string) (int, error) {
	b, err := httpGet(ctx, url)
	if err != nil {
		return 0, err
	}
	rows, err := readCSV(b)
	if err != nil || len(rows) == 0 {
		return 0, errors.New("snap_counts csv empty")
	}
	h := rows[0]
	is := func(n string) int { return idxOf(h, n) }
	iSeason := is("season")
	iWeek := is("week")
	iTeam := is("team")
	iPlayer := is("player")
	iPlayerID := is("player_id")
	iOff := is("offense_pct")
	iDef := is("defense_pct")
	iST := is("st_pct")

	// partition: season/team
	type key struct{ season, team string }
	buckets := map[key][]SnapCountsRow{}

	for _, rec := range rows[1:] {
		season := get(rec, iSeason)
		team := strings.ToUpper(get(rec, iTeam))
		week := get(rec, iWeek)
		if season == "" || team == "" || week == "" {
			continue
		}
		week = fmt.Sprintf("%02s", week)
		row := SnapCountsRow{
			Season:     season,
			Week:       week,
			Team:       team,
			Player:     strPtr(get(rec, iPlayer)),
			PlayerID:   strPtr(get(rec, iPlayerID)),
			OffensePct: parseFloat(rec, iOff),
			DefensePct: parseFloat(rec, iDef),
			STPct:      parseFloat(rec, iST),
		}
		k := key{season, team}
		buckets[k] = append(buckets[k], row)
	}

	schema := parquet.SchemaOf(new(SnapCountsRow))
	total := 0
	for k, part := range buckets {
		key := fmt.Sprintf("%s/snap_counts/season=%s/team=%s/part-%s.parquet", prefix, k.season, k.team, nowStamp())
		if err := writeParquetAndUpload(ctx, part, key, schema, up); err != nil {
			return total, err
		}
		total += len(part)
	}
	return total, nil
}

/* ---------- Plan + fetch/write ---------- */

type FetchPlan struct {
	Dataset   string
	Season    int
	URL       string
	AssetName string
	Format    string // "csv" or "parquet"
}

func buildFetchPlans(ctx context.Context, datasets []string, season int) ([]FetchPlan, error) {
	// Prefer CSV so we can partition on write. (You can flip to "parquet,csv" later.)
	prefer := strings.Split(strings.ToLower(strings.TrimSpace(getenv("NFLVERSE_FORMAT", "csv,parquet"))), ",")
	var plans []FetchPlan
	for _, ds := range datasets {
		ds = strings.TrimSpace(ds)
		if ds == "" {
			continue
		}
		url, name, err := nflverse.ResolveAssetURL(ctx, ds, season, prefer)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ds, err)
		}
		ext := "bin"
		for _, p := range prefer {
			if strings.HasSuffix(strings.ToLower(name), "."+p) || strings.Contains(strings.ToLower(name), "."+p+".") {
				ext = p
				break
			}
		}
		log.Printf("resolved asset dataset=%s season=%d asset=%s format=%s", ds, season, name, ext)
		plans = append(plans, FetchPlan{Dataset: ds, Season: season, URL: url, AssetName: name, Format: ext})
	}
	return plans, nil
}

func (h *Handler) fetchAndWrite(ctx context.Context, p FetchPlan) (int64, error) {
	up := &s3uploader{cl: h.S3, bucket: h.Bucket}
	switch strings.ToLower(p.Dataset) {
	case "players":
		if p.Format != "csv" {
			// best-effort raw store (you could add parquet-read+repartition later)
			b, err := httpGet(ctx, p.URL)
			if err != nil {
				return 0, err
			}
			key := fmt.Sprintf("%s/raw/players/%s", h.Prefix, p.AssetName)
			return int64(len(b)), up.put(ctx, key, b)
		}
		n, err := ingestPlayers(ctx, up, h.Prefix, p.URL)
		return int64(n), err

	case "rosters_weekly":
		if p.Format != "csv" {
			b, err := httpGet(ctx, p.URL)
			if err != nil {
				return 0, err
			}
			key := fmt.Sprintf("%s/raw/rosters_weekly/%s", h.Prefix, p.AssetName)
			return int64(len(b)), up.put(ctx, key, b)
		}
		n, err := ingestRostersWeekly(ctx, up, h.Prefix, p.URL)
		return int64(n), err

	case "snap_counts":
		if p.Format != "csv" {
			b, err := httpGet(ctx, p.URL)
			if err != nil {
				return 0, err
			}
			key := fmt.Sprintf("%s/raw/snap_counts/%s", h.Prefix, p.AssetName)
			return int64(len(b)), up.put(ctx, key, b)
		}
		n, err := ingestSnapCounts(ctx, up, h.Prefix, p.URL)
		return int64(n), err
	default:
		return 0, fmt.Errorf("unknown dataset %q", p.Dataset)
	}
}

/* ---------- Lambda entry ---------- */

func handler(ctx context.Context, e Event) (any, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	bucket := getenv("CURATED_BUCKET", "")
	if bucket == "" {
		return nil, errors.New("CURATED_BUCKET is required")
	}
	prefix := strings.Trim(getenv("CURATED_PREFIX", "nflverse_curated"), "/")

	season := e.Season
	if season == 0 {
		season, _ = strconv.Atoi(getenv("SEASON", "2024"))
	}
	datasets := e.Datasets
	if len(datasets) == 0 {
		datasets = strings.Split(getenv("DATASETS", "players,rosters_weekly,snap_counts"), ",")
	}

	h := &Handler{
		S3:     s3.NewFromConfig(awsCfg),
		Bucket: bucket,
		Prefix: prefix,
	}

	plans, err := buildFetchPlans(ctx, datasets, season)
	if err != nil {
		return nil, err
	}

	stats := map[string]int64{}
	var total int64
	for _, p := range plans {
		w, err := h.fetchAndWrite(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p.Dataset, err)
		}
		stats[p.Dataset] = w
		total += w
	}

	return map[string]any{
		"ok":       true,
		"season":   season,
		"datasets": datasets,
		"written":  stats,
		"s3":       fmt.Sprintf("s3://%s/%s/", bucket, prefix),
	}, nil
}

// func main() {
// 	if os.Getenv("_LAMBDA_SERVER_PORT") != "" {
// 		lambda.Start(handler)
// 		return
// 	}
// 	// local test
// 	ctx := context.Background()
// 	out, err := handler(ctx, Event{Datasets: []string{"players", "rosters_weekly", "snap_counts"}, Season: 2024})
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	j, _ := json.MarshalIndent(out, "", "  ")
// 	fmt.Println(string(j))
// }

func main() {
	lambda.Start(handler)
}
