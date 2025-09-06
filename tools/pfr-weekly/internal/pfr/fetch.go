package pfr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const baseWWW = "https://www.pro-football-reference.com"
const baseAWS = "https://aws.pro-football-reference.com"

var httpCli = &http.Client{Timeout: 30 * time.Second}
var ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119 Safari/537.36 (+stats-research)"

// Try the share-CSV path; caller should fall back to HTML parsing if this fails.
func FetchDefenseCSV(ctx context.Context, season string) (string, error) {
	html, src, err := FetchDefensePage(ctx, season)
	if err != nil {
		return "", fmt.Errorf("fetch defense page: %w", err)
	}
	csvURL, err := extractCSVLink(html, src)
	if err != nil {
		// debug: dump a slice of HTML for inspection
		if os.Getenv("DEBUG") == "1" {
			html, _, _ := FetchDefensePage(ctx, season)
			if len(html) > 0 {
				log.Printf("DEBUG first 2000 bytes of HTML:\n%s", html[:min(2000, len(html))])
			}
		}
		return "", fmt.Errorf("extract csv link: %w", err)
	}
	txt, err := getText(ctx, csvURL)
	if err != nil {
		return "", fmt.Errorf("download csv: %w", err)
	}
	return txt, nil
}

// Fetch the league defense page HTML; try both known hosts.
func FetchDefensePage(ctx context.Context, season string) (html string, sourceHost string, err error) {
	for _, host := range []string{baseWWW, baseAWS} {
		pageURL := fmt.Sprintf("%s/years/%s/defense.htm", host, season)
		txt, e := getText(ctx, pageURL)
		if e == nil {
			return txt, host, nil
		}
		err = e
	}
	return "", "", err
}

func getText(ctx context.Context, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", ua)
	resp, err := httpCli.Do(req)
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

// Handle several variants seen on SR sites; scope to #all_defense if possible.
func extractCSVLink(html string, host string) (string, error) {
	segment := html
	if i := strings.Index(html, `id="all_defense"`); i >= 0 {
		segment = html[i:]
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`href="([^"]*/tools/share\.fcgi\?id=[^"]+)"`),
		regexp.MustCompile(`href="([^"]*/share\.fcgi\?id=[^"]+)"`),
		regexp.MustCompile(`href="([^"]+)"[^>]*>(?:Get table as CSV(?:\s*$begin:math:text$for Excel$end:math:text$)?)</a>`),
	}
	for _, re := range patterns {
		if m := re.FindStringSubmatch(segment); m != nil {
			return absolutize(host, m[1]), nil
		}
	}
	for _, re := range patterns {
		if m := re.FindStringSubmatch(html); m != nil {
			return absolutize(host, m[1]), nil
		}
	}
	return "", errors.New("CSV share link not found")
}

func absolutize(host, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	if strings.HasPrefix(href, "/") {
		return host + href
	}
	return host + "/" + href
}
