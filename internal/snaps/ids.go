package snaps

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchNflversePlayerIDs downloads the player ids map from nflverse.
// Returns two maps:
//
//	gsisID -> pfrID
//	fullName (UPPER, normalized) -> pfrID  (best-effort)
func FetchNflversePlayerIDs(ctx context.Context, url string) (map[string]string, map[string]string, error) {
	if url == "" {
		// Stable "latest release" URL pattern. You can override via env if needed.
		url = "https://github.com/nflverse/nflverse-data/releases/download/players/players.csv"
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	// GitHub raw may require a UA
	req.Header.Set("User-Agent", "pfr-snaps/1.0 (+https://github.com)")
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 5120))
		return nil, nil, fmt.Errorf("ids fetch %s: status %d body=%q", url, res.StatusCode, string(body))
	}

	cr := csv.NewReader(res.Body)
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, nil, err
	}
	col := func(name string) int {
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), name) {
				return i
			}
		}
		return -1
	}

	ciGSIS := col("gsis_id")
	ciPfrID := col("pfr_player_id")
	if ciPfrID < 0 {
		ciPfrID = col("pfr_id") // fallback
	}
	ciName := col("full_name")

	byGSIS := make(map[string]string, 5000)
	byName := make(map[string]string, 5000)

	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return byGSIS, byName, nil // partial ok
		}
		gs := safeGet(rec, ciGSIS)
		pfr := strings.TrimSpace(safeGet(rec, ciPfrID))
		name := strings.TrimSpace(safeGet(rec, ciName))

		if pfr == "" {
			continue
		}
		pfr = normalizePfrID(pfr)
		if gs != "" {
			byGSIS[gs] = pfr
		}
		if name != "" {
			byName[normName(name)] = pfr
		}
	}
	return byGSIS, byName, nil
}

func safeGet(rec []string, idx int) string {
	if idx < 0 || idx >= len(rec) {
		return ""
	}
	return rec[idx]
}

func normalizePfrID(id string) string {
	// Accept "W/WattJJ00" or "WattJJ00" -> "WattJJ00"
	id = strings.TrimSpace(id)
	id = strings.TrimPrefix(id, "/players/")
	if n := strings.LastIndexByte(id, '/'); n >= 0 {
		id = id[n+1:]
	}
	return id
}

func normName(s string) string {
	// same normalizer as store.normName, local copy to avoid circular import
	repl := strings.NewReplacer(
		".", "", ",", "", "'", "", "`", "", "’", "",
		"-", " ", "–", " ", "—", " ",
		"(", "", ")", "",
	)
	s = repl.Replace(strings.ToUpper(strings.TrimSpace(s)))
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
