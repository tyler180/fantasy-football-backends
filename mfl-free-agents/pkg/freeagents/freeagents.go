package freeagents

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/tyler180/retrieve-secret/retrievesecrets"
)

const (
	proto   = "https"
	apiHost = "www49.myfantasyleague.com"
	reqType = "league"
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

type FreeAgent struct {
	PlayerID string `dynamodbav:"playerID"` // Maps to the "playerID" attribute in DynamoDB
	Name     string `dynamodbav:"name"`     // Example: Additional attribute
	Team     string `dynamodbav:"team"`     // Example: Additional attribute
	Position string `dynamodbav:"position"` // Maps to the "position" attribute (used in GSI)
}

type FAHTTPResponse struct {
	FreeAgents struct {
		LeagueUnit struct {
			Player []FreeAgentPlayer `json:"player"` // Use []interface{} if the player data is dynamic or unknown
			Unit   string            `json:"unit"`
		} `json:"leagueUnit"`
	} `json:"freeAgents"`
	Version  string `json:"version"`
	Encoding string `json:"encoding"`
}

type FreeAgentPlayer struct {
	ContractInfo string `json:"contractInfo"`
	Salary       string `json:"salary"`
	ID           string `json:"id"`
	ContractYear string `json:"contractYear"`
}

type FAParams struct {
	APIKey   string
	ReqType  string
	LeagueID string
	Position string
	JSON     string
}

func NewFAParams(ctx context.Context, position, secretName string) (*FAParams, error) {
	secretData, err := retrievesecrets.RetrieveSecret(ctx, secretName, "json", "")
	if err != nil {
		// fmt.Println("getting the secretData using retrievesecrets.RetrieveSecret is failing")
		return nil, err
	}

	return &FAParams{
		APIKey:   secretData["api_key"],
		ReqType:  "freeAgents",
		LeagueID: secretData["league_id"],
		Position: position,
		JSON:     "1",
	}, nil
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

// func (fap *FAParams) FreeAgents(ctx context.Context) error {
// 	baseURL := common.NewBaseURL("https", "www49.myfantasyleague.com", "freeAgents")
// 	return nil
// }

func (agent FreeAgent) AddFreeAgent(ctx context.Context) error {
	// Load the AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create a DynamoDB client
	svc := dynamodb.NewFromConfig(cfg)

	// Convert the struct to a map of DynamoDB attribute values
	item, err := attributevalue.MarshalMap(agent)
	if err != nil {
		return fmt.Errorf("failed to marshal FreeAgent struct: %w", err)
	}

	// Define the PutItemInput
	input := &dynamodb.PutItemInput{
		TableName: aws.String("mfl_free_agents"),
		Item:      item,
	}

	// Put the item into the DynamoDB table
	_, err = svc.PutItem(context.TODO(), input)
	if err != nil {
		return fmt.Errorf("failed to put item into DynamoDB: %w", err)
	}

	log.Println("Successfully added free agent:", agent.PlayerID)
	return nil
}

// func FreeAgents(ctx context.Context, mp common.MFLParams, cookie string, position ...string) error {
// 	client := &http.Client{}
// 	var args string
// 	// cookie, err := cmd.GetCookie(client)
// 	// if err != nil {
// 	// 	return fmt.Errorf("error getting cookie: %v", err)
// 	// }

// 	// https://www49.myfantasyleague.com/2024/export?TYPE=freeAgents&L=79286&APIKEY=ahBi2siVvuWrx1OmP1DDaTQeELox&POSITION=QB&JSON=1

// 	url := fmt.Sprintf("%s://%s/%s/export", proto, apiHost, mp.LeagueYear)
// 	headers := http.Header{}
// 	headers.Add("Cookie", fmt.Sprintf("MFL_USER_ID=%s", cookie))
// 	args = fmt.Sprintf("TYPE=freeAgents&L=%s&W=&JSON=%d", mp.LeagueID, mp.SetJSON)
// 	if len(position) > 0 {
// 		args = fmt.Sprintf("TYPE=freeAgents&L=%s&W=&POS=%s&JSON=%d", mp.LeagueID, position, mp.SetJSON)
// 		fmt.Printf("the FreeAgents position variable is longer than 0 at: %d\n", len(position))
// 	}
// 	mlURL := fmt.Sprintf("%s?%s", url, args)

// 	req, err := http.NewRequest("GET", mlURL, nil)
// 	if err != nil {
// 		return fmt.Errorf("error creating request: %v", err)
// 	}
// 	req.Header = headers

// 	mlResp, err := client.Do(req)
// 	if err != nil {
// 		return fmt.Errorf("error making league request: %v", err)
// 	}
// 	defer mlResp.Body.Close()

// 	mlBody, err := io.ReadAll(mlResp.Body)
// 	if err != nil {
// 		return fmt.Errorf("error reading league response: %v", err)
// 	}

// 	leagueHostRegex := regexp.MustCompile(`url="(https?)://([a-z0-9]+.myfantasyleague.com)/` + mp.LeagueYear + `/home/` + mp.LeagueID + `"`)
// 	leagueMatches := leagueHostRegex.FindStringSubmatch(string(mlBody))
// 	if len(leagueMatches) < 3 {
// 		fmt.Printf("In the players package. Cannot find league host in response: %s\n", string(mlBody))
// 		return nil
// 	}
// 	protocol := leagueMatches[1]
// 	leagueHost := leagueMatches[2]
// 	fmt.Printf("Got league host %s\n", leagueHost)
// 	url = fmt.Sprintf("%s://%s/%s/export", protocol, leagueHost, mp.LeagueYear)
// 	fmt.Println(url)

// 	// Ensure the program ends cleanly
// 	fmt.Println("FreeAgents function completed successfully.")

// 	return nil
// }

//  https://www49.myfantasyleague.com/2024/top?L=79286&SEARCHTYPE=BASIC&COUNT=32&YEAR=2024&START_WEEK=1&END_WEEK=17&CATEGORY=freeagent&POSITION=QB%7CRB%7CWR%7CTE&DISPLAY=points&TEAM=*
