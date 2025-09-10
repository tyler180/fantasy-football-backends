package store

import (
	"context"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var reSpace = regexp.MustCompile(`\s+`)

func normName(s string) string {
	// Uppercase, remove punctuation, collapse spaces
	up := strings.ToUpper(s)
	up = strings.NewReplacer(
		".", "", ",", "", "'", "", "`", "", "’", "",
		"-", " ", "–", " ", "—", " ",
		"(", "", ")", "",
	).Replace(up)
	up = reSpace.ReplaceAllString(strings.TrimSpace(up), " ")
	return up
}

// LoadPlayerPositions returns:
//
//	idPos[PlayerID] = Pos
//	namePos[normName(Player)] = Pos
func LoadPlayerPositions(ctx context.Context, ddb *dynamodb.Client, playersTable, season string, pfrTeams []string) (map[string]string, map[string]string, error) {
	idPos := make(map[string]string, 4096)
	namePos := make(map[string]string, 4096)

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
			ProjectionExpression: aws.String("PlayerID, Player, Pos"),
		})
		if err != nil {
			return nil, nil, err
		}
		for _, it := range out.Items {
			var pid, player, pos string
			if v, ok := it["PlayerID"].(*types.AttributeValueMemberS); ok {
				pid = strings.TrimSpace(v.Value)
			}
			if v, ok := it["Player"].(*types.AttributeValueMemberS); ok {
				player = strings.TrimSpace(v.Value)
			}
			if v, ok := it["Pos"].(*types.AttributeValueMemberS); ok {
				pos = strings.ToUpper(strings.TrimSpace(v.Value))
			}
			if pos == "" {
				continue
			}
			if pid != "" {
				idPos[pid] = pos
			}
			if player != "" {
				namePos[normName(player)] = pos
			}
		}
	}
	return idPos, namePos, nil
}
