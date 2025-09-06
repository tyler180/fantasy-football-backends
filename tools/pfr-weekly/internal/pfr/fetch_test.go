package pfr

import "testing"

func TestExtractCSVLink_ToolsAndPlain(t *testing.T) {
	html := `<div id="all_defense">
  <a href="/tools/share.fcgi?id=abc123">Get table as CSV (for Excel)</a>
</div>`
	got, err := extractCSVLink(html, baseWWW)
	if err != nil {
		t.Fatalf("extract err: %v", err)
	}
	want := baseWWW + "/tools/share.fcgi?id=abc123"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}

	html2 := `<div><a href="/share.fcgi?id=xyz">Get table as CSV</a></div>`
	got2, err := extractCSVLink(html2, baseAWS)
	if err != nil {
		t.Fatalf("extract2 err: %v", err)
	}
	want2 := baseAWS + "/share.fcgi?id=xyz"
	if got2 != want2 {
		t.Fatalf("got2 %q want2 %q", got2, want2)
	}
}
