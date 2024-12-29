package freeagents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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
