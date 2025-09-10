package curator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/tyler180/fantasy-football-backends/tools/nflverse-curator/internal/nflverse"
)

type FetchPlan struct {
	Dataset   string
	Season    int
	URL       string
	AssetName string
	Format    string // parquet or csv
}

func buildFetchPlans(ctx context.Context, datasets []string, season int) ([]FetchPlan, error) {
	prefer := strings.Split(strings.ToLower(strings.TrimSpace(getEnv("NFLVERSE_FORMAT", "parquet,csv"))), ",")
	var plans []FetchPlan
	for _, ds := range datasets {
		url, name, err := nflverse.ResolveAssetURL(ctx, ds, season, prefer)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", ds, err)
		}
		ext := "bin"
		for _, p := range prefer {
			if strings.Contains(strings.ToLower(name), "."+p) {
				ext = p
				break
			}
		}
		slog.Info("resolved asset", "dataset", ds, "season", season, "asset", name, "url", url, "format", ext)
		plans = append(plans, FetchPlan{
			Dataset: ds, Season: season, URL: url, AssetName: name, Format: ext,
		})
	}
	return plans, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Parse season from env (fallback)
func SeasonFromEnv() int {
	s := getEnv("SEASON", "")
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}
