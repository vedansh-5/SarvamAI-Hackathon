package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const (
	EnvTable     = "VANS_TABLE"
	testersKey   = "tester"
	feedbackKey  = "feedback"
	everyoneKey  = "all"
	dayKeyPrefix = "use#"
)

type DynamoAPI interface {
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, in *dynamodb.UpdateItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Query(ctx context.Context, in *dynamodb.QueryInput, opts ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
}

type Dynamo struct {
	Client DynamoAPI
	Table  string
}

func key(pk, sk string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"pk": &types.AttributeValueMemberS{Value: pk},
		"sk": &types.AttributeValueMemberS{Value: sk},
	}
}

func str(item map[string]types.AttributeValue, name string) string {
	if v, ok := item[name].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func num(item map[string]types.AttributeValue, name string) int {
	if v, ok := item[name].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.Atoi(v.Value)
		return n
	}
	return 0
}

func flag(item map[string]types.AttributeValue, name string) bool {
	v, ok := item[name].(*types.AttributeValueMemberBOOL)
	return ok && v.Value
}

func testerFrom(item map[string]types.AttributeValue) Tester {
	return Tester{Name: str(item, "name"), Code: str(item, "sk"), Daily: num(item, "daily"), Off: flag(item, "off")}
}

func (d *Dynamo) Tester(ctx context.Context, code string) (Tester, bool, error) {
	out, err := d.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &d.Table, Key: key(testersKey, code), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return Tester{}, false, err
	}
	if len(out.Item) == 0 {
		return Tester{}, false, nil
	}
	return testerFrom(out.Item), true, nil
}

func (d *Dynamo) add(ctx context.Context, day, who string) (int, error) {
	out, err := d.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.Table,
		Key:                       key(dayKeyPrefix+day, who),
		UpdateExpression:          aws.String("ADD n :one"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":one": &types.AttributeValueMemberN{Value: "1"}},
		ReturnValues:              types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, err
	}
	return num(out.Attributes, "n"), nil
}

func (d *Dynamo) Spend(ctx context.Context, code, day string) (int, int, error) {
	mine, err := d.add(ctx, day, code)
	if err != nil {
		return 0, 0, err
	}
	all, err := d.add(ctx, day, everyoneKey)
	if err != nil {
		return 0, 0, err
	}
	return mine, all, nil
}

func (d *Dynamo) Keep(ctx context.Context, f Feedback) error {
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":   &types.AttributeValueMemberS{Value: feedbackKey},
			"sk":   &types.AttributeValueMemberS{Value: f.At.UTC().Format(time.RFC3339Nano) + "#" + hex.EncodeToString(suffix)},
			"from": &types.AttributeValueMemberS{Value: f.From},
			"kind": &types.AttributeValueMemberS{Value: f.Kind},
			"text": &types.AttributeValueMemberS{Value: f.Text},
		},
	})
	return err
}

var ErrCodeTaken = errors.New("server: that code is already in use")

func NewCode() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return "vans-" + hex.EncodeToString(b)
}

func (d *Dynamo) AddTester(ctx context.Context, t Tester) error {
	if t.Name == "" || len(t.Code) < 12 || t.Daily < 0 {
		return fmt.Errorf("server: a tester needs a name, a code of 12 or more characters and a daily number")
	}
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":    &types.AttributeValueMemberS{Value: testersKey},
			"sk":    &types.AttributeValueMemberS{Value: t.Code},
			"name":  &types.AttributeValueMemberS{Value: t.Name},
			"daily": &types.AttributeValueMemberN{Value: strconv.Itoa(t.Daily)},
			"off":   &types.AttributeValueMemberBOOL{Value: t.Off},
		},
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	var taken *types.ConditionalCheckFailedException
	if errors.As(err, &taken) {
		return ErrCodeTaken
	}
	return err
}

func (d *Dynamo) SetOff(ctx context.Context, code string, off bool) error {
	_, err := d.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.Table,
		Key:                       key(testersKey, code),
		UpdateExpression:          aws.String("SET #off = :off"),
		ConditionExpression:       aws.String("attribute_exists(pk)"),
		ExpressionAttributeNames:  map[string]string{"#off": "off"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":off": &types.AttributeValueMemberBOOL{Value: off}},
	})
	var missing *types.ConditionalCheckFailedException
	if errors.As(err, &missing) {
		return fmt.Errorf("server: no tester has that code")
	}
	return err
}

func (d *Dynamo) everything(ctx context.Context, pk string) ([]map[string]types.AttributeValue, error) {
	var items []map[string]types.AttributeValue
	var after map[string]types.AttributeValue
	for {
		out, err := d.Client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 &d.Table,
			KeyConditionExpression:    aws.String("pk = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{":pk": &types.AttributeValueMemberS{Value: pk}},
			ExclusiveStartKey:         after,
		})
		if err != nil {
			return nil, err
		}
		items = append(items, out.Items...)
		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		after = out.LastEvaluatedKey
	}
}

func (d *Dynamo) Testers(ctx context.Context) ([]Tester, error) {
	items, err := d.everything(ctx, testersKey)
	if err != nil {
		return nil, err
	}
	out := make([]Tester, 0, len(items))
	for _, item := range items {
		out = append(out, testerFrom(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (d *Dynamo) Used(ctx context.Context, day string) (map[string]int, error) {
	items, err := d.everything(ctx, dayKeyPrefix+day)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, item := range items {
		out[str(item, "sk")] = num(item, "n")
	}
	return out, nil
}

func (d *Dynamo) Feedback(ctx context.Context) ([]Feedback, error) {
	items, err := d.everything(ctx, feedbackKey)
	if err != nil {
		return nil, err
	}
	out := make([]Feedback, 0, len(items))
	for _, item := range items {
		when, _, _ := strings.Cut(str(item, "sk"), "#")
		at, _ := time.Parse(time.RFC3339Nano, when)
		out = append(out, Feedback{From: str(item, "from"), Kind: str(item, "kind"), Text: str(item, "text"), At: at})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}
