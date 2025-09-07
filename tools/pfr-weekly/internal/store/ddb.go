package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"pfr-weekly/internal/pfr"
)

// Minimal interface for easy testing
type DynamoDBAPI interface {
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}

// Defensive players (materialized view): PK=Season (S), SK=Player (S)
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
			item := map[string]types.AttributeValue{
				"Season":    &types.AttributeValueMemberS{Value: season},
				"Player":    &types.AttributeValueMemberS{Value: r.Player},
				"Team":      &types.AttributeValueMemberS{Value: r.Team},
				"Teams":     &types.AttributeValueMemberS{Value: r.Teams},
				"Age":       &types.AttributeValueMemberN{Value: strconv.Itoa(r.Age)},
				"G":         &types.AttributeValueMemberN{Value: strconv.Itoa(r.G)},
				"GS":        &types.AttributeValueMemberN{Value: strconv.Itoa(r.GS)},
				"Pos":       &types.AttributeValueMemberS{Value: r.Pos},
				"UpdatedAt": &types.AttributeValueMemberN{Value: now},
			}
			reqs = append(reqs, types.WriteRequest{
				PutRequest: &types.PutRequest{Item: item},
			})
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
				// skip incomplete keys
				continue
			}
			sk := r.PlayerID + "#" + r.Team
			item := map[string]types.AttributeValue{
				"Season":    &types.AttributeValueMemberS{Value: r.Season},
				"SK":        &types.AttributeValueMemberS{Value: sk},
				"Player":    &types.AttributeValueMemberS{Value: r.Player},
				"PlayerID":  &types.AttributeValueMemberS{Value: r.PlayerID},
				"Team":      &types.AttributeValueMemberS{Value: r.Team},
				"Age":       &types.AttributeValueMemberN{Value: strconv.Itoa(r.Age)},
				"Pos":       &types.AttributeValueMemberS{Value: r.Pos},
				"G":         &types.AttributeValueMemberN{Value: strconv.Itoa(r.G)},
				"GS":        &types.AttributeValueMemberN{Value: strconv.Itoa(r.GS)},
				"UpdatedAt": &types.AttributeValueMemberN{Value: now},
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
