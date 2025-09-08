package pfr

import (
	"log"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Debug utility: list tables with ids and first header row
func DumpTablesForDebug(doc *goquery.Document, pageTag string) {
	if os.Getenv("DEBUG") != "1" {
		return
	}
	doc.Find("table").Each(func(i int, t *goquery.Selection) {
		id, _ := t.Attr("id")
		cl := t.AttrOr("class", "")
		var heads []string
		t.Find("thead tr").First().Find("th,td").Each(func(_ int, h *goquery.Selection) {
			txt := strings.ToLower(strings.TrimSpace(h.Text()))
			if txt != "" {
				heads = append(heads, txt)
			}
		})
		log.Printf("DEBUG table[%d] id=%q class=%q headers=%q (%s)", i, id, cl, strings.Join(heads, "|"), pageTag)
	})
}

// Header-based heuristic: looks like "Defense & Fumbles"
// func findDefenseTableByHeaders(doc *goquery.Document) *goquery.Selection {
// 	keywords := []string{"int", "sk", "sacks", "tackles", "tkl", "solo", "comb", "pd", "qb hits", "qb hit"}
// 	tables := doc.Find("table")
// 	chosen := tables.Slice(0, 0)
// 	tables.EachWithBreak(func(_ int, t *goquery.Selection) bool {
// 		var heads []string
// 		t.Find("thead tr").First().Find("th,td").Each(func(_ int, h *goquery.Selection) {
// 			txt := strings.ToLower(strings.TrimSpace(h.Text()))
// 			if txt != "" {
// 				heads = append(heads, txt)
// 			}
// 		})
// 		if len(heads) == 0 {
// 			return true
// 		}
// 		hasPlayer := false
// 		matchCount := 0
// 		for _, h := range heads {
// 			if strings.Contains(h, "player") {
// 				hasPlayer = true
// 			}
// 			for _, kw := range keywords {
// 				if strings.Contains(h, kw) {
// 					matchCount++
// 					break
// 				}
// 			}
// 		}
// 		if hasPlayer && matchCount >= 2 {
// 			chosen = t
// 			return false
// 		}
// 		return true
// 	})
// 	return chosen
// }

// // Strict league-page finder (requires player + G + GS)
// func findPlayerDefenseTable(doc *goquery.Document) *goquery.Selection {
// 	tables := doc.Find("table")
// 	if tables.Length() == 0 {
// 		return tables.Slice(0, 0)
// 	}
// 	if t := doc.Find(`table#defense, table#defense_and_fumbles, table#player_defense`); t.Length() > 0 {
// 		return t.First()
// 	}
// 	var chosen *goquery.Selection
// 	tables.EachWithBreak(func(_ int, t *goquery.Selection) bool {
// 		hasPlayer := t.Find(`tbody tr th[data-stat="player"]`).Length() > 0
// 		hasG := t.Find(`tbody tr td[data-stat="g"], tbody tr td[data-stat="games"]`).Length() > 0
// 		hasGS := t.Find(`tbody tr td[data-stat="gs"], tbody tr td[data-stat="games_started"]`).Length() > 0
// 		if hasPlayer && hasG && hasGS {
// 			chosen = t
// 			return false
// 		}
// 		return true
// 	})
// 	if chosen != nil {
// 		return chosen
// 	}
// 	if byHdr := findDefenseTableByHeaders(doc); byHdr.Length() > 0 {
// 		return byHdr
// 	}
// 	return tables.Slice(0, 0)
// }

// // Loose team-page finder: prefer ids; else header heuristic; else player+G
// func findPlayerDefenseTableLoose(doc *goquery.Document) *goquery.Selection {
// 	tables := doc.Find("table")
// 	if tables.Length() == 0 {
// 		return tables.Slice(0, 0)
// 	}
// 	if t := doc.Find(`table#defense, table#defense_and_fumbles, table#player_defense`); t.Length() > 0 {
// 		return t.First()
// 	}
// 	if byHdr := findDefenseTableByHeaders(doc); byHdr.Length() > 0 {
// 		return byHdr
// 	}
// 	var chosen *goquery.Selection
// 	tables.EachWithBreak(func(_ int, t *goquery.Selection) bool {
// 		hasPlayer := t.Find(`tbody tr th[data-stat="player"]`).Length() > 0
// 		hasG := t.Find(`tbody tr td[data-stat="g"], tbody tr td[data-stat="games"]`).Length() > 0
// 		if hasPlayer && hasG {
// 			chosen = t
// 			return false
// 		}
// 		return true
// 	})
// 	if chosen != nil {
// 		return chosen
// 	}
// 	return tables.Slice(0, 0)
// }
