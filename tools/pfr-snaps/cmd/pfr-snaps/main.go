package main

import (
	"log"

	"github.com/aws/aws-lambda-go/lambda"

	// update to your module path
	appsnaps "github.com/tyler180/fantasy-football-backends/tools/pfr-snaps/internal/app/snaps"
)

func main() {
	log.SetFlags(0)
	lambda.Start(appsnaps.LambdaEntrypoint)
}
