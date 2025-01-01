package freeagents

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Player represents the structure of a football player
type Player struct {
	PlayerID            string  `json:"playerID"`
	PlayerName          string  `json:"playerName"`
	Position            string  `json:"position"`
	Age                 int     `json:"age"`
	AverageFantasyScore float64 `json:"averageFantasyScore"`
	TotalFantasyPoints  int     `json:"totalFantasyPoints"`
}

// LoadPlayers loads player data from a JSON file
func LoadPlayers(filename string) ([]Player, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var players []Player
	if err := json.NewDecoder(file).Decode(&players); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}
	return players, nil
}

// AddPlayerToDynamoDB adds a single player to the DynamoDB table
func AddPlayerToDynamoDB(ctx context.Context, client *dynamodb.Client, tableName string, player Player) error {
	_, err := client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &tableName,
		Item: map[string]types.AttributeValue{
			"playerID":            &types.AttributeValueMemberS{Value: player.PlayerID},
			"playerName":          &types.AttributeValueMemberS{Value: player.PlayerName},
			"position":            &types.AttributeValueMemberS{Value: player.Position},
			"age":                 &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", player.Age)},
			"averageFantasyScore": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", player.AverageFantasyScore)},
			"totalFantasyPoints":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", player.TotalFantasyPoints)},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to put item: %w", err)
	}
	return nil
}

func FreeAgents(league_id, cookie string, position ...string) error {
	client := &http.Client{}
	var args string
	// cookie, err := cmd.GetCookie(client)
	// if err != nil {
	// 	return fmt.Errorf("error getting cookie: %v", err)
	// }

	url := fmt.Sprintf("%s://%s/%s/export", proto, apiHost, year)
	headers := http.Header{}
	headers.Add("Cookie", fmt.Sprintf("MFL_USER_ID=%s", cookie))
	args = fmt.Sprintf("TYPE=freeAgents&L=%s&W=&JSON=%d", league_id, json)
	if len(position) > 0 {
		args = fmt.Sprintf("TYPE=freeAgents&L=%s&W=&POS=%s&JSON=%d", league_id, position, json)
	}
	mlURL := fmt.Sprintf("%s?%s", url, args)

	req, err := http.NewRequest("GET", mlURL, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}
	req.Header = headers

	mlResp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making league request: %v", err)
	}
	defer mlResp.Body.Close()

	mlBody, err := io.ReadAll(mlResp.Body)
	if err != nil {
		return fmt.Errorf("error reading league response: %v", err)
	}

	leagueHostRegex := regexp.MustCompile(`url="(https?)://([a-z0-9]+.myfantasyleague.com)/` + year + `/home/` + leagueID + `"`)
	leagueMatches := leagueHostRegex.FindStringSubmatch(string(mlBody))
	if len(leagueMatches) < 3 {
		fmt.Printf("In the players package. Cannot find league host in response: %s\n", string(mlBody))
		return nil
	}
	protocol := leagueMatches[1]
	leagueHost := leagueMatches[2]
	fmt.Printf("Got league host %s\n", leagueHost)
	url = fmt.Sprintf("%s://%s/%s/export", protocol, leagueHost, year)
	fmt.Println(url)

	// Ensure the program ends cleanly
	fmt.Println("Program completed successfully.")

	return nil
}

//  https://www49.myfantasyleague.com/2024/top?L=79286&SEARCHTYPE=BASIC&COUNT=32&YEAR=2024&START_WEEK=1&END_WEEK=17&CATEGORY=freeagent&POSITION=QB%7CRB%7CWR%7CTE&DISPLAY=points&TEAM=*
