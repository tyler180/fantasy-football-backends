package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

const baseUrl = "https://api.sportradar.com/nfl/official/trial/v7/en/league/"

func main() {
	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		log.Fatalf("Error loading AWS configuration: %v", err)
	}

	// Initialize DynamoDB client
	dynamoDBSvc := dynamodb.NewFromConfig(cfg)

	// DynamoDB table name (from environment variable)
	tableName := os.Getenv("DYNAMODB_TABLE")
	if tableName == "" {
		log.Fatalf("DYNAMODB_TABLE environment variable is not set")
	}

	// Create the HandlerConfig
	config := &HandlerConfig{
		DynamoDBSvc: dynamoDBSvc,
		TableName:   tableName,
	}

	// Start the Lambda handler
	lambda.Start(config.Handler)
}

func handler() {

	API_KEY := (os.Getenv("AWS_REGION"))

	urls := []string{"hierarchy", "teams", "players"}

	for _, url := range urls {
		urlCombined := GetLeagueHierarchy(url, API_KEY)
		fmt.Printf("urlCombined is: %s\n", urlCombined)
	}

	// hierarchyUrl := "https://api.sportradar.com/nfl/official/trial/v7/en/league/hierarchy.json"
	// baseUrlTeams := "https://api.sportradar.com/nfl/official/trial/v7/en/league/teams.json"
	// playerProfileUrl := "https://api.sportradar.com/nfl/official/trial/v7/en/players/11cad59d-90dd-449c-a839-dddaba4fe16c/profile.json"

	// Construct the URL
	// urlTeams := baseUrlTeams + "?api_key=" + API_KEY
	// urlPlayerProfile := baseURLPlayerProfile + "?api_key=" + API_KEY

	req, _ := http.NewRequest("GET", urlTeams, nil)

	req.Header.Add("accept", "application/json")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	fmt.Println(string(body))

}

func GetLeagueHierarchy(url, apiKey string) string {
	urlCombined := baseUrl + url + ".json?api_key" + apiKey
	return fmt.Sprintf(urlCombined)
}
