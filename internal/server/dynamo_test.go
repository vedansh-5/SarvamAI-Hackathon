package server

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type table struct {
	mu    sync.Mutex
	items map[[2]string]map[string]types.AttributeValue
}

func newTable() *table { return &table{items: map[[2]string]map[string]types.AttributeValue{}} }

func keyOf(m map[string]types.AttributeValue) [2]string { return [2]string{str(m, "pk"), str(m, "sk")} }

func (t *table) GetItem(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return &dynamodb.GetItemOutput{Item: t.items[keyOf(in.Key)]}, nil
}

func (t *table) PutItem(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := keyOf(in.Item)
	if in.ConditionExpression != nil && *in.ConditionExpression == "attribute_not_exists(pk)" && t.items[k] != nil {
		return nil, &types.ConditionalCheckFailedException{}
	}
	t.items[k] = in.Item
	return &dynamodb.PutItemOutput{}, nil
}

func (t *table) DeleteItem(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.items, keyOf(in.Key))
	return &dynamodb.DeleteItemOutput{}, nil
}

func (t *table) UpdateItem(_ context.Context, in *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	k := keyOf(in.Key)
	item := t.items[k]
	if in.ConditionExpression != nil && *in.ConditionExpression == "attribute_exists(pk)" && item == nil {
		return nil, &types.ConditionalCheckFailedException{}
	}
	if item == nil {
		item = map[string]types.AttributeValue{"pk": in.Key["pk"], "sk": in.Key["sk"]}
	}
	switch *in.UpdateExpression {
	case "ADD n :one":
		item["n"] = &types.AttributeValueMemberN{Value: strconv.Itoa(num(item, "n") + 1)}
	case "SET #f = :v":
		item[in.ExpressionAttributeNames["#f"]] = in.ExpressionAttributeValues[":v"]
	default:
		return nil, errors.New("the stand-in table does not know " + *in.UpdateExpression)
	}
	t.items[k] = item
	return &dynamodb.UpdateItemOutput{Attributes: map[string]types.AttributeValue{"n": item["n"]}}, nil
}

func (t *table) Query(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	pk := str(in.ExpressionAttributeValues, ":pk")
	var out []map[string]types.AttributeValue
	for k, item := range t.items {
		if k[0] == pk {
			out = append(out, item)
		}
	}
	sort.Slice(out, func(i, j int) bool { return str(out[i], "sk") < str(out[j], "sk") })
	return &dynamodb.QueryOutput{Items: out}, nil
}

func TestTheTableKeepsTestersAndTheirCounts(t *testing.T) {
	ctx := context.Background()
	d := &Dynamo{Client: newTable(), Table: "vans"}

	riyaCode := NewCode()
	if !strings.HasPrefix(riyaCode, "vans-") || len(riyaCode) != 25 {
		t.Fatalf("code = %q", riyaCode)
	}
	if err := d.AddTester(ctx, Tester{Name: "Riya", Code: riyaCode, Daily: 2}); err != nil {
		t.Fatal(err)
	}
	if err := d.AddTester(ctx, Tester{Name: "Arjun", Code: riyaCode, Daily: 2}); !errors.Is(err, ErrCodeTaken) {
		t.Errorf("a second tester with the same code: err = %v", err)
	}

	got, found, err := d.Tester(ctx, riyaCode)
	if err != nil || !found || got.Name != "Riya" || got.Daily != 2 || got.Off {
		t.Fatalf("Tester = %+v, %v, %v", got, found, err)
	}
	if _, found, _ := d.Tester(ctx, "vans-nobody-has-this"); found {
		t.Error("an unknown code was found")
	}

	for want := 1; want <= 3; want++ {
		mine, all, err := d.Spend(ctx, riyaCode, "2026-10-14")
		if err != nil || mine != want || all != want {
			t.Fatalf("spend %d: mine %d all %d err %v", want, mine, all, err)
		}
	}
	if mine, _, _ := d.Spend(ctx, riyaCode, "2026-10-15"); mine != 1 {
		t.Errorf("the next day starts at %d, want 1", mine)
	}
	used, err := d.Used(ctx, "2026-10-14")
	if err != nil || used[riyaCode] != 3 || used["all"] != 3 {
		t.Errorf("used = %v, %v", used, err)
	}

	if err := d.SetOff(ctx, riyaCode, true); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := d.Tester(ctx, riyaCode); !got.Off {
		t.Error("the code is still on")
	}
	if err := d.SetOff(ctx, "vans-nobody-has-this", true); err == nil {
		t.Error("switched off a code nobody has")
	}
	if all, _ := d.Testers(ctx); len(all) != 1 || all[0].Name != "Riya" {
		t.Errorf("testers = %+v", all)
	}
}

func TestTheTableKeepsFeedbackNewestFirst(t *testing.T) {
	ctx := context.Background()
	d := &Dynamo{Client: newTable(), Table: "vans"}
	first := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)

	for i, text := range []string{"first", "second"} {
		if err := d.Keep(ctx, Feedback{From: "Riya", Kind: "note", Text: text, At: first.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := d.Feedback(ctx)
	if err != nil || len(all) != 2 || all[0].Text != "second" || !all[1].At.Equal(first) {
		t.Errorf("feedback = %+v, %v", all, err)
	}
}

func TestTheTableGivesBackWhatEachTesterSentAsLearned(t *testing.T) {
	ctx := context.Background()
	d := &Dynamo{Client: newTable(), Table: "vans"}
	at := time.Date(2026, 10, 16, 7, 30, 0, 0, time.UTC)
	if err := d.KeepLearned(ctx, "Riya", at, `[{"what":"Start in Clockify","days":4}]`); err != nil {
		t.Fatal(err)
	}

	got, err := d.Habits(ctx, "Riya")
	if err != nil || !got.At.Equal(at) || len(got.List) != 1 || got.List[0].What != "Start in Clockify" || got.List[0].Days != 4 {
		t.Errorf("Riya's habits = %+v, %v", got, err)
	}
	if none, err := d.Habits(ctx, "Kabir"); err != nil || len(none.List) != 0 || !none.At.IsZero() {
		t.Errorf("a tester who sent nothing = %+v, %v", none, err)
	}
}
