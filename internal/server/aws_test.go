package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"

	"github.com/vedansh/vans/internal/synth"
)

func TestAFunctionURLRequestReachesTheServer(t *testing.T) {
	var seen *http.Request
	var body string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, `{"ok":true}`)
	})

	in := events.LambdaFunctionURLRequest{
		RawPath:         synth.AskPath,
		Headers:         map[string]string{"authorization": "Bearer " + riya},
		Body:            base64.StdEncoding.EncodeToString([]byte(`{"kind":"ping"}`)),
		IsBase64Encoded: true,
	}
	in.RequestContext.HTTP.Method = http.MethodPost
	out, err := FunctionURL(handler)(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if seen.Method != http.MethodPost || seen.URL.Path != synth.AskPath || codeOf(seen) != riya || body != `{"kind":"ping"}` {
		t.Errorf("server saw %s %s, code %q, body %q", seen.Method, seen.URL.Path, codeOf(seen), body)
	}
	if out.StatusCode != http.StatusTeapot || out.Headers["Content-Type"] != "application/json" || out.Body != `{"ok":true}` {
		t.Errorf("answered %+v", out)
	}
}

func TestTheWholeServerAnswersThroughAFunctionURL(t *testing.T) {
	srv := &Server{Model: func(*synth.Usage) synth.Model { return &standIn{} }, Ledger: NewMemory([]Tester{{Name: "Riya", Code: riya, Daily: 5}})}
	request, _ := json.Marshal(synth.Asking{Kind: synth.KindPlan, Request: json.RawMessage(`{"task":"Settings"}`)})
	in := events.LambdaFunctionURLRequest{RawPath: synth.AskPath, Headers: map[string]string{"authorization": "Bearer " + riya}, Body: string(request)}
	in.RequestContext.HTTP.Method = http.MethodPost

	out, err := FunctionURL(srv)(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	var answered synth.Answered
	if err := json.Unmarshal([]byte(out.Body), &answered); err != nil || answered.Fault != nil || out.StatusCode != http.StatusOK {
		t.Fatalf("status %d, body %s", out.StatusCode, out.Body)
	}
	var plan synth.PlanAnswer
	if err := json.Unmarshal(answered.Answer, &plan); err != nil || len(plan.Steps) != 1 {
		t.Errorf("plan = %+v, %v", plan, err)
	}
}

type settingsStore struct{ pages [][]ssmtypes.Parameter }

func (s *settingsStore) GetParametersByPath(_ context.Context, in *ssm.GetParametersByPathInput, _ ...func(*ssm.Options)) (*ssm.GetParametersByPathOutput, error) {
	page := 0
	if in.NextToken != nil {
		page = 1
	}
	out := &ssm.GetParametersByPathOutput{Parameters: s.pages[page]}
	if page+1 < len(s.pages) {
		out.NextToken = aws.String("more")
	}
	return out, nil
}

func TestSettingsAreReadIntoTheEnvironment(t *testing.T) {
	t.Setenv("VANS_TEST_ONE", "")
	t.Setenv("VANS_TEST_TWO", "")
	store := &settingsStore{pages: [][]ssmtypes.Parameter{
		{{Name: aws.String(SettingsPath + "VANS_TEST_ONE"), Value: aws.String("first")}},
		{{Name: aws.String(SettingsPath + "VANS_TEST_TWO"), Value: aws.String("second")}},
	}}
	n, err := LoadSettings(context.Background(), store)
	if err != nil || n != 2 {
		t.Fatalf("loaded %d, %v", n, err)
	}
	if os.Getenv("VANS_TEST_ONE") != "first" || os.Getenv("VANS_TEST_TWO") != "second" {
		t.Errorf("env = %q, %q", os.Getenv("VANS_TEST_ONE"), os.Getenv("VANS_TEST_TWO"))
	}
}
