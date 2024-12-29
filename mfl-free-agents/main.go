package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/tyler180/retrieve-secret/retrievesecrets"
)

func main() {
	lambda.Start(lambdaHandler)
}

func lambdaHandler(ctx context.Context) {
	client := &http.Client{}

	KVPairs, err := retrievesecrets.RetrieveSecret(ctx)

	secretName := os.Getenv("SECRET_NAME")
	password := os.Getenv("PASSWORD")
	leagueID := os.Getenv("LEAGUE_ID")

	fmt.Println("lambdaHandler function before getting cookie")
	cookie, err := cmd.GetCookie(client, username, password)
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

	league_url, err := league.GetLeagueURL(cookie, leagueID)
	if err != nil {
		fmt.Printf("error getting the league_url %v", err)
	}

	fmt.Printf("Selected League ID: %s\n", leagueID)
	fmt.Printf("Selected League URL: %s\n", league_url)
	fmt.Println("Program completed successfully.")

	err = players.FreeAgents(leagueID, cookie, "")
	if err != nil {
		fmt.Printf("Error getting free agents: %v\n", err)
		return
	}
	fmt.Println("Program completed successfully.")
}
