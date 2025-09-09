package store

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// LoadPlayerPositions builds a map[PlayerID]Pos by querying each SeasonTeam partition.
func LoadPlayerPositions(ctx context.Context, ddb *dynamodb.Client, playersTable, season string, pfrTeams []string) (map[string]string, error) {
	posMap := make(map[string]string, 4096)
	for _, team := range pfrTeams {
		st := season + "#" + team
		out, err := ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(playersTable),
			KeyConditionExpression: aws.String("#pk = :v"),
			ExpressionAttributeNames: map[string]string{
				"#pk": "SeasonTeam",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":v": &types.AttributeValueMemberS{Value: st},
			},
			ProjectionExpression: aws.String("PlayerID, Pos"),
		})
		if err != nil {
			return nil, err
		}
		for _, it := range out.Items {
			pid, ok1 := it["PlayerID"].(*types.AttributeValueMemberS)
			pos, ok2 := it["Pos"].(*types.AttributeValueMemberS)
			if ok1 && ok2 {
				p := strings.ToUpper(strings.TrimSpace(pos.Value))
				if p != "" {
					posMap[pid.Value] = p
				}
			}
		}
	}
	return posMap, nil
}
