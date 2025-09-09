package snaps

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type SnapRow struct {
	Season     int
	Week       int
	Team       string
	Opponent   string
	Player     string
	PlayerID   string // pfr_player_id
	Position   string // <- NEW
	DefensePct float64
}

func FetchNflverseSnapCounts(ctx context.Context, season int, teamFilter map[string]struct{}) ([]SnapRow, error) {
	url := fmt.Sprintf("https://github.com/nflverse/nflverse-data/releases/download/snap_counts/snap_counts_%d.csv", season)

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "pfr-snaps/1.1 (+https://example.com)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get snap_counts csv: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("snap_counts download %s: %s (%s)", url, resp.Status, string(b))
	}

	r := csv.NewReader(resp.Body)
	r.FieldsPerRecord = -1

	// header -> index map
	hdr, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	idx := func(name string) int {
		for i, h := range hdr {
			if strings.EqualFold(h, name) {
				return i
			}
		}
		return -1
	}

	iSeason := idx("season")
	iWeek := idx("week")
	iTeam := idx("team")
	iOpp := idx("opponent")
	iPlayer := idx("player")
	iPfrID := idx("pfr_player_id")
	iPos := idx("position") // <- NEW
	iDefPct := idx("defense_pct")

	if iSeason < 0 || iWeek < 0 || iTeam < 0 || iPlayer < 0 || iPfrID < 0 || iDefPct < 0 {
		return nil, fmt.Errorf("required columns missing (need season, week, team, player, pfr_player_id, defense_pct)")
	}

	rows := make([]SnapRow, 0, 50000)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}

		s, _ := strconv.Atoi(rec[iSeason])
		if s != season {
			continue
		}
		team := strings.ToUpper(rec[iTeam])
		if len(teamFilter) > 0 {
			if _, ok := teamFilter[team]; !ok {
				continue
			}
		}

		w, _ := strconv.Atoi(rec[iWeek])
		dpct := 0.0
		if rec[iDefPct] != "" {
			dpct, _ = strconv.ParseFloat(rec[iDefPct], 64)
		}

		pos := ""
		if iPos >= 0 {
			pos = strings.ToUpper(strings.TrimSpace(rec[iPos]))
		}

		rows = append(rows, SnapRow{
			Season:     s,
			Week:       w,
			Team:       team,
			Opponent:   rec[iOpp],
			Player:     rec[iPlayer],
			PlayerID:   rec[iPfrID],
			Position:   pos,
			DefensePct: dpct,
		})
	}
	return rows, nil
}
