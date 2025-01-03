package main

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/tyler180/fantasy-football-backends/mfl-free-agents/pkg/common"
	"github.com/tyler180/retrieve-secret/retrievesecrets"
)

func main() {
	lambda.Start(lambdaHandler)
}

func lambdaHandler(ctx context.Context) {
	// client := &http.Client{}

	secretName := os.Getenv("SECRET_NAME")
	fmt.Println("the value of secretName is:", secretName)

	secretsRetrieved, err := retrievesecrets.RetrieveSecret(ctx, "mfl-secrets20241228193911667400000001", "json", "")
	if err != nil {
		fmt.Printf("err is not nil and is: %s", err)
		//fmt.Errorf("failed to load AWS config %w", err)
		return
	}

	fmt.Printf("the value of secretsRetrieved is: %+v \n", secretsRetrieved)

	mflCookieParams, err := common.NewMFLParams(ctx, secretName)

	fmt.Printf("mflParams is: %+v\n", mflCookieParams)

	fmt.Println("lambdaHandler function before getting cookie")
	// cookie, err := mflCookieParams.GetCookie(client)
	// if err != nil {
	// 	fmt.Printf("Error getting cookie: %v\n", err)
	// 	return
	// }
	// fmt.Printf("Got cookie %s\n", cookie)

	league_url, err := mflCookieParams.GetLeagueURL()
	if err != nil {
		fmt.Printf("error getting the league_url %v", err)
	}

	fmt.Printf("Selected League ID: %s\n", mflCookieParams.LeagueID)
	fmt.Printf("Selected League URL: %s\n", league_url)
	// fmt.Println("Program completed successfully.")

	// err = freeagents.FreeAgents(ctx, *mflCookieParams, cookie, "QB")
	// if err != nil {
	// 	fmt.Printf("Error getting free agents: %v\n", err)
	// 	return
	// }
	// fmt.Println("Program completed successfully.")
}
