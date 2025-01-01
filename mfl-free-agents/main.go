package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/tyler180/fantasy-football-backends/mfl-free-agents/common"
	"github.com/tyler180/fantasy-football-backends/mfl-free-agents/freeagents"
)

func main() {
	lambda.Start(lambdaHandler)
}

func lambdaHandler(ctx context.Context) {
	client := &http.Client{}

	secretName := os.Getenv("SECRET_NAME")
	// password := os.Getenv("PASSWORD")
	// leagueID := os.Getenv("LEAGUE_ID")

	mflParams, err := common.NewMFLParams(ctx, secretName, "1", "0")

	// username, err := retrievesecrets.RetrieveSecret(ctx, secretName, retrievesecrets.SecretTypeJSON, "username")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// password, err := retrievesecrets.RetrieveSecret(ctx, secretName, retrievesecrets.SecretTypeJSON, "password")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// league_id, err := retrievesecrets.RetrieveSecret(ctx, secretName, retrievesecrets.SecretTypeJSON, "league_id")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	fmt.Println("lambdaHandler function before getting cookie")
	cookie, err := mflParams.GetCookie(client)
	if err != nil {
		fmt.Printf("Error getting cookie: %v\n", err)
		return
	}
	fmt.Printf("Got cookie %s\n", cookie)

	// leagues, err := league.GetLeagueInfo(cookie)
	// if err != nil {
	// 	fmt.Printf("Error getting league IDs: %v\n", err)
	// 	return
	// }

	// var leagueID string
	// for _, l := range leagues {
	// 	fmt.Printf("League ID: %s, Name: %s, Franchise ID: %s, URL: %s\n", l.LeagueID, l.Name, l.FranchiseID, l.URL)
	// 	if strings.HasPrefix(l.Name, "I Paid What") {
	// 		leagueID = l.LeagueID
	// 	}
	// }

	// if leagueID == "" {
	// 	fmt.Println("No league found with name starting with 'I Paid What'")
	// 	return
	// }

	league_url, err := common.GetLeagueURL(cookie, mflParams.LeagueID)
	if err != nil {
		fmt.Printf("error getting the league_url %v", err)
	}

	fmt.Printf("Selected League ID: %s\n", mflParams.LeagueID)
	fmt.Printf("Selected League URL: %s\n", league_url)
	fmt.Println("Program completed successfully.")

	err = freeagents.FreeAgents(mflParams.LeagueID, cookie, "")
	if err != nil {
		fmt.Printf("Error getting free agents: %v\n", err)
		return
	}
	fmt.Println("Program completed successfully.")
}
