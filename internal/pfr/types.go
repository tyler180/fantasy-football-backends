package pfr

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const ua = "Mozilla/5.0 (compatible; PFRRosterBot/1.0; +https://example.com/bot)"

type PlayerRow struct {
	Player     string
	PlayerID   string
	Team       string
	Teams      string
	Age        int
	G          int
	GS         int
	Pos        string
	DefSnapNum int
	DefSnapPct float64
}

type RosterRow struct {
	Season     string
	PlayerID   string
	Player     string
	Team       string
	Age        int
	Pos        string
	G          int
	GS         int
	DefSnapNum int
	DefSnapPct float64
}

// tools/pfr-weekly/internal/pfr/types.go
type SnapGameRow struct {
	Season     string
	Team       string
	Week       int
	PlayerID   string
	Player     string
	DefSnapPct float64 // 0..100; if DNP/missing -> 0 (we can add a flag if you prefer)
}

type Team struct {
	Abbr string // e.g. "SEA"
	Path string // e.g. "sea" (used in https://www.pro-football-reference.com/teams/{Path}/...)
	Name string // optional display name, not required
}

func Atoi(s string, def int) int {
	i, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return i
}

func ParsePositions(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func isPositionMatch(allow []string, pos string) bool {
	if len(allow) == 0 {
		return true
	}
	pos = strings.ToUpper(strings.TrimSpace(pos))
	for _, p := range strings.Split(pos, ",") {
		p = strings.TrimSpace(p)
		for _, want := range allow {
			if p == want {
				return true
			}
		}
	}
	return false
}

func cleanPlayer(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*+")
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, " ")
}

func ifZeroThenLarge(n int) int {
	if n == 0 {
		return 1 << 30
	}
	return n
}

func joinSortedKeys(set map[string]struct{}, sep string) string {
	if len(set) == 0 {
		return ""
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, sep)
}

func joinSortedKeysMap(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// AllTeams returns the canonical list used by PFR for team URL paths.
// Note: PFR uses historical 3-letter codes like gnb, sfo, sdg, rai, etc.
func AllTeams() []Team {
	return []Team{
		{Abbr: "ARI", Path: "crd", Name: "Arizona Cardinals"},
		{Abbr: "ATL", Path: "atl", Name: "Atlanta Falcons"},
		{Abbr: "BAL", Path: "rav", Name: "Baltimore Ravens"},
		{Abbr: "BUF", Path: "buf", Name: "Buffalo Bills"},
		{Abbr: "CAR", Path: "car", Name: "Carolina Panthers"},
		{Abbr: "CHI", Path: "chi", Name: "Chicago Bears"},
		{Abbr: "CIN", Path: "cin", Name: "Cincinnati Bengals"},
		{Abbr: "CLE", Path: "cle", Name: "Cleveland Browns"},
		{Abbr: "DAL", Path: "dal", Name: "Dallas Cowboys"},
		{Abbr: "DEN", Path: "den", Name: "Denver Broncos"},
		{Abbr: "DET", Path: "det", Name: "Detroit Lions"},
		{Abbr: "GNB", Path: "gnb", Name: "Green Bay Packers"}, // PFR uses GNB
		{Abbr: "HOU", Path: "htx", Name: "Houston Texans"},
		{Abbr: "IND", Path: "clt", Name: "Indianapolis Colts"},
		{Abbr: "JAX", Path: "jax", Name: "Jacksonville Jaguars"},
		{Abbr: "KAN", Path: "kan", Name: "Kansas City Chiefs"},   // PFR uses KAN
		{Abbr: "LVR", Path: "rai", Name: "Las Vegas Raiders"},    // PFR path is "rai"
		{Abbr: "LAC", Path: "sdg", Name: "Los Angeles Chargers"}, // PFR path is "sdg"
		{Abbr: "LAR", Path: "ram", Name: "Los Angeles Rams"},     // PFR path is "ram"
		{Abbr: "MIA", Path: "mia", Name: "Miami Dolphins"},
		{Abbr: "MIN", Path: "min", Name: "Minnesota Vikings"},
		{Abbr: "NWE", Path: "nwe", Name: "New England Patriots"}, // PFR uses NWE
		{Abbr: "NOR", Path: "nor", Name: "New Orleans Saints"},   // PFR uses NOR
		{Abbr: "NYG", Path: "nyg", Name: "New York Giants"},
		{Abbr: "NYJ", Path: "nyj", Name: "New York Jets"},
		{Abbr: "PHI", Path: "phi", Name: "Philadelphia Eagles"},
		{Abbr: "PIT", Path: "pit", Name: "Pittsburgh Steelers"},
		{Abbr: "SFO", Path: "sfo", Name: "San Francisco 49ers"}, // PFR uses SFO
		{Abbr: "SEA", Path: "sea", Name: "Seattle Seahawks"},
		{Abbr: "TAM", Path: "tam", Name: "Tampa Bay Buccaneers"}, // PFR uses TAM
		{Abbr: "TEN", Path: "oti", Name: "Tennessee Titans"},     // PFR path "oti"
		{Abbr: "WAS", Path: "was", Name: "Washington Commanders"},
	}
}

// AbbrToPath returns the PFR path (e.g., "sea") for an NFL abbr (e.g., "SEA").
func AbbrToPath(abbr string) (string, bool) {
	a := strings.ToUpper(strings.TrimSpace(abbr))
	for _, t := range AllTeams() {
		if t.Abbr == a {
			return t.Path, true
		}
	}
	return "", false
}

// Abbrs returns a slice of all NFL abbreviations in the canonical order above.
func Abbrs() []string {
	ts := AllTeams()
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Abbr)
	}
	return out
}

// Paths returns a slice of all team URL path fragments (e.g., "sea", "tam", ...).
func Paths() []string {
	ts := AllTeams()
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Path)
	}
	return out
}
