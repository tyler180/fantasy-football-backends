package nflverse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type releaseResp struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
}
type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

var datasetToTag = map[string]string{
	// left = input “dataset”; right = GitHub release tag
	"players":        "players",
	"rosters_weekly": "weekly_rosters",
	"snap_counts":    "snap_counts",
	// add others as needed, e.g. "player_stats": "player_stats"
}

// ResolveAssetURL finds an nflverse-data release asset that matches the dataset, season and preferred format.
func ResolveAssetURL(ctx context.Context, dataset string, season int, prefer []string) (string, string, error) {
	tag, ok := datasetToTag[dataset]
	if !ok {
		return "", "", fmt.Errorf("unknown dataset %q", dataset)
	}
	api := fmt.Sprintf("https://api.github.com/repos/nflverse/nflverse-data/releases/tags/%s", tag)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	// optional: to increase rate limits
	if tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("github api request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("github api status %d for %s", resp.StatusCode, api)
	}
	var r releaseResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return "", "", fmt.Errorf("decode release json: %w", err)
	}
	if len(r.Assets) == 0 {
		return "", "", fmt.Errorf("no assets for tag %s", tag)
	}

	year := fmt.Sprintf("%d", season)

	// scoring function: higher is better
	score := func(name string) int {
		s := 0
		// prefer assets that include the season explicitly
		if strings.Contains(name, year) {
			s += 10
		}
		// prefer “parquet” (or first in prefer list)
		for i, ext := range prefer {
			if strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(ext)) ||
				strings.Contains(strings.ToLower(name), "."+strings.ToLower(ext)+".") {
				s += 5 - i // earlier = more points
				break
			}
		}
		// slight boost if the dataset name is in the asset name
		if strings.Contains(strings.ToLower(name), strings.ToLower(dataset)) {
			s += 2
		}
		// slight boost if tag label pattern is present (e.g., "rosters_weekly"/"snap_counts")
		if strings.Contains(strings.ToLower(name), strings.ToLower(tag)) {
			s += 1
		}
		return s
	}

	type cand struct {
		Name, URL string
		Score     int
	}
	var cands []cand
	for _, a := range r.Assets {
		cands = append(cands, cand{
			Name:  a.Name,
			URL:   a.URL,
			Score: score(a.Name),
		})
	}
	// pick highest score; if tie, lexicographically stable
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Score == cands[j].Score {
			return cands[i].Name < cands[j].Name
		}
		return cands[i].Score > cands[j].Score
	})
	best := cands[0]
	if best.Score <= 0 {
		// fallback: pick any asset that ends with the preferred ext or any asset at all
		for _, a := range r.Assets {
			for _, ext := range prefer {
				if strings.HasSuffix(strings.ToLower(a.Name), "."+strings.ToLower(ext)) {
					return a.URL, a.Name, nil
				}
			}
		}
		// last resort: first asset
		return r.Assets[0].URL, r.Assets[0].Name, nil
	}
	return best.URL, best.Name, nil
}
