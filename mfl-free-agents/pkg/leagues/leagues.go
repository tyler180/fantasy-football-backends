package leagues

import (
	"os"

	"github.com/tyler180/fantasy-football-backends/mfl-free-agents/pkg/common"
)

type Leagues struct {
	Version  string `json:"version"`
	Encoding string `json:"encoding"`
}

type LeagueHTTPResponse struct {
	Leagues struct {
		League []League `json:"league"`
	} `json:"leagues"`
	Version  string `json:"version"`
	Encoding string `json:"encoding"`
}

type League struct {
	LeagueID      string `json:"league_id"`
	Name          string `json:"name"`
	FranchiseID   string `json:"franchise_id"`
	URL           string `json:"url"`
	FranchiseName string `json:"franchise_name"`
}

func GetLeagues() (LeagueHTTPResponse, error) {

	secretName := os.Getenv("SECRET_NAME")
	baseURL := common.NewBaseURL("https", "api.myfantasyleague.com", "myleagues")
}

// https://api.myfantasyleague.com/2024/export?TYPE=myleagues&YEAR=2024&FRANCHISE_NAMES=1&JSON=1
