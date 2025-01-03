package common

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"

	"github.com/tyler180/retrieve-secret/retrievesecrets"
)

const (
	proto   = "https"
	apiHost = "www49.myfantasyleague.com"
	reqType = "league"
)

type BaseURL struct {
	Proto   string
	APIHost string
	ReqType string
}

func NewBaseURL(proto, apiHost, reqType string) *BaseURL {
	return &BaseURL{
		Proto:   defaultIfEmpty(proto, "https"),
		APIHost: defaultIfEmpty(apiHost, "www49.myfantasyleague.com"),
		ReqType: defaultIfEmpty(reqType, "league"),
	}
}

type URLBuilder struct {
	BaseURL
	MFLParams
}

func NewURLBuilder(mp MFLParams, bu BaseURL) URLBuilder {
	return URLBuilder{
		BaseURL:   bu,
		MFLParams: mp,
	}
}

type MFLParams struct {
	UserName    string
	Password    string
	APIKey      string
	LeagueID    string
	FranchiseID string
	LeagueYear  string
	SetJSON     string
	SetXML      string
}

func NewMFLParams(ctx context.Context, secretName string) (*MFLParams, error) {

	secretData, err := retrievesecrets.RetrieveSecret(ctx, secretName, "json", "")
	if err != nil {
		// fmt.Println("getting the secretData using retrievesecrets.RetrieveSecret is failing")
		return nil, err
	}

	// fmt.Println("secretData:", secretData)

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

	setjson := secretData["json"]
	setxml := secretData["xml"]

	// var setjson int
	// var setxml int
	// setjson = ConvertStringToInt(json, 1)
	// setxml = ConvertStringToInt(xml, 0)

	if setjson == setxml {
		setjson = "1"
		setxml = "0"
	}

	return &MFLParams{
		UserName:    un,
		Password:    pw,
		APIKey:      apiKey,
		LeagueID:    lID,
		FranchiseID: fID,
		LeagueYear:  leagueYear,
		SetJSON:     setjson,
		SetXML:      setxml,
	}, nil
}

func GetSecrets(ctx context.Context) (map[string]string, error) {
	secretName := os.Getenv("SECRET_NAME")
	// fmt.Println("the value of secretName is:", secretName)

	secretsRetrieved, err := retrievesecrets.RetrieveSecret(ctx, secretName, "json", "")
	if err != nil {
		fmt.Printf("err is not nil and is: %s", err)
		//fmt.Errorf("failed to load AWS config %w", err)
		return nil, err
	}

	return secretsRetrieved, nil
}

// https://www49.myfantasyleague.com/2024/export?TYPE=freeAgents&L=79286&APIKEY=ahBi2siVvuWrx1OmP1DDaTQeELox&POSITION=QB&JSON=1

func (p *MFLParams) GetLeagueURL() (string, error) {
	var cookie string
	var err error
	var url string
	var mlArgs string
	var mlURL string
	var headers http.Header
	client := &http.Client{}

	url = fmt.Sprintf("%s://%s/%s/export", proto, apiHost, p.LeagueYear)
	mlArgs = fmt.Sprintf("TYPE=myleagues&JSON=%s&APIKEY=%s", p.SetJSON, p.APIKey)
	mlURL = fmt.Sprintf("%s?%s", url, mlArgs)

	if p.APIKey == "notfound" {
		cookie, err = p.GetCookie(client)
		if err != nil {
			// fmt.Printf("Error getting cookie: %v\n", err)
			return "", err
		}

		url = fmt.Sprintf("%s://%s/%s/export", proto, apiHost, p.LeagueYear)
		headers = http.Header{}
		headers.Add("Cookie", fmt.Sprintf("MFL_USER_ID=%s", cookie))
		mlArgs = fmt.Sprintf("TYPE=myleagues&JSON=%s", p.SetJSON)
		mlURL = fmt.Sprintf("%s?%s", url, mlArgs)
	}
	// client := &http.Client{}

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

	leagueHostRegex := regexp.MustCompile(`url="(https?)://([a-z0-9]+.myfantasyleague.com)/` + p.LeagueYear + `/home/` + p.LeagueID + `"`)
	leagueMatches := leagueHostRegex.FindStringSubmatch(string(mlBody))
	if len(leagueMatches) < 1 {
		return "", fmt.Errorf("no league host found in response")
	}
	protocol := leagueMatches[1]
	leagueHost := leagueMatches[2]
	// fmt.Printf("Got league host %s\n", leagueHost)
	url = fmt.Sprintf("%s://%s/%s/export", protocol, leagueHost, p.LeagueYear)
	// fmt.Println("The value of url is:")
	// fmt.Println(url)

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

	// fmt.Printf("the value of MFLParams is: %v\n", p)
	fmt.Printf("the value of username is %s\n", p.UserName)
	loginURL := fmt.Sprintf("https://%s/%s/login?USERNAME=%s&PASSWORD=%s&XML=%s", apiHost, p.LeagueYear, p.UserName, p.Password, p.SetXML)
	// fmt.Printf("Making request to get cookie: %s\n", loginURL)
	loginResp, err := client.Get(loginURL)
	if err != nil {
		// fmt.Println("in the loginResp error check")
		return "", fmt.Errorf("error making login request: %v", err)
	}
	defer loginResp.Body.Close()

	body, err := io.ReadAll(loginResp.Body)
	if err != nil {
		// fmt.Println("in the body error check")
		return "", fmt.Errorf("error reading login response: %v", err)
	}

	cookieRegex := regexp.MustCompile(`MFL_USER_ID="([^"]*)">OK`)
	// fmt.Printf("value of cookieRegex is: %v", cookieRegex)
	matches := cookieRegex.FindStringSubmatch(string(body))
	if len(matches) < 1 {
		fmt.Println("len(matches) is less than 1")
		// return "", fmt.Errorf("cannot get login cookie. Response: %s", string(body))
	}
	cookie := matches[1]
	// fmt.Printf("in GetCookie function and the value of cookie is: %s", cookie)
	return cookie, nil
}

func ConvertStringToInt(input string, fallback int) int {
	num, err := strconv.Atoi(input)
	if err != nil || input == "" {
		// Log or handle the error as needed
		fmt.Printf("Warning: could not convert '%s' to int: %v\n", input, err)
		return fallback
	}
	return num
}

func defaultIfEmpty(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}
