package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/tyler180/fantasy-football-backends/internal/pfr"
)

type DynamoDBAPI interface {
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func PutRows(ctx context.Context, ddb DynamoDBAPI, tableName, season string, rows []pfr.PlayerRow) error {
	if len(rows) == 0 {
		return nil
	}
	const maxBatch = 25
	now := strconv.FormatInt(time.Now().Unix(), 10)

	for i := 0; i < len(rows); i += maxBatch {
		end := i + maxBatch
		if end > len(rows) {
			end = len(rows)
		}

		reqs := make([]types.WriteRequest, 0, end-i)
		for _, r := range rows[i:end] {
			if r.PlayerID == "" || r.Team == "" {
				continue
			}
			item := map[string]types.AttributeValue{
				"SeasonTeam":   &types.AttributeValueMemberS{Value: season + "#" + r.Team}, // PK
				"PlayerID":     &types.AttributeValueMemberS{Value: r.PlayerID},            // SK
				"Season":       &types.AttributeValueMemberS{Value: season},
				"Team":         &types.AttributeValueMemberS{Value: r.Team},
				"Player":       &types.AttributeValueMemberS{Value: r.Player}, // display name
				"Teams":        &types.AttributeValueMemberS{Value: r.Teams},
				"Age":          &types.AttributeValueMemberN{Value: strconv.Itoa(r.Age)},
				"G":            &types.AttributeValueMemberN{Value: strconv.Itoa(r.G)},
				"GS":           &types.AttributeValueMemberN{Value: strconv.Itoa(r.GS)},
				"Pos":          &types.AttributeValueMemberS{Value: r.Pos},
				"DefSnapNum":   &types.AttributeValueMemberN{Value: strconv.Itoa(r.DefSnapNum)},
				"DefSnapPct":   &types.AttributeValueMemberN{Value: strconv.FormatFloat(r.DefSnapPct, 'f', 1, 64)},
				"TeamPlayerID": &types.AttributeValueMemberS{Value: r.Team + "#" + r.PlayerID},
				"UpdatedAt":    &types.AttributeValueMemberN{Value: now},
			}
			reqs = append(reqs, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
		}
		if len(reqs) == 0 {
			continue
		}
		if err := batchWriteWithRetry(ctx, ddb, tableName, reqs); err != nil {
			return fmt.Errorf("batch write defensive rows: %w", err)
		}
	}
	return nil
}

// Raw roster rows: PK=Season (S), SK=PlayerID#Team (S)
func PutRosterRows(ctx context.Context, ddb DynamoDBAPI, table string, rows []pfr.RosterRow) error {
	if len(rows) == 0 {
		return nil
	}
	const maxBatch = 25
	now := strconv.FormatInt(time.Now().Unix(), 10)

	for i := 0; i < len(rows); i += maxBatch {
		end := i + maxBatch
		if end > len(rows) {
			end = len(rows)
		}

		reqs := make([]types.WriteRequest, 0, end-i)
		for _, r := range rows[i:end] {
			if r.PlayerID == "" || r.Team == "" || r.Season == "" {
				continue
			}
			sk := r.PlayerID + "#" + r.Team
			item := map[string]types.AttributeValue{
				"Season":     &types.AttributeValueMemberS{Value: r.Season},
				"SK":         &types.AttributeValueMemberS{Value: sk},
				"Player":     &types.AttributeValueMemberS{Value: r.Player},
				"PlayerID":   &types.AttributeValueMemberS{Value: r.PlayerID},
				"Team":       &types.AttributeValueMemberS{Value: r.Team},
				"Age":        &types.AttributeValueMemberN{Value: strconv.Itoa(r.Age)},
				"Pos":        &types.AttributeValueMemberS{Value: r.Pos},
				"G":          &types.AttributeValueMemberN{Value: strconv.Itoa(r.G)},
				"GS":         &types.AttributeValueMemberN{Value: strconv.Itoa(r.GS)},
				"DefSnapNum": &types.AttributeValueMemberN{Value: strconv.Itoa(r.DefSnapNum)},
				"DefSnapPct": &types.AttributeValueMemberN{Value: strconv.FormatFloat(r.DefSnapPct, 'f', 1, 64)},
				"UpdatedAt":  &types.AttributeValueMemberN{Value: now},
			}
			reqs = append(reqs, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
		}
		if len(reqs) == 0 {
			continue
		}
		if err := batchWriteWithRetry(ctx, ddb, table, reqs); err != nil {
			return fmt.Errorf("batch write roster rows: %w", err)
		}
	}
	return nil
}

func batchWriteWithRetry(ctx context.Context, ddb DynamoDBAPI, table string, reqs []types.WriteRequest) error {
	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{table: reqs},
	}
	const maxAttempts = 6
	backoff := 120 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, err := ddb.BatchWriteItem(ctx, input)
		if err != nil {
			return err
		}
		if len(out.UnprocessedItems) == 0 {
			return nil
		}
		input.RequestItems = out.UnprocessedItems
		time.Sleep(backoff)
		if backoff < 2*time.Second {
			backoff += 120 * time.Millisecond
		}
	}
	return fmt.Errorf("unprocessed items remained after retries for table %s", table)
}

// func PutSnapGameRows(ctx context.Context, ddb DynamoDBAPI, table string, rows []pfr.SnapGameRow) error {
// 	if len(rows) == 0 {
// 		return nil
// 	}
// 	const maxBatch = 25
// 	now := strconv.FormatInt(time.Now().Unix(), 10)

// 	for i := 0; i < len(rows); i += maxBatch {
// 		end := i + maxBatch
// 		if end > len(rows) {
// 			end = len(rows)
// 		}

// 		reqs := make([]types.WriteRequest, 0, end-i)
// 		for _, r := range rows[i:end] {
// 			stw := fmt.Sprintf("%s#%s#%02d", r.Season, r.Team, r.Week)
// 			sw := fmt.Sprintf("%s#%02d", r.Season, r.Week)
// 			item := map[string]types.AttributeValue{
// 				"SeasonTeamWeek": &types.AttributeValueMemberS{Value: stw},
// 				"PlayerID":       &types.AttributeValueMemberS{Value: r.PlayerID},
// 				"Season":         &types.AttributeValueMemberS{Value: r.Season},
// 				"Team":           &types.AttributeValueMemberS{Value: r.Team},
// 				"Week":           &types.AttributeValueMemberN{Value: strconv.Itoa(r.Week)},
// 				"Player":         &types.AttributeValueMemberS{Value: r.Player},
// 				"DefSnapPct":     &types.AttributeValueMemberN{Value: strconv.FormatFloat(r.DefSnapPct, 'f', 1, 64)},
// 				"SeasonWeek":     &types.AttributeValueMemberS{Value: sw}, // for PlayerGames GSI
// 				"UpdatedAt":      &types.AttributeValueMemberN{Value: now},
// 			}
// 			reqs = append(reqs, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}})
// 		}
// 		if err := batchWriteWithRetry(ctx, ddb, table, reqs); err != nil {
// 			return fmt.Errorf("batch write snap rows: %w", err)
// 		}
// 	}
// 	return nil
// }

func UpdatePlayerTrends(
	ctx context.Context,
	ddb DynamoDBAPI,
	table string,
	season string,
	team string,
	playerID string,
	last float64,
	slope3 float64,
	slope5 float64,
	change3 float64,
) error {
	key := map[string]types.AttributeValue{
		"SeasonTeam": &types.AttributeValueMemberS{Value: season + "#" + team}, // PK
		"PlayerID":   &types.AttributeValueMemberS{Value: playerID},            // SK
	}

	// format numbers (match how you write elsewhere)
	now := strconv.FormatInt(time.Now().Unix(), 10)
	vals := map[string]types.AttributeValue{
		":l":   &types.AttributeValueMemberN{Value: strconv.FormatFloat(last, 'f', 1, 64)},    // last game %
		":s3":  &types.AttributeValueMemberN{Value: strconv.FormatFloat(slope3, 'f', 3, 64)},  // slope over last 3
		":s5":  &types.AttributeValueMemberN{Value: strconv.FormatFloat(slope5, 'f', 3, 64)},  // slope over last 5
		":c3":  &types.AttributeValueMemberN{Value: strconv.FormatFloat(change3, 'f', 1, 64)}, // last - avg of prior 2 (in a 3-window)
		":now": &types.AttributeValueMemberN{Value: now},
	}

	_, err := ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:        aws.String(table),
		Key:              key,
		UpdateExpression: aws.String("SET DefSnapPctLast=:l, DefSnapPctSlope3=:s3, DefSnapPctSlope5=:s5, DefSnapPctChange3=:c3, UpdatedAt=:now"),
		// avoid creating new items accidentally
		ConditionExpression:       aws.String("attribute_exists(SeasonTeam) AND attribute_exists(PlayerID)"),
		ExpressionAttributeValues: vals,
	})
	return err
}
