package store

import (
	"context"
	"fmt"
	"strconv"
	"time"

	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"pfr-weekly/internal/pfr"
)

type DynamoDBAPI interface {
	BatchWriteItem(ctx context.Context, params *ddb.BatchWriteItemInput, optFns ...func(*ddb.Options)) (*ddb.BatchWriteItemOutput, error)
}

// PutRows writes rows to DynamoDB in batches of 25 with retries for UnprocessedItems.
func PutRows(ctx context.Context, c DynamoDBAPI, tableName, season string, rows []pfr.PlayerRow) error {
	if len(rows) == 0 {
		return nil
	}

	const maxBatch = 25
	for i := 0; i < len(rows); i += maxBatch {
		end := i + maxBatch
		if end > len(rows) {
			end = len(rows)
		}
		chunk := rows[i:end]

		wrs := make([]types.WriteRequest, 0, len(chunk))
		for _, r := range chunk {
			item := map[string]types.AttributeValue{
				"Season": &types.AttributeValueMemberS{Value: season},
				"Player": &types.AttributeValueMemberS{Value: r.Player},
				"Team":   &types.AttributeValueMemberS{Value: r.Team},
				"Teams":  &types.AttributeValueMemberS{Value: r.Teams},
				"Age":    &types.AttributeValueMemberN{Value: strconv.Itoa(r.Age)},
				"G":      &types.AttributeValueMemberN{Value: strconv.Itoa(r.G)},
				"GS":     &types.AttributeValueMemberN{Value: strconv.Itoa(r.GS)},
				"Pos":    &types.AttributeValueMemberS{Value: r.Pos},
			}
			wrs = append(wrs, types.WriteRequest{PutRequest: &types.PutRequest{Item: item}})
		}

		unprocessed := map[string][]types.WriteRequest{tableName: wrs}
		backoff := 100 * time.Millisecond
		for attempt := 0; attempt < 8 && len(unprocessed) > 0; attempt++ {
			out, err := c.BatchWriteItem(ctx, &ddb.BatchWriteItemInput{
				RequestItems: unprocessed,
			})
			if err != nil {
				return err
			}
			if len(out.UnprocessedItems) == 0 {
				break
			}
			unprocessed = out.UnprocessedItems
			time.Sleep(backoff)
			backoff *= 2
			if backoff > 3*time.Second {
				backoff = 3 * time.Second
			}
		}
		if len(unprocessed) > 0 {
			return fmt.Errorf("unprocessed items remain after retries")
		}
	}

	return nil
}
