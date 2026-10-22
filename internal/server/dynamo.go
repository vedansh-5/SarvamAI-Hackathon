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

	"github.com/vedansh/vans/internal/synth"
)

const (
	EnvTable     = "VANS_TABLE"
	testersKey   = "tester"
	feedbackKey  = "feedback"
	learnedKey   = "learned"
	contentKey   = "content"
	emailKey     = "email"
	joinKey      = "join"
	joinsPrefix  = "joins#"
	everyoneKey  = "all"
	dayKeyPrefix = "use#"
	actKeyPrefix = "act#"
	KeepActivity = 30 * 24 * time.Hour
)

type DynamoAPI interface {
	GetItem(ctx context.Context, in *dynamodb.GetItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	PutItem(ctx context.Context, in *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	UpdateItem(ctx context.Context, in *dynamodb.UpdateItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	Query(ctx context.Context, in *dynamodb.QueryInput, opts ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	DeleteItem(ctx context.Context, in *dynamodb.DeleteItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
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
	t := Tester{Name: str(item, "name"), Code: str(item, "sk"), Daily: num(item, "daily"), Off: flag(item, "off"),
		Email: str(item, "email"), Waiting: flag(item, "waiting")}
	if spelled := str(item, "trials"); spelled != "" {
		t.Trials = synth.TrialsSpelled(spelled)
	}
	return t
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

func stamp(at time.Time) string {
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	return at.UTC().Format(time.RFC3339Nano) + "#" + hex.EncodeToString(suffix)
}

func timeOf(sk string) time.Time {
	when, _, _ := strings.Cut(sk, "#")
	at, _ := time.Parse(time.RFC3339Nano, when)
	return at
}

func (d *Dynamo) Record(ctx context.Context, a Activity) error {
	item := map[string]types.AttributeValue{
		"pk":   &types.AttributeValueMemberS{Value: actKeyPrefix + Day(a.At)},
		"sk":   &types.AttributeValueMemberS{Value: stamp(a.At)},
		"from": &types.AttributeValueMemberS{Value: a.From},
		"kind": &types.AttributeValueMemberS{Value: a.Kind},
		"ms":   &types.AttributeValueMemberN{Value: strconv.FormatInt(a.Took.Milliseconds(), 10)},
		"gone": &types.AttributeValueMemberN{Value: strconv.FormatInt(a.At.Add(KeepActivity).Unix(), 10)},
	}
	for name, value := range map[string]string{"words": a.Words, "app": a.App, "fault": a.Fault, "by": a.By, "trials": a.Trials, "why": a.Why} {
		if value != "" {
			item[name] = &types.AttributeValueMemberS{Value: value}
		}
	}
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{TableName: &d.Table, Item: item})
	return err
}

func (d *Dynamo) Activity(ctx context.Context, day string) ([]Activity, error) {
	items, err := d.everything(ctx, actKeyPrefix+day)
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(items))
	for _, item := range items {
		out = append(out, Activity{
			At: timeOf(str(item, "sk")), From: str(item, "from"), Kind: str(item, "kind"),
			Words: str(item, "words"), App: str(item, "app"), Fault: str(item, "fault"), Why: str(item, "why"),
			Took: time.Duration(num(item, "ms")) * time.Millisecond, By: str(item, "by"), Trials: str(item, "trials"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}

func (d *Dynamo) Keep(ctx context.Context, f Feedback) error {
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: feedbackKey},
			"sk":     &types.AttributeValueMemberS{Value: stamp(f.At)},
			"from":   &types.AttributeValueMemberS{Value: f.From},
			"kind":   &types.AttributeValueMemberS{Value: f.Kind},
			"text":   &types.AttributeValueMemberS{Value: f.Text},
			"trials": &types.AttributeValueMemberS{Value: f.Trials},
		},
	})
	return err
}

func (d *Dynamo) KeepLearned(ctx context.Context, from string, at time.Time, habits string) error {
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":     &types.AttributeValueMemberS{Value: learnedKey},
			"sk":     &types.AttributeValueMemberS{Value: from},
			"at":     &types.AttributeValueMemberS{Value: at.UTC().Format(time.RFC3339)},
			"habits": &types.AttributeValueMemberS{Value: habits},
		},
	})
	return err
}

func (d *Dynamo) Habits(ctx context.Context, from string) (Habits, error) {
	out, err := d.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &d.Table, Key: key(learnedKey, from)})
	if err != nil || len(out.Item) == 0 {
		return Habits{}, err
	}
	at, _ := time.Parse(time.RFC3339, str(out.Item, "at"))
	return habitsFrom(at, str(out.Item, "habits"))
}

func (d *Dynamo) Content(ctx context.Context) (string, error) {
	out, err := d.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &d.Table, Key: key(contentKey, "current")})
	if err != nil || len(out.Item) == 0 {
		return "", err
	}
	return str(out.Item, "doc"), nil
}

func (d *Dynamo) SetContent(ctx context.Context, raw string) error {
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":  &types.AttributeValueMemberS{Value: contentKey},
			"sk":  &types.AttributeValueMemberS{Value: "current"},
			"doc": &types.AttributeValueMemberS{Value: raw},
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
	item := map[string]types.AttributeValue{
		"pk":    &types.AttributeValueMemberS{Value: testersKey},
		"sk":    &types.AttributeValueMemberS{Value: t.Code},
		"name":  &types.AttributeValueMemberS{Value: t.Name},
		"daily": &types.AttributeValueMemberN{Value: strconv.Itoa(t.Daily)},
		"off":   &types.AttributeValueMemberBOOL{Value: t.Off},
	}
	if t.Email != "" {
		item["email"] = &types.AttributeValueMemberS{Value: t.Email}
	}
	if t.Waiting {
		item["waiting"] = &types.AttributeValueMemberBOOL{Value: true}
	}
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &d.Table,
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	var taken *types.ConditionalCheckFailedException
	if errors.As(err, &taken) {
		return ErrCodeTaken
	}
	return err
}

func (d *Dynamo) set(ctx context.Context, pk, sk, field string, value types.AttributeValue, missing error) error {
	_, err := d.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.Table,
		Key:                       key(pk, sk),
		UpdateExpression:          aws.String("SET #f = :v"),
		ConditionExpression:       aws.String("attribute_exists(pk)"),
		ExpressionAttributeNames:  map[string]string{"#f": field},
		ExpressionAttributeValues: map[string]types.AttributeValue{":v": value},
	})
	var gone *types.ConditionalCheckFailedException
	if errors.As(err, &gone) {
		return missing
	}
	return err
}

func (d *Dynamo) DeleteTester(ctx context.Context, code string) error {
	t, ok, err := d.Tester(ctx, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoTester
	}
	if _, err := d.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: &d.Table, Key: key(testersKey, code)}); err != nil {
		return err
	}
	if t.Email != "" {
		if _, err := d.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: &d.Table, Key: key(emailKey, t.Email)}); err != nil {
			return err
		}
	}
	_, err = d.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: &d.Table, Key: key(learnedKey, t.Name)})
	return err
}

func (d *Dynamo) SetTrial(ctx context.Context, code, name, variant string) error {
	if err := synth.CheckTrial(name, variant); err != nil {
		return err
	}
	t, ok, err := d.Tester(ctx, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoTester
	}
	chosen := synth.TrialsFor(t.Trials)
	chosen[name] = variant
	return d.set(ctx, testersKey, code, "trials", &types.AttributeValueMemberS{Value: chosen.Spelled()}, ErrNoTester)
}

func (d *Dynamo) SetOff(ctx context.Context, code string, off bool) error {
	if off {
		return d.set(ctx, testersKey, code, "off", &types.AttributeValueMemberBOOL{Value: true}, ErrNoTester)
	}
	_, err := d.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 &d.Table,
		Key:                       key(testersKey, code),
		UpdateExpression:          aws.String("SET #f = :v REMOVE waiting"),
		ConditionExpression:       aws.String("attribute_exists(pk)"),
		ExpressionAttributeNames:  map[string]string{"#f": "off"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":v": &types.AttributeValueMemberBOOL{Value: false}},
	})
	var gone *types.ConditionalCheckFailedException
	if errors.As(err, &gone) {
		return ErrNoTester
	}
	return err
}

func (d *Dynamo) Joined(ctx context.Context, email string) (Tester, bool, error) {
	out, err := d.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &d.Table, Key: key(emailKey, email), ConsistentRead: aws.Bool(true)})
	if err != nil || len(out.Item) == 0 {
		return Tester{}, false, err
	}
	return d.Tester(ctx, str(out.Item, "code"))
}

func (d *Dynamo) Join(ctx context.Context, t Tester) error {
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":   &types.AttributeValueMemberS{Value: emailKey},
			"sk":   &types.AttributeValueMemberS{Value: t.Email},
			"code": &types.AttributeValueMemberS{Value: t.Code},
		},
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	var taken *types.ConditionalCheckFailedException
	if errors.As(err, &taken) {
		return ErrEmailTaken
	}
	if err != nil {
		return err
	}
	if err := d.AddTester(ctx, t); err != nil {
		_, _ = d.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: &d.Table, Key: key(emailKey, t.Email)})
		return err
	}
	return nil
}

func (d *Dynamo) Pending(ctx context.Context, email string) (Pending, bool, error) {
	out, err := d.Client.GetItem(ctx, &dynamodb.GetItemInput{TableName: &d.Table, Key: key(joinKey, email), ConsistentRead: aws.Bool(true)})
	if err != nil || len(out.Item) == 0 {
		return Pending{}, false, err
	}
	return Pending{
		Email: email, Hash: str(out.Item, "hash"), Day: str(out.Item, "day"),
		Expires: time.Unix(int64(num(out.Item, "expires")), 0), SentAt: time.Unix(int64(num(out.Item, "sent")), 0),
		Tries: num(out.Item, "tries"), Sends: num(out.Item, "sends"),
	}, true, nil
}

func (d *Dynamo) SetPending(ctx context.Context, p Pending) error {
	n := func(v int64) types.AttributeValue { return &types.AttributeValueMemberN{Value: strconv.FormatInt(v, 10)} }
	_, err := d.Client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &d.Table,
		Item: map[string]types.AttributeValue{
			"pk":      &types.AttributeValueMemberS{Value: joinKey},
			"sk":      &types.AttributeValueMemberS{Value: p.Email},
			"hash":    &types.AttributeValueMemberS{Value: p.Hash},
			"day":     &types.AttributeValueMemberS{Value: p.Day},
			"expires": n(p.Expires.Unix()),
			"sent":    n(p.SentAt.Unix()),
			"tries":   n(int64(p.Tries)),
			"sends":   n(int64(p.Sends)),
			"gone":    n(p.SentAt.Add(48 * time.Hour).Unix()),
		},
	})
	return err
}

func (d *Dynamo) Count(ctx context.Context, day, what string) (int, error) {
	at, _ := time.Parse("2006-01-02", day)
	out, err := d.Client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:        &d.Table,
		Key:              key(joinsPrefix+day, what),
		UpdateExpression: aws.String("ADD n :one SET gone = :gone"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one":  &types.AttributeValueMemberN{Value: "1"},
			":gone": &types.AttributeValueMemberN{Value: strconv.FormatInt(at.Add(72*time.Hour).Unix(), 10)},
		},
		ReturnValues: types.ReturnValueUpdatedNew,
	})
	if err != nil {
		return 0, err
	}
	return num(out.Attributes, "n"), nil
}

func (d *Dynamo) SetDaily(ctx context.Context, code string, daily int) error {
	if daily < 0 {
		return fmt.Errorf("server: a daily allowance cannot be below zero")
	}
	return d.set(ctx, testersKey, code, "daily", &types.AttributeValueMemberN{Value: strconv.Itoa(daily)}, ErrNoTester)
}

func (d *Dynamo) Handled(ctx context.Context, id string) error {
	return d.set(ctx, feedbackKey, id, "handled", &types.AttributeValueMemberBOOL{Value: true}, ErrNoFeedback)
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
		out = append(out, Feedback{
			ID: str(item, "sk"), From: str(item, "from"), Kind: str(item, "kind"), Text: str(item, "text"),
			At: timeOf(str(item, "sk")), Handled: flag(item, "handled"), Trials: str(item, "trials"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, nil
}
