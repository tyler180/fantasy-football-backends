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

// URL path → display abbr (used as the team on each roster page)
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

// -------------------- env + retry tunables --------------------

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return def
}

// base per-team delay + small jitter to avoid burst alignment
func teamDelay() time.Duration {
	ms := envInt("TEAM_DELAY_MS", 300)
	if ms < 0 {
		ms = 0
	}
	if ms > 5000 {
		ms = 5000
	}
	// ±100ms jitter
	j := rand.Intn(201) - 100
	d := time.Duration(ms+j) * time.Millisecond
	if d < 0 {
		d = 0
	}
	return d
}

func retryConfig() (maxAttempts int, base, maxBackoff, cooldown time.Duration) {
	maxAttempts = envInt("HTTP_MAX_ATTEMPTS", 6)                                     // attempts per request
	base = time.Duration(envInt("HTTP_RETRY_BASE_MS", 400)) * time.Millisecond       // base backoff
	maxBackoff = time.Duration(envInt("HTTP_RETRY_MAX_MS", 6000)) * time.Millisecond // cap per-attempt backoff
	cooldown = time.Duration(envInt("HTTP_COOLDOWN_MS", 7000)) * time.Millisecond    // used on 429 when no Retry-After
	return
}

func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	// seconds form
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	// HTTP date
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func backoff(attempt int, base, max time.Duration) time.Duration {
	// exponential + jitter, capped
	d := base * time.Duration(1<<attempt)
	j := time.Duration(rand.Intn(250)) * time.Millisecond
	if d+j > max {
		return max
	}
	return d + j
}

// getTextWithUAWithRetry fetches a URL with UA/headers and retries on 429/5xx.
// Respects Retry-After when present.
func getTextWithUAWithRetry(ctx context.Context, url, referer string) (string, error) {
	maxAttempts, base, maxBackoff, cooldown := retryConfig()

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
			time.Sleep(backoff(attempt, base, maxBackoff))
			continue
		}

		defer resp.Body.Close()
		// func() { defer resp.Body.Close() }()

		if resp.StatusCode == 200 {
			b, e := io.ReadAll(resp.Body)
			if e != nil {
				if attempt == maxAttempts-1 {
					return "", e
				}
				time.Sleep(backoff(attempt, base, maxBackoff))
				continue
			}
			return string(b), nil
		}

		if resp.StatusCode == 429 {
			// Respect Retry-After if provided; otherwise use configured cooldown
			sleep := parseRetryAfter(resp.Header.Get("Retry-After"))
			if sleep == 0 {
				sleep = cooldown
			}
			time.Sleep(sleep)
			continue
		}

		if resp.StatusCode >= 500 && resp.StatusCode <= 599 {
			time.Sleep(backoff(attempt, base, maxBackoff))
			continue
		}

		// Non-retryable
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("status %d for %s (body len=%d)", resp.StatusCode, url, len(b))
	}
	return "", fmt.Errorf("exhausted retries for %s", url)
}

// -------------------- roster/snap parsing helpers --------------------

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

// func extractPlayerIDFromCell(cell *goquery.Selection) string {
// 	id := ""
// 	cell.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
// 		if href, ok := a.Attr("href"); ok {
// 			// Example: /players/A/AlleNi00.htm
// 			if strings.HasPrefix(href, "/players/") && strings.HasSuffix(href, ".htm") {
// 				parts := strings.Split(href, "/")
// 				last := parts[len(parts)-1]
// 				id = strings.TrimSuffix(last, ".htm")
// 				return false
// 			}
// 		}
// 		return true
// 	})
// 	// Fallback (very defensive): sanitized text
// 	if id == "" {
// 		txt := strings.ToLower(strings.TrimSpace(cell.Text()))
// 		txt = strings.ReplaceAll(txt, " ", "")
// 		txt = strings.ReplaceAll(txt, ".", "")
// 		txt = strings.ReplaceAll(txt, "'", "")
// 		if txt != "" {
// 			id = txt
// 		}
// 	}
// 	return id
// }

// ---------- snap counts ----------

type SnapCounts struct {
	DefNum int
	DefPct float64
}

type snapHeaderMap struct {
	idxPlayer int
	idxDefNum int
	idxDefPct int
}

// func parsePct(s string) float64 {
// 	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
// 	if s == "" {
// 		return 0
// 	}
// 	f, _ := strconv.ParseFloat(s, 64)
// 	return f
// }

func mapSnapHeader(table *goquery.Selection) (snapHeaderMap, bool) {
	h := snapHeaderMap{-1, -1, -1}
	thead := table.Find("thead tr").Last()
	if thead.Length() == 0 {
		return h, false
	}
	thead.Find("th,td").Each(func(i int, cell *goquery.Selection) {
		txt := normHeader(cell.Text())
		switch {
		case txt == "player":
			h.idxPlayer = i
		case strings.Contains(txt, "def") && strings.Contains(txt, "num"):
			h.idxDefNum = i
		case strings.Contains(txt, "def") && (strings.Contains(txt, "pct") || strings.Contains(txt, "percent")):
			h.idxDefPct = i
		}
	})
	ok := h.idxPlayer >= 0 && (h.idxDefNum >= 0 || h.idxDefPct >= 0)
	return h, ok
}

func findSnapTable(doc *goquery.Document) *goquery.Selection {
	if t := doc.Find(`table#snap_counts, table#snap_counts_d, table#snap_counts_defense`); t.Length() > 0 {
		return t.First()
	}
	var chosen *goquery.Selection
	doc.Find("table").EachWithBreak(func(_ int, t *goquery.Selection) bool {
		h, ok := mapSnapHeader(t)
		if ok && h.idxPlayer >= 0 && (h.idxDefNum >= 0 || h.idxDefPct >= 0) {
			chosen = t
			return false
		}
		return true
	})
	if chosen != nil {
		return chosen
	}
	return doc.Find("table").Slice(0, 0)
}

func fetchTeamSnapCounts(ctx context.Context, teamPath, season, referer string) (map[string]SnapCounts, error) {
	url := fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s_snap_counts.htm", teamPath, season)
	html, err := getTextWithUAWithRetry(ctx, url, referer)
	if err != nil {
		return nil, err
	}

	clean := strings.ReplaceAll(html, "<!--", "")
	clean = strings.ReplaceAll(clean, "-->", "")

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
	if err != nil {
		return nil, err
	}

	table := findSnapTable(doc)
	if table.Length() == 0 {
		return map[string]SnapCounts{}, nil
	}
	hdr, ok := mapSnapHeader(table)
	if !ok {
		return map[string]SnapCounts{}, nil
	}

	out := map[string]SnapCounts{}
	rows := table.Find("tbody tr")
	if rows.Length() == 0 {
		rows = table.Find("tr")
	}
	rows.Each(func(_ int, tr *goquery.Selection) {
		if strings.Contains(tr.AttrOr("class", ""), "thead") {
			return
		}
		cells := tr.Find("th,td")
		if cells.Length() == 0 {
			return
		}
		playerCell := cells.Eq(hdr.idxPlayer)
		playerID := extractPlayerIDFromCell(playerCell)
		if playerID == "" {
			return
		}
		defNum := 0
		defPct := 0.0
		if hdr.idxDefNum >= 0 && hdr.idxDefNum < cells.Length() {
			defNum = Atoi(strings.TrimSpace(cells.Eq(hdr.idxDefNum).Text()), 0)
		}
		if hdr.idxDefPct >= 0 && hdr.idxDefPct < cells.Length() {
			defPct = parsePct(cells.Eq(hdr.idxDefPct).Text())
		}
		out[playerID] = SnapCounts{DefNum: defNum, DefPct: defPct}
	})
	return out, nil
}

// -------------------- team subset (chunking / explicit list) --------------------

type tcode struct{ Path, Abbr string }

func allTeams() []tcode {
	ts := make([]tcode, len(teamCodes))
	for i, t := range teamCodes {
		ts[i] = tcode{t.Path, t.Abbr}
	}
	return ts
}

func applyTeamSubset(all []tcode) []tcode {
	debug := os.Getenv("DEBUG") == "1"

	// Explicit list wins (abbr or path, comma-separated)
	if lst := strings.TrimSpace(os.Getenv("TEAM_LIST")); lst != "" {
		want := make(map[string]struct{})
		for _, tok := range strings.Split(lst, ",") {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			want[strings.ToUpper(tok)] = struct{}{}
			want[strings.ToLower(tok)] = struct{}{}
		}
		sub := make([]tcode, 0, len(all))
		for _, t := range all {
			if _, ok := want[strings.ToUpper(t.Abbr)]; ok {
				sub = append(sub, t)
				continue
			}
			if _, ok := want[strings.ToLower(t.Path)]; ok {
				sub = append(sub, t)
				continue
			}
		}
		if debug {
			log.Printf("DEBUG team subset via TEAM_LIST=%q -> %d teams", lst, len(sub))
		}
		return sub
	}

	// Otherwise: chunking by index/total
	total := envInt("TEAM_CHUNK_TOTAL", 1)
	index := envInt("TEAM_CHUNK_INDEX", 0)
	if total <= 1 {
		return all
	}
	if index < 0 {
		index = 0
	}
	if index >= total {
		index = total - 1
	}

	size := (len(all) + total - 1) / total // ceil
	start := index * size
	if start >= len(all) {
		if debug {
			log.Printf("DEBUG team subset chunk=%d/%d -> 0 teams", index, total)
		}
		return nil
	}
	end := start + size
	if end > len(all) {
		end = len(all)
	}
	sub := all[start:end]
	if debug {
		abbrs := make([]string, 0, len(sub))
		for _, t := range sub {
			abbrs = append(abbrs, t.Abbr)
		}
		log.Printf("DEBUG team subset chunk=%d/%d -> %d teams: %s", index, total, len(sub), strings.Join(abbrs, ","))
	}
	return sub
}

// -------------------- main entry: scrape roster rows --------------------

// FetchSeasonRosterRows scrapes each team's roster page, merges in snap counts,
// and supports TEAM_LIST / TEAM_CHUNK_{TOTAL,INDEX} to limit scope.
func FetchSeasonRosterRows(ctx context.Context, season string) ([]RosterRow, error) {
	debug := os.Getenv("DEBUG") == "1"
	referer := fmt.Sprintf("https://www.pro-football-reference.com/years/%s/", season)
	fetchSnaps := os.Getenv("SNAP_COUNTS") != "0"

	// Build subset first (keeps chunk stable), then optional shuffle within the subset
	pending := applyTeamSubset(allTeams())
	if len(pending) == 0 {
		if debug {
			log.Printf("DEBUG roster: no teams selected (check TEAM_LIST or TEAM_CHUNK_* envs)")
		}
		return nil, nil
	}
	if os.Getenv("SHUFFLE_TEAMS") == "1" {
		rand.Shuffle(len(pending), func(i, j int) { pending[i], pending[j] = pending[j], pending[i] })
	}

	out := make([]RosterRow, 0, 900)

	// Multi-pass retry on teams (rate-limit friendly)
	passMax := envInt("PASS_MAX", 3)
	baseCooldown := time.Duration(envInt("HTTP_FINAL_COOLDOWN_MS", 12000)) * time.Millisecond

	for pass := 1; pass <= passMax && len(pending) > 0; pass++ {
		failed := make([]tcode, 0, 4)
		if debug {
			log.Printf("DEBUG roster: pass %d starting for %d teams", pass, len(pending))
		}

		for _, t := range pending {
			rosterURL := fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s_roster.htm", t.Path, season)
			if debug {
				log.Printf("DEBUG roster: GET %s", rosterURL)
			}

			html, err := getTextWithUAWithRetry(ctx, rosterURL, referer)
			if err != nil {
				if debug {
					log.Printf("DEBUG roster: fetch %s failed: %v", rosterURL, err)
				}
				failed = append(failed, t)
				time.Sleep(teamDelay())
				continue
			}

			clean := strings.ReplaceAll(html, "<!--", "")
			clean = strings.ReplaceAll(clean, "-->", "")

			doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
			if err != nil {
				if debug {
					log.Printf("DEBUG roster: parse %s failed: %v", rosterURL, err)
				}
				failed = append(failed, t)
				time.Sleep(teamDelay())
				continue
			}

			DumpTablesForDebug(doc, t.Abbr)

			table := doc.Find("table#roster").First()
			if table.Length() == 0 {
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
				failed = append(failed, t)
				time.Sleep(teamDelay())
				continue
			}

			hdr, ok := mapRosterHeader(table)
			if !ok {
				if debug {
					log.Printf("DEBUG roster: header mapping failed for %s", t.Abbr)
				}
				failed = append(failed, t)
				time.Sleep(teamDelay())
				continue
			}

			// Optional: fetch snap counts for this team
			var snaps map[string]SnapCounts
			if fetchSnaps {
				if debug {
					log.Printf("DEBUG snapcounts: GET %s/%s", t.Abbr, season)
				}
				snaps, _ = fetchTeamSnapCounts(ctx, t.Path, season, referer) // tolerate empty/err; merge if present
				time.Sleep(teamDelay())
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

				r := RosterRow{
					Season:   season,
					PlayerID: playerID,
					Player:   player,
					Team:     t.Abbr,
					Age:      age,
					Pos:      pos,
					G:        g,
					GS:       gs,
				}
				if sc, ok := snaps[playerID]; ok {
					r.DefSnapNum = sc.DefNum
					r.DefSnapPct = sc.DefPct
				}

				out = append(out, r)
				count++
			})
			if debug {
				log.Printf("DEBUG roster: %s parsed rows=%d", t.Abbr, count)
			}
			time.Sleep(teamDelay())
		}

		if len(failed) == 0 {
			break
		}
		if pass < passMax {
			// Escalate cooldown by pass number (1x, 2x, 3x …)
			cool := time.Duration(pass) * baseCooldown
			if debug {
				abbrs := make([]string, 0, len(failed))
				for _, t := range failed {
					abbrs = append(abbrs, t.Abbr)
				}
				log.Printf("DEBUG roster: cooldown %v before next pass (retrying %d teams: %s)", cool, len(failed), strings.Join(abbrs, ","))
			}
			time.Sleep(cool)
		}
		pending = failed
	}

	// Stable order for reproducibility
	sort.Slice(out, func(i, j int) bool {
		if out[i].Team == out[j].Team {
			return out[i].Player < out[j].Player
		}
		return out[i].Team < out[j].Team
	})
	return out, nil
}

// var wkRe = regexp.MustCompile(`^(wk\.?\s*)?(\d{1,2})$`)

// func FetchTeamDefSnapPctsByGame(ctx context.Context, teamPath, teamAbbr, season, referer string) ([]SnapGameRow, error) {
// 	url := fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s-snap-counts.htm", teamPath, season)
// 	html, err := getTextWithUAWithRetry(ctx, url, referer)
// 	if err != nil {
// 		return nil, err
// 	}

// 	clean := strings.ReplaceAll(html, "<!--", "")
// 	clean = strings.ReplaceAll(clean, "-->", "")

// 	doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
// 	if err != nil {
// 		return nil, err
// 	}

// 	table := findSnapTable(doc)
// 	if table.Length() == 0 {
// 		return nil, nil // no snap table found
// 	}

// 	// Map columns: player + def% + weekly def% (if present)
// 	headerIdx := map[string]int{}
// 	weekCols := []int{}
// 	thead := table.Find("thead tr").Last()
// 	thead.Find("th,td").Each(func(i int, th *goquery.Selection) {
// 		lbl := strings.ToLower(strings.TrimSpace(th.Text()))
// 		lbl = strings.ReplaceAll(lbl, ".", "")
// 		switch {
// 		case lbl == "player":
// 			headerIdx["player"] = i
// 		case strings.Contains(lbl, "def") && (strings.Contains(lbl, "pct") || strings.Contains(lbl, "percent")):
// 			headerIdx["defpct_total"] = i
// 		default:
// 			// Detect week columns
// 			s := strings.ToLower(strings.TrimSpace(th.Text()))
// 			s = strings.ReplaceAll(s, ".", "")
// 			s = strings.ReplaceAll(s, "wk", "wk ")
// 			s = strings.TrimSpace(s)
// 			m := wkRe.FindStringSubmatch(s)
// 			if len(m) == 3 {
// 				if _, err := strconv.Atoi(m[2]); err == nil {
// 					weekCols = append(weekCols, i)
// 				}
// 			}
// 		}
// 	})

// 	if _, ok := headerIdx["player"]; !ok {
// 		return nil, fmt.Errorf("snap table: no player column")
// 	}

// 	out := make([]SnapGameRow, 0, 256)
// 	rows := table.Find("tbody tr")
// 	if rows.Length() == 0 {
// 		rows = table.Find("tr")
// 	}

// 	rows.Each(func(_ int, tr *goquery.Selection) {
// 		if strings.Contains(tr.AttrOr("class", ""), "thead") {
// 			return
// 		}
// 		cells := tr.Find("th,td")
// 		if cells.Length() == 0 {
// 			return
// 		}

// 		playerCell := cells.Eq(headerIdx["player"])
// 		player := cleanPlayer(playerCell.Text())
// 		playerID := extractPlayerIDFromCell(playerCell)
// 		if player == "" || playerID == "" {
// 			return
// 		}

// 		// Weekly values if present
// 		if len(weekCols) > 0 {
// 			for _, ci := range weekCols {
// 				if ci >= cells.Length() {
// 					continue
// 				}
// 				raw := strings.TrimSpace(cells.Eq(ci).Text())
// 				// Treat blank/DNP as 0.0
// 				pct := parsePct(raw)
// 				// If the cell is numeric only (no % sign), parsePct handles it (we store as % value)
// 				out = append(out, SnapGameRow{
// 					Season: season, Team: teamAbbr,
// 					Week:     inferWeekFromHeader(thead, ci),
// 					PlayerID: playerID, Player: player,
// 					DefSnapPct: pct,
// 				})
// 			}
// 		}
// 	})

// 	return out, nil
// }

// inferWeekFromHeader reads the header cell text at column ci and returns an int week
func inferWeekFromHeader(thead *goquery.Selection, ci int) int {
	lbl := thead.Find("th,td").Eq(ci).Text()
	lbl = strings.ToLower(strings.TrimSpace(lbl))
	lbl = strings.ReplaceAll(lbl, ".", "")
	lbl = strings.ReplaceAll(lbl, "wk", "")
	lbl = strings.TrimSpace(lbl)
	wk, _ := strconv.Atoi(lbl)
	return wk
}
