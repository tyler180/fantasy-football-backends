package pfr

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type SnapGameRow struct {
	Season     string
	Team       string
	Week       int
	PlayerID   string
	Player     string
	DefSnapPct float64 // 0..100
	Pos        string  // <- NEW
}

var (
	reDefWeek = regexp.MustCompile(`(?i)^(?:def|defense).*?pct[_-]?(\d{1,2})$`)
	wsRe      = regexp.MustCompile(`\s+`)
	trimPct   = strings.NewReplacer("%", "", "\u00A0", "", "\u2009", "", ",", "")
)

func osBool(k string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	return v == "1" || v == "true" || v == "on" || v == "yes"
}

// FetchTeamDefSnapPctsByGame scrapes per-game DEF% for a team/season.
// It does not rely on headers; it reads week numbers from td[data-stat] names like def_pct_7.
func FetchTeamDefSnapPctsByGame(ctx context.Context, teamPath, teamAbbr, season, referer string) ([]SnapGameRow, error) {
	candidates := []string{
		fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s-snap-counts.htm", teamPath, season),
		fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s_snap_counts.htm", teamPath, season),
	}

	var html string
	var err error
	for i, url := range candidates {
		html, err = getTextWithUAWithRetry(ctx, url, referer)
		if err == nil && strings.Contains(html, "<table") {
			if osBool("DEBUG") {
				log.Printf("snaps: using url[%d]=%s", i, url)
			}
			break
		}
		if osBool("DEBUG") {
			log.Printf("snaps: candidate[%d] failed for %s: %v", i, url, err)
		}
		html = ""
	}
	if html == "" {
		return nil, fmt.Errorf("snap counts page not found for %s %s", teamAbbr, season)
	}

	// PFR often comments tables
	clean := strings.ReplaceAll(html, "<!--", "")
	clean = strings.ReplaceAll(clean, "-->", "")

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
	if err != nil {
		return nil, err
	}

	rows := make([]SnapGameRow, 0, 512)

	// Walk ALL table rows; look for a player cell + td[data-stat=def_pct_*]
	doc.Find("table tbody tr").Each(func(_ int, tr *goquery.Selection) {
		if strings.Contains(tr.AttrOr("class", ""), "thead") {
			return
		}

		// player cell can be th or td with data-stat="player"
		playerCell := tr.Find(`th[data-stat="player"], td[data-stat="player"]`).First()
		if playerCell.Length() == 0 {
			return
		}

		player := cleanPlayer(playerCell.Text())
		playerID := extractPlayerIDFromCell(playerCell)
		if player == "" || playerID == "" {
			return
		}

		// For each DEF weekly cell in this row, emit a record
		tr.Find("td").Each(func(_ int, td *goquery.Selection) {
			ds := strings.TrimSpace(td.AttrOr("data-stat", ""))
			if ds == "" {
				return
			}
			m := reDefWeek.FindStringSubmatch(ds)
			if m == nil {
				return
			}
			week, _ := strconv.Atoi(m[1])
			if week <= 0 || week > 22 {
				return
			}
			pct := parsePct(td.Text())
			rows = append(rows, SnapGameRow{
				Season:     season,
				Team:       teamAbbr,
				Week:       week,
				PlayerID:   playerID,
				Player:     player,
				DefSnapPct: pct,
			})
		})
	})

	if osBool("DEBUG") {
		log.Printf("snaps: built %d rows for %s %s", len(rows), teamAbbr, season)
	}
	return rows, nil
}

// ---- helpers ----

func extractPlayerIDFromCell(s *goquery.Selection) string {
	href, ok := s.Find("a").Attr("href")
	if !ok || href == "" {
		return ""
	}
	parts := strings.Split(href, "/")
	last := parts[len(parts)-1]
	return strings.TrimSuffix(last, ".htm")
}

func cleanPlayer(s string) string {
	return wsRe.ReplaceAllString(strings.TrimSpace(s), " ")
}

func parsePct(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "—" || s == "-" {
		return 0
	}
	s = trimPct.Replace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f // already a percentage value on PFR
}
