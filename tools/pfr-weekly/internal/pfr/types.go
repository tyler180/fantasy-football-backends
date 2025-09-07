package pfr

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const ua = "Mozilla/5.0 (compatible; PFRRosterBot/1.0; +https://example.com/bot)"

type PlayerRow struct {
	Player string
	Team   string
	Teams  string
	Age    int
	G      int
	GS     int
	Pos    string
}

type RosterRow struct {
	Season   string
	PlayerID string
	Player   string
	Team     string
	Age      int
	Pos      string
	G        int
	GS       int
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
	// allow comma-separated positions from source
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
	// PFR marks awards with *, +; strip trailing/leading spaces and these marks
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "*+")
	// collapse internal whitespace
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
