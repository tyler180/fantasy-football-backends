package store

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// LoadRosterPositions builds a name->pos map from nfl_roster_rows (by SeasonTeam partitions).
// Uses normalized names to improve matching.
func LoadRosterPositions(ctx context.Context, ddb *dynamodb.Client, rosterTable, season string, pfrTeams []string) (map[string]string, error) {
	namePos := make(map[string]string, 4096)
	for _, team := range pfrTeams {
		st := season + "#" + team
		out, err := ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(rosterTable),
			KeyConditionExpression: aws.String("#pk = :v"),
			ExpressionAttributeNames: map[string]string{
				"#pk": "SeasonTeam",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":v": &types.AttributeValueMemberS{Value: st},
			},
			ProjectionExpression: aws.String("Player, Pos"),
		})
		if err != nil {
			return nil, err
		}
		for _, it := range out.Items {
			player := ""
			pos := ""
			if v, ok := it["Player"].(*types.AttributeValueMemberS); ok {
				player = v.Value
			}
			if v, ok := it["Pos"].(*types.AttributeValueMemberS); ok {
				pos = v.Value
			}
			nn := normName(player)
			pp := strings.ToUpper(strings.TrimSpace(pos))
			if nn != "" && pp != "" {
				namePos[nn] = pp
			}
		}
	}
	return namePos, nil
}
