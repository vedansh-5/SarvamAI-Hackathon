//go:build integration

package synth

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/extract"
)

// Run with: go test -tags integration ./internal/synth/
//
// Makes a real API call against whichever provider the environment selects.
// Free open-weight default:
//
//	export VANS_LLM_API_KEY=sk-or-v1-...      # openrouter.ai key
//	go test -tags integration ./internal/synth/
//
// Against Claude instead:
//
//	export VANS_LLM_PROVIDER=anthropic ANTHROPIC_API_KEY=sk-ant-...
func fixtureCandidates(t *testing.T) []extract.Candidate {
	t.Helper()

	html, err := os.ReadFile("../extract/testdata/products.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	candidates, err := extract.FindCandidates(string(html), extract.Options{})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("no candidates in the fixture")
	}
	return candidates
}

// The configured endpoint, with the model named separately so a test can point
// the ladder somewhere deliberate.
func liveClient(t *testing.T) (*OpenAICompatible, string) {
	t.Helper()

	key := os.Getenv(EnvAPIKey)
	if key == "" {
		t.Skipf("%s is not set", EnvAPIKey)
	}
	base := os.Getenv(EnvBaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	model := os.Getenv(EnvModel)
	if model == "" {
		model = DefaultOSModel
	}

	return &OpenAICompatible{BaseURL: base, APIKey: key, Model: model}, model
}

// The first name does not exist, so an answer can only have come from the
// second. D-097 was written from an outage that had one model dead and another
// on the same key answering in a second and a half; this is that shape, made
// repeatable without waiting for another outage.
func TestFallbackModelAnswersLive(t *testing.T) {
	c, model := liveClient(t)
	c.Model = "models/vans-no-such-model"
	c.Fallback = model

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	got, err := c.Synthesize(ctx, Request{
		Intent:     "the product name, link and price for each item",
		URL:        "https://shop.example/search",
		Candidates: fixtureCandidates(t),
	})
	if err != nil {
		t.Fatalf("the fallback did not answer: %v", err)
	}
	if got.Root == "" {
		t.Error("empty program from the fallback model")
	}
	t.Logf("fallback %s answered: root=%q", model, got.Root)
}

// A wait nothing can meet, against a real socket rather than a stub: the point
// is that the failure says the model did not answer, and arrives now.
func TestShortWaitIsReportedAsSilenceLive(t *testing.T) {
	c, _ := liveClient(t)
	c.AnswerWait = time.Millisecond

	start := time.Now()
	_, err := c.Synthesize(context.Background(), Request{
		Intent:     "the product name, link and price for each item",
		URL:        "https://shop.example/search",
		Candidates: fixtureCandidates(t),
	})
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, ErrNoAnswer) {
		t.Errorf("not reported as an unanswered request: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Errorf("took %v to give up on a 1ms wait", elapsed)
	}
	t.Logf("gave up in %v: %v", time.Since(start).Round(time.Millisecond), err)
}

func TestSynthesizeLive(t *testing.T) {
	candidates := fixtureCandidates(t)

	s, err := FromEnv()
	if err != nil {
		t.Skipf("no synthesis backend configured: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	got, err := s.Synthesize(ctx, Request{
		Intent:     "the product name, link and price for each item",
		URL:        "https://shop.example/search",
		Candidates: candidates,
	})
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	// Validate already ran inside Synthesize; these check it picked the right
	// region and understood the task, not merely that it returned valid JSON.
	if !strings.Contains(got.Root, "card") {
		t.Errorf("root = %q, expected it to target the product cards", got.Root)
	}
	if len(got.Fields) < 2 {
		t.Errorf("only %d fields: %+v", len(got.Fields), got.Fields)
	}

	var sawHref bool
	for _, f := range got.Fields {
		if f.Attr == "href" {
			sawHref = true
		}
	}
	if !sawHref {
		t.Errorf("no field reads href, though a link was requested: %+v", got.Fields)
	}

	t.Logf("root=%q fields=%v expect=%+v", got.Root, got.FieldNames(), got.Expect)
}
