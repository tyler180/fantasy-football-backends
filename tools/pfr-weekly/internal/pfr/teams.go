package pfr

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var httpCliTeams = &http.Client{Timeout: 30 * time.Second}

// URL path → display abbr (used when roster table omits team col)
var teamCodes = []struct {
	Path string
	Abbr string
}{
	{"crd", "ARI"}, {"atl", "ATL"}, {"rav", "BAL"}, {"buf", "BUF"},
	{"car", "CAR"}, {"chi", "CHI"}, {"cin", "CIN"}, {"cle", "CLE"},
	{"dal", "DAL"}, {"den", "DEN"}, {"det", "DET"}, {"gnb", "GNB"},
	{"htx", "HOU"}, {"clt", "IND"}, {"jax", "JAX"}, {"kan", "KAN"},
	{"rai", "LVR"}, {"sdg", "LAC"}, {"ram", "LAR"}, {"mia", "MIA"},
	{"min", "MIN"}, {"nwe", "NWE"}, {"nor", "NOR"}, {"nyg", "NYG"},
	{"nyj", "NYJ"}, {"phi", "PHI"}, {"pit", "PIT"}, {"sfo", "SFO"},
	{"sea", "SEA"}, {"tam", "TAM"}, {"oti", "TEN"}, {"was", "WAS"},
}

// polite delay between team requests (env TEAM_DELAY_MS), default 300ms
func teamDelay() time.Duration {
	ms := 300
	if v, err := strconv.Atoi(os.Getenv("TEAM_DELAY_MS")); err == nil && v >= 0 && v <= 5000 {
		ms = v
	}
	return time.Duration(ms) * time.Millisecond
}

// fetch with UA, Accept-Language + optional Referer, retries on 429/5xx with jittered backoff
func getTextWithUAWithRetry(ctx context.Context, url, referer string) (string, error) {
	maxAttempts := 4
	base := 250 * time.Millisecond
	for attempt := 0; attempt < maxAttempts; attempt++ {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		resp, err := httpCliTeams.Do(req)
		if err != nil {
			if attempt == maxAttempts-1 {
				return "", err
			}
		} else {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				b, e := io.ReadAll(resp.Body)
				if e != nil {
					if attempt == maxAttempts-1 {
						return "", e
					}
				} else {
					return string(b), nil
				}
			}
			// retry only on 429/5xx
			if resp.StatusCode != 429 && (resp.StatusCode < 500 || resp.StatusCode > 599) {
				b, _ := io.ReadAll(resp.Body)
				return "", fmt.Errorf("status %d for %s (body len=%d)", resp.StatusCode, url, len(b))
			}
		}
		sleep := base*time.Duration(1<<attempt) + time.Duration(rand.Intn(200))*time.Millisecond
		time.Sleep(sleep)
	}
	return "", fmt.Errorf("exhausted retries for %s", url)
}

func normHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, "|", " ")
	return strings.TrimSpace(s)
}

type rosterHeaderMap struct {
	idxPlayer int
	idxAge    int
	idxPos    int
	idxG      int
	idxGS     int
}

func mapRosterHeader(table *goquery.Selection) (rosterHeaderMap, bool) {
	h := rosterHeaderMap{-1, -1, -1, -1, -1}
	thead := table.Find("thead tr").First()
	if thead.Length() == 0 {
		return h, false
	}
	thead.Find("th,td").Each(func(i int, cell *goquery.Selection) {
		txt := normHeader(cell.Text())
		switch txt {
		case "player":
			h.idxPlayer = i
		case "age":
			h.idxAge = i
		case "pos", "position":
			h.idxPos = i
		case "g", "games":
			h.idxG = i
		case "gs", "games started":
			h.idxGS = i
		}
	})
	ok := h.idxPlayer >= 0 && h.idxAge >= 0 && h.idxPos >= 0 && h.idxG >= 0
	return h, ok
}

func extractPlayerIDFromCell(cell *goquery.Selection) string {
	id := ""
	cell.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		if href, ok := a.Attr("href"); ok {
			// /players/A/AlleNi00.htm
			if strings.HasPrefix(href, "/players/") && strings.HasSuffix(href, ".htm") {
				parts := strings.Split(href, "/")
				last := parts[len(parts)-1]
				id = strings.TrimSuffix(last, ".htm")
				return false
			}
		}
		return true
	})
	// fallback (very defensive): sanitize player text
	if id == "" {
		txt := strings.ToLower(strings.TrimSpace(cell.Text()))
		txt = strings.ReplaceAll(txt, " ", "")
		txt = strings.ReplaceAll(txt, ".", "")
		txt = strings.ReplaceAll(txt, "'", "")
		if txt != "" {
			id = txt
		}
	}
	return id
}

// FetchSeasonRosterRows scrapes each team's roster page (#roster table) and returns raw rows.
func FetchSeasonRosterRows(ctx context.Context, season string) ([]RosterRow, error) {
	debug := os.Getenv("DEBUG") == "1"
	delay := teamDelay()
	referer := fmt.Sprintf("https://www.pro-football-reference.com/years/%s/", season)

	out := make([]RosterRow, 0, 1800)
	for _, t := range teamCodes {
		rosterURL := fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s_roster.htm", t.Path, season)
		if debug {
			log.Printf("DEBUG roster: GET %s", rosterURL)
		}
		html, err := getTextWithUAWithRetry(ctx, rosterURL, referer)
		if err != nil {
			if debug {
				log.Printf("DEBUG roster: fetch %s failed: %v", rosterURL, err)
			}
			time.Sleep(delay)
			continue
		}
		// de-comment
		clean := strings.ReplaceAll(html, "<!--", "")
		clean = strings.ReplaceAll(clean, "-->", "")

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
		if err != nil {
			if debug {
				log.Printf("DEBUG roster: parse %s failed: %v", rosterURL, err)
			}
			time.Sleep(delay)
			continue
		}

		// visibility
		DumpTablesForDebug(doc, t.Abbr)

		table := doc.Find("table#roster").First()
		if table.Length() == 0 {
			// header-based fallback: choose first table with player+g
			doc.Find("table").EachWithBreak(func(_ int, cand *goquery.Selection) bool {
				hdr, ok := mapRosterHeader(cand)
				if ok && hdr.idxG >= 0 {
					table = cand
					return false
				}
				return true
			})
		}
		if table.Length() == 0 {
			if debug {
				log.Printf("DEBUG roster: no roster table for %s", t.Abbr)
			}
			time.Sleep(delay)
			continue
		}

		hdr, ok := mapRosterHeader(table)
		if !ok {
			if debug {
				log.Printf("DEBUG roster: header mapping failed for %s", t.Abbr)
			}
			time.Sleep(delay)
			continue
		}

		rows := table.Find("tbody tr")
		if rows.Length() == 0 {
			rows = table.Find("tr")
		}
		count := 0
		rows.Each(func(_ int, tr *goquery.Selection) {
			if strings.Contains(tr.AttrOr("class", ""), "thead") {
				return
			}
			cells := tr.Find("th,td")
			if cells.Length() == 0 {
				return
			}
			get := func(idx int) string {
				if idx < 0 || idx >= cells.Length() {
					return ""
				}
				return strings.TrimSpace(cells.Eq(idx).Text())
			}

			playerCell := cells.Eq(hdr.idxPlayer)
			player := cleanPlayer(playerCell.Text())
			if player == "" {
				return
			}
			playerID := extractPlayerIDFromCell(playerCell)

			age := Atoi(get(hdr.idxAge), 0)
			pos := get(hdr.idxPos)
			g := Atoi(get(hdr.idxG), 0)
			gs := 0
			if hdr.idxGS >= 0 {
				gs = Atoi(get(hdr.idxGS), 0)
			}

			out = append(out, RosterRow{
				Season:   season,
				PlayerID: playerID,
				Player:   player,
				Team:     t.Abbr,
				Age:      age,
				Pos:      pos,
				G:        g,
				GS:       gs,
			})
			count++
		})
		if debug {
			log.Printf("DEBUG roster: %s parsed rows=%d", t.Abbr, count)
		}
		time.Sleep(delay)
	}

	// keep consistent order (optional)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Team == out[j].Team {
			return out[i].Player < out[j].Player
		}
		return out[i].Team < out[j].Team
	})
	return out, nil
}
