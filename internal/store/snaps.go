package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	// update to your module path
	"github.com/tyler180/fantasy-football-backends/internal/pfr"
)

// Env helpers
func envStr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

// Get key attribute names from env (with safe defaults).
//
//	SNAPS_PK_ATTR (default "SeasonTeamWeek")
//	SNAPS_SK_ATTR (default "PlayerID")
func snapsKeyAttrNames() (string, string) {
	pk := envStr("SNAPS_PK_ATTR", "SeasonTeamWeek")
	sk := envStr("SNAPS_SK_ATTR", "PlayerID")
	return pk, sk
}

// PutSnapGameRows upserts per-game defensive snap percentages into the snaps table.
//
// Default key schema (override via env):
//
//	PK (SNAPS_PK_ATTR) = SeasonTeamWeek (e.g., "2024#SEA#01")
//	SK (SNAPS_SK_ATTR) = PlayerID      (e.g., "AdauJa00")
//
// Recommended GSI "PlayerGames":
//
//	PK: PlayerID (S)
//	SK: SeasonWeek (S)
//
// De-duplicates by (Season,Team,Week,PlayerID) to avoid duplicate-key ValidationException.
func PutSnapGameRows(ctx context.Context, ddb *dynamodb.Client, tableName string, rows []pfr.SnapGameRow) error {
	if len(rows) == 0 {
		return nil
	}

	pkAttr, skAttr := snapsKeyAttrNames()

	// De-duplicate in-memory
	type key struct {
		season string
		team   string
		week   int
		pid    string
	}
	seen := make(map[key]struct{}, len(rows))

	wreqs := make([]types.WriteRequest, 0, len(rows))
	for _, r := range rows {
		if r.Season == "" || r.Team == "" || r.Week <= 0 || r.PlayerID == "" {
			continue // skip incomplete
		}
		k := key{season: r.Season, team: r.Team, week: r.Week, pid: r.PlayerID}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}

		item := buildSnapItem(r, pkAttr, skAttr)
		wreqs = append(wreqs, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})
	}

	if len(wreqs) == 0 {
		return nil
	}
	return batchWriteAll(ctx, ddb, tableName, wreqs)
}

// buildSnapItem converts a SnapGameRow to a DynamoDB item and uses the provided key attribute names.
func buildSnapItem(r pfr.SnapGameRow, pkAttr, skAttr string) map[string]types.AttributeValue {
	seasonTeamWeek := fmt.Sprintf("%s#%s#%02d", r.Season, r.Team, r.Week) // e.g., "2024#SEA#01"
	seasonWeek := fmt.Sprintf("%s#%02d", r.Season, r.Week)                // e.g., "2024#01"
	playerKey := r.PlayerID

	item := map[string]types.AttributeValue{
		// Primary key — names come from env to match table schema
		pkAttr: &types.AttributeValueMemberS{Value: seasonTeamWeek},
		skAttr: &types.AttributeValueMemberS{Value: playerKey},

		// Common attributes (safe to include regardless of key names)
		"Season":     &types.AttributeValueMemberS{Value: r.Season},
		"Team":       &types.AttributeValueMemberS{Value: r.Team},
		"Week":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", r.Week)},
		"Player":     &types.AttributeValueMemberS{Value: r.Player},
		"Pos":        &types.AttributeValueMemberS{Value: r.Pos},
		"DefSnapPct": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.2f", r.DefSnapPct)},

		// GSI for PlayerGames
		"PlayerID":   &types.AttributeValueMemberS{Value: r.PlayerID},
		"SeasonWeek": &types.AttributeValueMemberS{Value: seasonWeek},
	}

	// Also store SeasonTeamWeek as a named attribute if it's not already your PK attribute
	if pkAttr != "SeasonTeamWeek" {
		item["SeasonTeamWeek"] = &types.AttributeValueMemberS{Value: seasonTeamWeek}
	}

	return item
}

// batchWriteAll writes in chunks of 25 with exponential backoff for UnprocessedItems.
func batchWriteAll(ctx context.Context, ddb *dynamodb.Client, table string, reqs []types.WriteRequest) error {
	const chunk = 25
	for start := 0; start < len(reqs); start += chunk {
		end := start + chunk
		if end > len(reqs) {
			end = len(reqs)
		}
		batch := reqs[start:end]

		requestItems := map[string][]types.WriteRequest{
			table: batch,
		}

		backoff := 100 * time.Millisecond
		for attempt := 1; ; attempt++ {
			out, err := ddb.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
				RequestItems: requestItems,
			})
			if err != nil {
				if attempt >= 8 {
					return fmt.Errorf("batch write (attempt %d): %w", attempt, err)
				}
				time.Sleep(backoff)
				backoff = nextBackoff(backoff)
				continue
			}
			if len(out.UnprocessedItems) == 0 || len(out.UnprocessedItems[table]) == 0 {
				break
			}
			// retry only the unprocessed ones
			requestItems = out.UnprocessedItems
			if attempt >= 8 {
				return fmt.Errorf("batch write: exhausted retries; %d unprocessed remain", len(out.UnprocessedItems[table]))
			}
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
		}
	}
	return nil
}

func nextBackoff(cur time.Duration) time.Duration {
	cur *= 2
	if cur > 2*time.Second {
		return 2 * time.Second
	}
	return cur
}
