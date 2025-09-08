package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	ddb "github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/tyler180/fantasy-football-backends/internal/pfr"
)

// fake client implementing DynamoDBAPI
type fakeDDB struct {
	calls int
	// simulate first attempt returning unprocessed, second succeeds
	failFirst bool
}

func (f *fakeDDB) BatchWriteItem(ctx context.Context, in *ddb.BatchWriteItemInput, _ ...func(*ddb.Options)) (*ddb.BatchWriteItemOutput, error) {
	f.calls++
	if f.failFirst {
		f.failFirst = false
		// Echo back all as unprocessed to force a retry
		return &ddb.BatchWriteItemOutput{
			UnprocessedItems: in.RequestItems,
		}, nil
	}
	// Success (no unprocessed)
	return &ddb.BatchWriteItemOutput{}, nil
}

func (f *fakeDDB) UpdateItem(ctx context.Context, in *ddb.UpdateItemInput, _ ...func(*ddb.Options)) (*ddb.UpdateItemOutput, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestPutRows_BatchingAndRetry(t *testing.T) {
	// build 30 rows → 25 + 5 batches
	var rows []pfr.PlayerRow
	for i := 0; i < 30; i++ {
		rows = append(rows, pfr.PlayerRow{
			Player: fmt.Sprintf("P%02d", i),
			Team:   "ATL",
			Teams:  "ATL",
			Age:    23,
			G:      1,
			GS:     1,
			Pos:    "CB",
		})
	}

	// speed up test (cap backoff sleeps); using context with timeout just in case
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	fc := &fakeDDB{failFirst: true}
	err := PutRows(ctx, fc, "tbl", "2025", rows)
	if err != nil {
		t.Fatalf("PutRows error: %v", err)
	}

	// Each batch is attempted twice (one retry), and there are 2 batches.
	if fc.calls != 4 {
		t.Fatalf("expected 4 BatchWriteItem calls (2 batches x 2 attempts), got %d", fc.calls)
	}
}
