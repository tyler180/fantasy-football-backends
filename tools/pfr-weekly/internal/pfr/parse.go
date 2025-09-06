package pfr

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func ParseAndFilterCSV(r io.Reader, posTokens []string, maxAge int) ([]PlayerRow, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	all, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, errors.New("empty CSV")
	}

	hdrIdx := detectHeaderRow(all)
	if hdrIdx < 0 {
		return nil, errors.New("could not detect header row")
	}

	h := normalizeHeaders(all[hdrIdx])
	idxPlayer := findColumn(h, "Player")
	idxAge := findColumn(h, "Age")
	idxG := findColumn(h, "G")
	idxGS := findColumn(h, "GS")
	idxPos := findColumn(h, "Pos")
	idxTeam := findColumnPrefer(h, []string{"Team", "Tm"})
	if idxPlayer < 0 || idxAge < 0 || idxG < 0 || idxGS < 0 || idxPos < 0 {
		return nil, errors.New("missing required columns")
	}

	type agg struct {
		Player string
		AgeMin int
		GSum   int
		GSSum  int
		PosSet map[string]struct{}
		TeamGS map[string]int
		TeamG  map[string]int
		Teams  map[string]struct{}
	}
	ag := map[string]*agg{}

	for i := hdrIdx + 1; i < len(all); i++ {
		cols := all[i]
		if len(cols) == 0 {
			continue
		}
		if strings.TrimSpace(cols[0]) == "Rk" {
			continue
		}

		get := func(idx int) string {
			if idx >= 0 && idx < len(cols) {
				return cols[idx]
			}
			return ""
		}

		player := cleanPlayer(get(idxPlayer))
		if player == "" || player == "Player" {
			continue
		}

		pos := strings.TrimSpace(get(idxPos))
		if !isPositionMatch(posTokens, pos) {
			continue
		}

		var tm string
		if idxTeam >= 0 {
			tm = strings.ToUpper(strings.TrimSpace(get(idxTeam)))
			if tm == "TOT" || strings.HasSuffix(tm, "TM") { // 2TM/3TM/TOT
				tm = ""
				continue
			}
		}

		age := Atoi(get(idxAge), 0)
		g := Atoi(get(idxG), 0)
		gs := Atoi(get(idxGS), 0)

		a := ag[player]
		if a == nil {
			a = &agg{
				Player: player,
				AgeMin: ifZeroThenLarge(age),
				PosSet: map[string]struct{}{},
				TeamGS: map[string]int{},
				TeamG:  map[string]int{},
				Teams:  map[string]struct{}{},
			}
			ag[player] = a
		}
		if age > 0 && age < a.AgeMin {
			a.AgeMin = age
		}
		a.GSum += g
		a.GSSum += gs
		if p := strings.TrimSpace(pos); p != "" {
			a.PosSet[p] = struct{}{}
		}
		if tm != "" {
			a.TeamGS[tm] += gs
			a.TeamG[tm] += g
			a.Teams[tm] = struct{}{}
		}
	}

	var out []PlayerRow
	for _, a := range ag {
		age := a.AgeMin
		if age == 1<<30 {
			age = 0
		}
		if age <= maxAge && a.GSum > 0 && a.GSum == a.GSSum {
			out = append(out, PlayerRow{
				Player: a.Player,
				Team:   pickPrimaryTeam(a.TeamGS, a.TeamG),
				Teams:  joinSortedKeys(a.Teams, ","),
				Age:    age,
				G:      a.GSum,
				GS:     a.GSSum,
				Pos:    joinSortedKeys(a.PosSet, ","),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Player < out[j].Player })
	return out, nil
}

// ---------- helpers (unexported) ----------

func detectHeaderRow(rows [][]string) int {
	for i, r := range rows {
		if len(r) == 0 {
			continue
		}
		if strings.TrimSpace(stripBOM(r[0])) != "Rk" {
			continue
		}
		h := normalizeHeaders(r)
		if hasAll(h, []string{"Player", "Age", "G", "GS"}) {
			return i
		}
	}
	for i, r := range rows {
		h := normalizeHeaders(r)
		if hasAll(h, []string{"Player", "GS"}) {
			return i
		}
	}
	return -1
}
func normalizeHeaders(h []string) []string {
	out := make([]string, len(h))
	for i, s := range h {
		s = stripBOM(s)
		s = strings.TrimSpace(s)
		s = strings.Join(strings.Fields(s), " ")
		out[i] = s
	}
	return out
}
func stripBOM(s string) string { return strings.TrimPrefix(s, "\uFEFF") }
func hasAll(h []string, want []string) bool {
	set := map[string]struct{}{}
	for _, s := range h {
		set[s] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}
func findColumn(h []string, base string) int {
	for i, s := range h {
		if s == base {
			return i
		}
	}
	pref := base + "."
	for i, s := range h {
		if strings.HasPrefix(s, pref) {
			return i
		}
	}
	return -1
}
func findColumnPrefer(h []string, opts []string) int {
	for _, o := range opts {
		if idx := findColumn(h, o); idx >= 0 {
			return idx
		}
	}
	return -1
}
func cleanPlayer(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "*", "")
	p = strings.ReplaceAll(p, "+", "")
	return strings.TrimSpace(p)
}
func ParsePositions(s string) []string {
	ps := strings.Split(s, ",")
	var out []string
	for _, p := range ps {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
func isPositionMatch(tokens []string, pos string) bool {
	if pos == "" {
		return false
	}
	p := strings.ToUpper(strings.TrimSpace(pos))
	p = strings.ReplaceAll(p, "/", "")
	p = strings.ReplaceAll(p, "-", "")
	for _, t := range tokens {
		if strings.HasSuffix(p, t) || strings.Contains(p, t) {
			return true
		}
	}
	return false
}
func pickPrimaryTeam(teamGS, teamG map[string]int) string {
	if len(teamGS) == 0 {
		return ""
	}
	type t struct {
		Tm    string
		GS, G int
	}
	all := make([]t, 0, len(teamGS))
	for tm, gs := range teamGS {
		all = append(all, t{tm, gs, teamG[tm]})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].GS != all[j].GS {
			return all[i].GS > all[j].GS
		}
		if all[i].G != all[j].G {
			return all[i].G > all[j].G
		}
		return all[i].Tm < all[j].Tm
	})
	return all[0].Tm
}
func joinSortedKeys[M ~map[string]struct{}](m M, sep string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, sep)
}
func Atoi(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
func ifZeroThenLarge(n int) int {
	if n == 0 {
		return 1 << 30
	}
	return n
}

// ParseDefenseHTML parses the league player defense table from HTML.
// It auto-detects the table and aggregates per-player rows.
// If the selected table lacks GS entirely, you may set ASSUME_GS_EQUALS_G=1
// to treat GS as G for testing (not recommended for final stats).
func ParseDefenseHTML(html string, posTokens []string, maxAge int) (_ []PlayerRow, err error) {
	// Panic guard
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("html parse panic: %v", r)
		}
	}()

	// De-comment (SR often wraps tables in comments)
	clean := strings.ReplaceAll(html, "<!--", "")
	clean = strings.ReplaceAll(clean, "-->", "")

	doc, err := goquery.NewDocumentFromReader(bytes.NewBufferString(clean))
	if err != nil {
		return nil, err
	}

	table := findPlayerDefenseTable(doc)
	if table.Length() == 0 {
		return nil, errors.New("defense player table not found in HTML")
	}
	if os.Getenv("DEBUG") == "1" {
		if id, ok := table.Attr("id"); ok {
			log.Printf("DEBUG ParseDefenseHTML: selected table id=%s", id)
		} else {
			log.Printf("DEBUG ParseDefenseHTML: selected table with no id (auto-detected)")
		}
	}

	// Detect whether this table actually has a GS column
	hasGSCol := table.Find(`tbody tr td[data-stat="gs"], tbody tr td[data-stat="games_started"]`).Length() > 0
	assumeGSEqualsG := (os.Getenv("ASSUME_GS_EQUALS_G") == "1") && !hasGSCol
	if os.Getenv("DEBUG") == "1" {
		log.Printf("DEBUG ParseDefenseHTML: hasGSCol=%v, assumeGSEqualsG=%v", hasGSCol, assumeGSEqualsG)
	}

	// Debug counters
	debug := os.Getenv("DEBUG") == "1"
	totalRows, defPosRows, afterSkipTOT := 0, 0, 0

	type agg struct {
		Player string
		AgeMin int
		GSum   int
		GSSum  int
		PosSet map[string]struct{}
		TeamGS map[string]int
		TeamG  map[string]int
		Teams  map[string]struct{}
	}
	ag := map[string]*agg{}

	rows := table.Find("tbody tr")
	if rows.Length() == 0 {
		rows = table.Find("tr")
	}

	rows.Each(func(_ int, tr *goquery.Selection) {
		cls := tr.AttrOr("class", "")
		if strings.Contains(cls, "thead") {
			return
		}
		totalRows++

		var player, pos, tm string
		var age, g, gs int

		tr.Find("th, td").Each(func(_ int, cell *goquery.Selection) {
			ds := cell.AttrOr("data-stat", "")
			text := strings.TrimSpace(cell.Text())
			switch ds {
			case "player":
				player = cleanPlayer(text)

			case "age":
				age = Atoi(text, 0)

			case "team", "team_name", "team_abbr", "team_id":
				tm = strings.ToUpper(text)

			case "pos", "position", "def_pos":
				pos = text

			case "g", "games":
				g = Atoi(text, 0)

			case "gs", "games_started":
				gs = Atoi(text, 0)
			}
		})

		if player == "" {
			return
		}
		if !isPositionMatch(posTokens, pos) {
			return
		}
		defPosRows++

		// skip aggregate TOT / 2TM / 3TM
		if tm == "TOT" || strings.HasSuffix(tm, "TM") {
			return
		}
		afterSkipTOT++

		if assumeGSEqualsG {
			gs = g
		}

		a := ag[player]
		if a == nil {
			a = &agg{
				Player: player,
				AgeMin: ifZeroThenLarge(age),
				PosSet: map[string]struct{}{},
				TeamGS: map[string]int{},
				TeamG:  map[string]int{},
				Teams:  map[string]struct{}{},
			}
			ag[player] = a
		}
		if age > 0 && age < a.AgeMin {
			a.AgeMin = age
		}
		a.GSum += g
		a.GSSum += gs
		if p := strings.TrimSpace(pos); p != "" {
			a.PosSet[p] = struct{}{}
		}
		if tm != "" {
			a.TeamGS[tm] += gs
			a.TeamG[tm] += g
			a.Teams[tm] = struct{}{}
		}
	})

	var out []PlayerRow
	for _, a := range ag {
		age := a.AgeMin
		if age == 1<<30 {
			age = 0
		}
		// must have played at least one game and started them all
		if age <= maxAge && a.GSum > 0 && a.GSum == a.GSSum {
			out = append(out, PlayerRow{
				Player: a.Player,
				Team:   pickPrimaryTeam(a.TeamGS, a.TeamG),
				Teams:  joinSortedKeys(a.Teams, ","),
				Age:    age,
				G:      a.GSum,
				GS:     a.GSSum,
				Pos:    joinSortedKeys(a.PosSet, ","),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Player < out[j].Player })

	if debug {
		log.Printf("DEBUG ParseDefenseHTML: totalRows=%d defPosRows=%d afterSkipTOT=%d aggregatedPlayers=%d eligibleFinal=%d",
			totalRows, defPosRows, afterSkipTOT, len(ag), len(out))
	}

	return out, nil
}

// findPlayerDefenseTable scans all <table> nodes and returns the one that looks
// like the player defense table (has player + G + GS under common data-stat names).
// Returns an empty-but-valid selection if none found.
func findPlayerDefenseTable(doc *goquery.Document) *goquery.Selection {
	tables := doc.Find("table") // bound to doc
	if tables.Length() == 0 {
		return tables.Slice(0, 0)
	}

	var chosen *goquery.Selection
	tables.EachWithBreak(func(_ int, t *goquery.Selection) bool {
		hasPlayer := t.Find(`tbody tr th[data-stat="player"]`).Length() > 0
		hasG := t.Find(`tbody tr td[data-stat="g"], tbody tr td[data-stat="games"]`).Length() > 0
		hasGS := t.Find(`tbody tr td[data-stat="gs"], tbody tr td[data-stat="games_started"]`).Length() > 0
		if hasPlayer && hasG && hasGS {
			chosen = t
			return false
		}
		return true
	})

	if chosen != nil {
		return chosen
	}
	return tables.Slice(0, 0)
}

// at top (near other helpers)
func acceptPosition(posTokens []string, pos string) bool {
	relax := os.Getenv("RELAX_POS") == "1"
	if relax {
		return true
	}
	return isPositionMatch(posTokens, pos)
}
