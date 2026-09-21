package synth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/extract"
)

type fakeSynth struct {
	program extract.Program
	err     error
	lastReq Request
}

func (f *fakeSynth) Synthesize(_ context.Context, req Request) (extract.Program, error) {
	f.lastReq = req
	return f.program, f.err
}

func sampleRequest() Request {
	return Request{
		Intent: "the product names and prices",
		URL:    "https://shop.example/search?q=widget",
		Candidates: []extract.Candidate{
			{
				Selector:    "div#main > div.results > article.card",
				RecordCount: 4,
				Skeleton:    `<article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>`,
				Samples:     []string{"Widget $10", "Gadget $20"},
			},
			{
				Selector:    "nav.topbar > a.link",
				RecordCount: 6,
				Skeleton:    `<a class="link" href="/">Home</a>`,
				Samples:     []string{"Home", "About"},
			},
		},
	}
}

func TestBuildPromptCarriesIntentAndCandidates(t *testing.T) {
	got := BuildPrompt(sampleRequest())

	for _, want := range []string{
		"the product names and prices",
		"https://shop.example/search?q=widget",
		"div#main > div.results > article.card",
		"nav.topbar > a.link",
		`<span class="price">$10</span>`,
		"Widget $10",
		"4 records",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q:\n%s", want, got)
		}
	}
}

// Rung 2 of the failure ladder re-synthesizes with the failure reasons in
// hand. If they don't reach the prompt, the retry is just a coin flip.
func TestBuildPromptIncludesFailures(t *testing.T) {
	req := sampleRequest()
	req.Failures = []string{"row 3 had an empty title", "only 1 row matched, expected at least 4"}

	got := BuildPrompt(req)
	for _, want := range req.Failures {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing failure %q:\n%s", want, got)
		}
	}
}

func TestBuildPromptOmitsFailureSectionWhenClean(t *testing.T) {
	if got := BuildPrompt(sampleRequest()); strings.Contains(got, "rejected") {
		t.Errorf("clean request produced a failure section:\n%s", got)
	}
}

func TestBuildPromptTolerateseMissingURL(t *testing.T) {
	req := sampleRequest()
	req.URL = ""
	if got := BuildPrompt(req); strings.Contains(got, "Page:") {
		t.Errorf("empty URL still rendered a Page line:\n%s", got)
	}
}

// The whole thesis reduces to this number being zero on a warm run.
func TestCounterCountsCalls(t *testing.T) {
	fake := &fakeSynth{program: extract.Program{Root: "x"}}
	c := NewCounter(fake)

	if c.Calls() != 0 {
		t.Fatalf("fresh counter reports %d calls", c.Calls())
	}

	for i := 0; i < 3; i++ {
		if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
			t.Fatalf("Synthesize: %v", err)
		}
	}
	if c.Calls() != 3 {
		t.Errorf("Calls() = %d, want 3", c.Calls())
	}

	c.Reset()
	if c.Calls() != 0 {
		t.Errorf("after Reset, Calls() = %d", c.Calls())
	}
}

// A failed call still reached the model and still cost money, so it counts.
func TestCounterCountsFailures(t *testing.T) {
	c := NewCounter(&fakeSynth{err: errors.New("boom")})

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want error")
	}
	if c.Calls() != 1 {
		t.Errorf("Calls() = %d, want 1", c.Calls())
	}
}

func TestCounterPassesRequestThrough(t *testing.T) {
	fake := &fakeSynth{}
	c := NewCounter(fake)

	req := sampleRequest()
	if _, err := c.Synthesize(context.Background(), req); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if fake.lastReq.Intent != req.Intent {
		t.Errorf("intent not passed through: %q", fake.lastReq.Intent)
	}
}

func TestAnthropicRejectsEmptyCandidates(t *testing.T) {
	req := sampleRequest()
	req.Candidates = nil

	if _, err := (&Anthropic{}).Synthesize(context.Background(), req); err == nil {
		t.Error("want an error before any API call when there are no candidates")
	}
}

// Coaching through System Settings asked the person to press Enter, they pressed
// it, and the next step was to press Enter. The list of what has been done is the
// record of what happened; the screen afterwards is not an invitation to do it
// again.
func TestTheStepWriterIsToldNotToRepeatItself(t *testing.T) {
	if !strings.Contains(driveSystemPrompt, "already listed under what has been done") {
		t.Error("nothing tells the step writer that a done step is done")
	}
}
