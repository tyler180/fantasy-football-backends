package pfr

// import (
// 	"bytes"
// 	"encoding/csv"
// 	"strings"
// 	"testing"
// )

// func defaultTokens() []string {
// 	return ParsePositions("DE,DT,NT,DL,EDGE,LB,ILB,OLB,MLB,CB,DB,S,FS,SS,SAF,NB")
// }

// func TestParseAndFilterCSV_Basics(t *testing.T) {
// 	csv := `Rk,Player,Age,Team,Pos,G,GS
// 1,John Doe,23,ATL,CB,10,10
// 2,John Doe,23,CHI,CB,7,7
// 3,John Doe,23,TOT,CB,17,17
// 4,Old Guy,25,NE,SS,17,17
// 5,Backup,22,SEA,LB,9,2
// 6,Quarterback,24,BUF,QB,17,17
// `
// 	rows, err := ParseAndFilterCSV(strings.NewReader(csv), defaultTokens(), 24)
// 	if err != nil {
// 		t.Fatalf("ParseAndFilterCSV error: %v", err)
// 	}
// 	if len(rows) != 1 {
// 		t.Fatalf("expected 1 eligible row, got %d", len(rows))
// 	}
// 	got := rows[0]
// 	if got.Player != "John Doe" {
// 		t.Errorf("Player = %q, want %q", got.Player, "John Doe")
// 	}
// 	if got.Age != 23 {
// 		t.Errorf("Age = %d, want 23", got.Age)
// 	}
// 	if got.G != 17 || got.GS != 17 {
// 		t.Errorf("G/GS = %d/%d, want 17/17", got.G, got.GS)
// 	}
// 	if got.Team != "ATL" {
// 		t.Errorf("Team = %q, want ATL (highest GS team)", got.Team)
// 	}
// 	if got.Pos != "CB" {
// 		t.Errorf("Pos = %q, want CB", got.Pos)
// 	}
// 	if got.Teams != "ATL,CHI" {
// 		t.Errorf("Teams = %q, want ATL,CHI", got.Teams)
// 	}
// }

// func TestParseDefenseHTML_WithCommentsAndVariants(t *testing.T) {
// 	html := `<!--
// <table id="defense">
//   <tbody>
//     <tr>
//       <th scope="row" data-stat="player">John Doe</th>
//       <td data-stat="age">23</td>
//       <td data-stat="team_name">ATL</td>
//       <td data-stat="pos">CB</td>
//       <td data-stat="g">10</td>
//       <td data-stat="gs">10</td>
//     </tr>
//     <tr>
//       <th scope="row" data-stat="player">John Doe</th>
//       <td data-stat="age">23</td>
//       <td data-stat="team_abbr">CHI</td>
//       <td data-stat="position">CB</td>
//       <td data-stat="g">7</td>
//       <td data-stat="gs">7</td>
//     </tr>
//     <tr>
//       <th scope="row" data-stat="player">John Doe</th>
//       <td data-stat="age">23</td>
//       <td data-stat="team">TOT</td>
//       <td data-stat="pos">CB</td>
//       <td data-stat="g">17</td>
//       <td data-stat="gs">17</td>
//     </tr>
//     <tr>
//       <th scope="row" data-stat="player">Old Guy</th>
//       <td data-stat="age">25</td>
//       <td data-stat="team">NE</td>
//       <td data-stat="pos">SS</td>
//       <td data-stat="g">17</td>
//       <td data-stat="gs">17</td>
//     </tr>
//   </tbody>
// </table>
// -->`

// 	rows, err := ParseDefenseHTML(html, defaultTokens(), 24)
// 	if err != nil {
// 		t.Fatalf("ParseDefenseHTML error: %v", err)
// 	}
// 	if len(rows) != 1 {
// 		t.Fatalf("expected 1 eligible row, got %d", len(rows))
// 	}
// 	got := rows[0]
// 	if got.Player != "John Doe" || got.Team != "ATL" || got.G != 17 || got.GS != 17 {
// 		t.Errorf("unexpected row: %+v", got)
// 	}
// }

// // sanity: helper funcs
// func TestIsPositionMatch(t *testing.T) {
// 	if !isPositionMatch(defaultTokens(), "CB") {
// 		t.Error("CB should match")
// 	}
// 	if isPositionMatch(defaultTokens(), "QB") {
// 		t.Error("QB should NOT match")
// 	}
// }

// func TestFindColumnAndHeaderDetect(t *testing.T) {
// 	csv := "Rk,Player,Age,Team,Pos,G,GS\n1,A,22,ATL,CB,1,1\n"
// 	r := csvReader(csv)
// 	all, _ := r.ReadAll()
// 	idx := detectHeaderRow(all)
// 	if idx != 0 {
// 		t.Fatalf("header row idx = %d, want 0", idx)
// 	}

// 	h := normalizeHeaders(all[idx])
// 	if findColumn(h, "Player") < 0 || findColumn(h, "GS") < 0 {
// 		t.Fatal("expected to find Player and GS columns")
// 	}
// }

// // small helper
// func csvReader(s string) *csv.Reader {
// 	return csv.NewReader(bytes.NewBufferString(s))
// }
