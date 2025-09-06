package pfr

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var httpCliTeams = &http.Client{Timeout: 30 * time.Second}

var teamCodes = []struct {
	Path string
	Abbr string
}{
	{"crd", "CRD"}, {"atl", "ATL"}, {"rav", "RAV"}, {"buf", "BUF"},
	{"car", "CAR"}, {"chi", "CHI"}, {"cin", "CIN"}, {"cle", "CLE"},
	{"dal", "DAL"}, {"den", "DEN"}, {"det", "DET"}, {"gnb", "GNB"},
	{"htx", "HTX"}, {"clt", "CLT"}, {"jax", "JAX"}, {"kan", "KAN"},
	{"rai", "RAI"}, {"sdg", "SDG"}, {"ram", "RAM"}, {"mia", "MIA"},
	{"min", "MIN"}, {"nwe", "NWE"}, {"nor", "NOR"}, {"nyg", "NYG"},
	{"nyj", "NYJ"}, {"phi", "PHI"}, {"pit", "PIT"}, {"sfo", "SFO"},
	{"sea", "SEA"}, {"tam", "TAM"}, {"oti", "OTI"}, {"was", "WAS"},
}

func FetchSeasonDefenseViaTeams(ctx context.Context, season string, posTokens []string, maxAge int) ([]PlayerRow, error) {
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
	debug := os.Getenv("DEBUG") == "1"
	assumeGSEqualsG := os.Getenv("ASSUME_GS_EQUALS_G") == "1"

	totalTeams := 0
	totalRows := 0
	defPosRows := 0

	for _, t := range teamCodes {
		url := fmt.Sprintf("https://www.pro-football-reference.com/teams/%s/%s.htm", t.Path, season)
		html, err := getTextWithUA(ctx, url)
		if err != nil {
			if debug {
				log.Printf("DEBUG teams: fetch %s failed: %v", url, err)
			}
			continue
		}

		clean := strings.ReplaceAll(html, "<!--", "")
		clean = strings.ReplaceAll(clean, "-->", "")
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(clean))
		if err != nil {
			if debug {
				log.Printf("DEBUG teams: parse %s failed: %v", url, err)
			}
			continue
		}

		table := findPlayerDefenseTable(doc)
		if table.Length() == 0 {
			if debug {
				log.Printf("DEBUG teams: no player-defense table for %s", t.Abbr)
			}
			continue
		}

		hasGSCol := table.Find(`tbody tr td[data-stat="gs"], tbody tr td[data-stat="games_started"]`).Length() > 0
		rows := table.Find("tbody tr")
		if rows.Length() == 0 {
			rows = table.Find("tr")
		}

		thisRows := 0
		thisKept := 0

		rows.Each(func(_ int, tr *goquery.Selection) {
			cls := tr.AttrOr("class", "")
			if strings.Contains(cls, "thead") {
				return
			}

			totalRows++
			thisRows++

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
			if !acceptPosition(posTokens, pos) {
				return
			}
			defPosRows++

			// team pages often omit the team col; default to page team
			if tm == "" {
				tm = t.Abbr
			}
			if tm == "TOT" || strings.HasSuffix(tm, "TM") {
				return
			}

			if !hasGSCol && assumeGSEqualsG {
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
			thisKept++
		})

		totalTeams++
		if debug {
			log.Printf("DEBUG teams: %s rows=%d kept=%d hasGSCol=%v", t.Abbr, thisRows, thisKept, hasGSCol)
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

	if debug {
		log.Printf("DEBUG teams summary: teams=%d totalRows=%d defPosRows=%d eligibleFinal=%d",
			totalTeams, totalRows, defPosRows, len(out))
	}

	return out, nil
}

func getTextWithUA(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := httpCliTeams.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("status %d for %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
