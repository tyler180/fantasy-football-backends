package store

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"pfr-weekly/internal/pfr"
)

type DynamoDBReadAPI interface {
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

// Read roster rows for a season → group by PlayerID → filter under-25 & G==GS → []PlayerRow
func MaterializeDefenseFromRoster(
	ctx context.Context,
	ddb DynamoDBReadAPI,
	rosterTable string,
	season string,
	defPos []string,
	maxAge int,
) ([]pfr.PlayerRow, error) {
	posAllow := make(map[string]struct{}, len(defPos))
	for _, p := range defPos {
		posAllow[strings.ToUpper(strings.TrimSpace(p))] = struct{}{}
	}

	type agg struct {
		Player   string
		PlayerID string
		AgeMin   int
		GSum     int
		GSSum    int
		PosSet   map[string]struct{}
		TeamGS   map[string]int
		TeamG    map[string]int
		Teams    map[string]struct{}
	}
	byPlayer := map[string]*agg{}

	var lastKey map[string]types.AttributeValue
	for {
		out, err := ddb.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(rosterTable),
			KeyConditionExpression: aws.String("#S = :s"),
			ExpressionAttributeNames: map[string]string{
				"#S": "Season",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":s": &types.AttributeValueMemberS{Value: season},
			},
			ExclusiveStartKey: lastKey,
		})
		if err != nil {
			return nil, err
		}

		for _, it := range out.Items {
			player := getStr(it, "Player")
			playerID := getStr(it, "PlayerID")
			team := getStr(it, "Team")
			age := getNum(it, "Age")
			pos := getStr(it, "Pos")
			g := getNum(it, "G")
			gs := getNum(it, "GS")

			if player == "" || playerID == "" {
				continue
			}
			if !posAllowed(posAllow, pos) {
				continue
			}

			a := byPlayer[playerID]
			if a == nil {
				a = &agg{
					Player:   player,
					PlayerID: playerID,
					AgeMin:   1 << 30,
					PosSet:   map[string]struct{}{},
					TeamGS:   map[string]int{},
					TeamG:    map[string]int{},
					Teams:    map[string]struct{}{},
				}
				byPlayer[playerID] = a
			}
			if age > 0 && age < a.AgeMin {
				a.AgeMin = age
			}
			a.GSum += g
			a.GSSum += gs
			for _, p := range splitCSV(pos) {
				if p != "" {
					a.PosSet[p] = struct{}{}
				}
			}
			if team != "" {
				a.TeamGS[team] += gs
				a.TeamG[team] += g
				a.Teams[team] = struct{}{}
			}
		}

		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		lastKey = out.LastEvaluatedKey
	}

	rows := make([]pfr.PlayerRow, 0, len(byPlayer))
	for _, a := range byPlayer {
		age := a.AgeMin
		if age == 1<<30 {
			age = 0
		}
		if age <= maxAge && a.GSum > 0 && a.GSum == a.GSSum {
			rows = append(rows, pfr.PlayerRow{
				Player: a.Player,
				Team:   pickPrimaryTeam(a.TeamGS, a.TeamG),
				Teams:  joinSortedKeys(a.Teams, ","),
				Age:    age,
				G:      a.GSum,
				GS:     a.GSSum,
				Pos:    joinSortedKeys(a.PosSet, ","),
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].Player < rows[j].Player })
	return rows, nil
}

// ---------- helpers (local to store) ----------

func getStr(m map[string]types.AttributeValue, key string) string {
	if v, ok := m[key]; ok {
		if s, ok2 := v.(*types.AttributeValueMemberS); ok2 {
			return s.Value
		}
	}
	return ""
}

func getNum(m map[string]types.AttributeValue, key string) int {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case *types.AttributeValueMemberN:
			n, _ := strconv.Atoi(t.Value)
			return n
		case *types.AttributeValueMemberS:
			n, _ := strconv.Atoi(t.Value)
			return n
		}
	}
	return 0
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.ToUpper(strings.TrimSpace(parts[i]))
	}
	return parts
}

func posAllowed(allow map[string]struct{}, pos string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, p := range splitCSV(pos) {
		if _, ok := allow[p]; ok {
			return true
		}
	}
	return false
}

func joinSortedKeys(set map[string]struct{}, sep string) string {
	if len(set) == 0 {
		return ""
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, sep)
}

// 1) highest GS, 2) then highest G, 3) then lexicographically
func pickPrimaryTeam(teamGS map[string]int, teamG map[string]int) string {
	best := ""
	bestGS, bestG := -1, -1
	for tm, gs := range teamGS {
		g := teamG[tm]
		if gs > bestGS || (gs == bestGS && (g > bestG || (g == bestG && tm < best))) {
			best, bestGS, bestG = tm, gs, g
		}
	}
	if best == "" {
		for tm, g := range teamG {
			if g > bestG || (g == bestG && (tm < best || best == "")) {
				best, bestG = tm, g
			}
		}
	}
	return best
}
