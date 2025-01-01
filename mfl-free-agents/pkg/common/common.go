package common

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"github.com/tyler180/retrieve-secret/retrievesecrets"
)

const (
	// leagueID = "LEAGUE_ID"
	// username = "USERNAME"
	// password = "PASSWORD"
	year    = "2024"
	proto   = "https"
	apiHost = "www49.myfantasyleague.com"
	// setjson = 1
	reqType = "league"
)

type User struct {
	Name     string
	Email    string
	Username string
}

type MFLParams struct {
	UserName    string
	Password    string
	APIKey      string
	LeagueID    string
	FranchiseID string
	LeagueYear  string
	SetJSON     int
	SetXML      int
}

func NewUser(name, email, username string) User {
	return User{
		Name:     defaultIfEmpty(name, "Anonymous"),
		Email:    defaultIfEmpty(email, "no-reply@example.com"),
		Username: defaultIfEmpty(username, "guest"),
	}
}

// defaultIfEmpty is a generic function that works for both string and int
func defaultIfEmpty[T comparable](value, defaultValue T) T {
	var zero T // Zero value for the type
	if value == zero {
		return defaultValue
	}
	return value
}

func NewMFLParams(ctx context.Context, json, xml int, secretName string) (MFLParams, error) {

	secretData, err := retrievesecrets.RetrieveSecret(ctx, secretName, retrievesecrets.SecretTypeJSON, "")
	if err != nil {
		return MFLParams{}, err
	}

	un, ok := secretData["username"]
	if !ok || un == "" {
		un = "notfound"
	}

	pw, ok := secretData["password"]
	if !ok || pw == "" {
		pw = "notfound"
	}

	apiKey, ok := secretData["api_key"]
	if !ok || apiKey == "" {
		apiKey = "notfound"
	}

	lID, ok := secretData["league_id"]
	if !ok || lID == "" {
		lID = "notfound"
	}

	fID, ok := secretData["franchise_id"]
	if !ok || fID == "" {
		fID = "notfound"
	}

	leagueYear, ok := secretData["league_year"]
	if !ok || leagueYear == "" {
		leagueYear = "notfound"
	}

	var setjson int
	var setxml int
	setjson = defaultIfEmpty(json, 1)
	setxml = defaultIfEmpty(xml, 0)

	if setjson == setxml {
		setjson = 1
		setxml = 0
	}

	return MFLParams{
		UserName:    un,
		Password:    pw,
		APIKey:      apiKey,
		LeagueID:    lID,
		FranchiseID: fID,
		LeagueYear:  defaultIfEmpty(leagueYear, "2024"),
		SetJSON:     setjson,
		SetXML:      setxml,
	}, nil
}

func (p *MFLParams) GetLeagueURL(cookie string) (string, error) {
	client := &http.Client{}
	url := fmt.Sprintf("%s://%s/%s/export", proto, apiHost, year)
	headers := http.Header{}
	headers.Add("Cookie", fmt.Sprintf("MFL_USER_ID=%s", cookie))
	mlArgs := fmt.Sprintf("TYPE=myleagues&JSON=%d", p.SetJSON)
	mlURL := fmt.Sprintf("%s?%s", url, mlArgs)

	req, err := http.NewRequest("GET", mlURL, nil)
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers

	mlResp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making league request: %v", err)
	}
	defer mlResp.Body.Close()

	mlBody, err := io.ReadAll(mlResp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading league response: %v", err)
	}

	leagueHostRegex := regexp.MustCompile(`url="(https?)://([a-z0-9]+.myfantasyleague.com)/` + year + `/home/` + p.LeagueID + `"`)
	leagueMatches := leagueHostRegex.FindStringSubmatch(string(mlBody))
	if len(leagueMatches) < 1 {
		return "", fmt.Errorf("no league host found in response")
	}
	protocol := leagueMatches[1]
	leagueHost := leagueMatches[2]
	fmt.Printf("Got league host %s\n", leagueHost)
	url = fmt.Sprintf("%s://%s/%s/export", protocol, leagueHost, year)
	fmt.Println("The value of url is:")
	fmt.Println(url)

	return url, nil
}

// ConstructURL constructs a URL based on the base URL, keys, and their values
func ConstructURL(baseURL string, params map[string]string) (string, error) {
	// Parse the base URL
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Add query parameters
	query := u.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	u.RawQuery = query.Encode()

	return u.String(), nil
}

func (p *MFLParams) GetCookie(client *http.Client) (string, error) {

	loginURL := fmt.Sprintf("https://%s/%s/login?USERNAME=%s&PASSWORD=%s&JSON=%d", apiHost, year, p.UserName, p.Password, p.SetJSON)
	fmt.Printf("Making request to get cookie: %s\n", loginURL)
	loginResp, err := client.Get(loginURL)
	if err != nil {
		fmt.Println("in the loginResp error check")
		return "", fmt.Errorf("error making login request: %v", err)
	}
	defer loginResp.Body.Close()

	body, err := io.ReadAll(loginResp.Body)
	if err != nil {
		fmt.Println("in the body error check")
		return "", fmt.Errorf("error reading login response: %v", err)
	}

	cookieRegex := regexp.MustCompile(`MFL_USER_ID="([^"]*)">OK`)
	fmt.Printf("value of cookieRegex is: %v", cookieRegex)
	matches := cookieRegex.FindStringSubmatch(string(body))
	if len(matches) < 1 {
		fmt.Println("len(matches) is less than 1")
		return "", fmt.Errorf("cannot get login cookie. Response: %s", string(body))
	}
	cookie := matches[1]
	fmt.Printf("in GetCookie function and the value of cookie is: %s", cookie)
	return cookie, nil
}
