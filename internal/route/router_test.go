package route

import (
	"context"
	"errors"
	"testing"
)

// countingDecider records how many times the model was asked.
type countingDecider struct {
	calls    int
	answer   Decision
	answers  []Decision
	lastReq  Request
	failures [][]string
}

func (d *countingDecider) Decide(_ context.Context, req Request) (Decision, error) {
	d.calls++
	d.lastReq = req
	d.failures = append(d.failures, append([]string(nil), req.Failures...))
	if len(d.answers) > 0 {
		a := d.answers[0]
		d.answers = d.answers[1:]
		return a, nil
	}
	return d.answer, nil
}

type memCache struct {
	m       map[string]Decision
	forgot  []string
	saves   int
	lookups int
}

func newMemCache() *memCache { return &memCache{m: map[string]Decision{}} }

func (c *memCache) Route(_ context.Context, shape string) (Decision, bool, error) {
	c.lookups++
	d, ok := c.m[shape]
	return d, ok, nil
}

func (c *memCache) SaveRoute(_ context.Context, shape string, d Decision, _ string) error {
	c.saves++
	c.m[shape] = d
	return nil
}

func (c *memCache) ForgetRoute(_ context.Context, shape string) error {
	c.forgot = append(c.forgot, shape)
	delete(c.m, shape)
	return nil
}

func webExtract() Decision {
	return Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "top {n0} stories"},
	}
}

// The claim, for routing. Same as the extraction path makes: after the first
// time, the model is not asked again.
func TestARepeatedInstructionCostsNoModelCall(t *testing.T) {
	d := &countingDecider{answer: webExtract()}
	r := &Router{Decider: d, Cache: newMemCache()}

	const instruction = "the top 10 stories from https://news.ycombinator.com"

	first, err := r.Route(context.Background(), instruction)
	if err != nil {
		t.Fatal(err)
	}
	if first.DecisionCalls != 1 {
		t.Fatalf("cold run made %d calls, want 1", first.DecisionCalls)
	}

	for i := 0; i < 5; i++ {
		got, err := r.Route(context.Background(), instruction)
		if err != nil {
			t.Fatal(err)
		}
		if got.DecisionCalls != 0 {
			t.Fatalf("warm run %d made %d model calls, want 0", i, got.DecisionCalls)
		}
		if !got.Route.FromCache {
			t.Fatalf("warm run %d did not report itself cached", i)
		}
	}

	if d.calls != 1 {
		t.Fatalf("model was asked %d times across six runs, want 1", d.calls)
	}
}

// The point of shaping. Different page, different count, same question — and
// the model is still asked only once.
func TestTheSameQuestionAboutSomethingElseStaysWarm(t *testing.T) {
	d := &countingDecider{answer: webExtract()}
	r := &Router{Decider: d, Cache: newMemCache()}

	for _, instruction := range []string{
		"the top 10 stories from https://news.ycombinator.com",
		"the top 30 stories from https://lobste.rs",
		"The top 5 stories from example.com",
	} {
		res, err := r.Route(context.Background(), instruction)
		if err != nil {
			t.Fatalf("%q: %v", instruction, err)
		}
		if res.Route.Slot("url") == "" {
			t.Fatalf("%q routed with no url", instruction)
		}
	}

	if d.calls != 1 {
		t.Fatalf("model was asked %d times for three instructions of one shape, want 1", d.calls)
	}
}

// The other half of the cache being worth anything: a genuinely different
// request must not reuse the answer.
func TestADifferentQuestionIsDecidedAfresh(t *testing.T) {
	d := &countingDecider{answer: webExtract()}
	r := &Router{Decider: d, Cache: newMemCache()}

	if _, err := r.Route(context.Background(), "the top 10 stories from news.ycombinator.com"); err != nil {
		t.Fatal(err)
	}

	d.answer = Decision{
		Kind:  KindAppAction,
		Slots: map[string]string{"app": "Telegram", "action": "reply to the last message"},
	}
	res, err := r.Route(context.Background(), "reply to the last message in Telegram")
	if err != nil {
		t.Fatal(err)
	}
	if res.Route.Kind != KindAppAction {
		t.Errorf("kind = %q", res.Route.Kind)
	}
	if d.calls != 2 {
		t.Fatalf("model asked %d times for two different shapes, want 2", d.calls)
	}
}

// Declining is a real answer and has to reach the caller as one.
func TestDecliningIsReportedWithItsReason(t *testing.T) {
	d := &countingDecider{answer: Decision{Kind: KindUnknown, Why: "nothing here reads PDFs"}}
	r := &Router{Decider: d, Cache: newMemCache()}

	_, err := r.Route(context.Background(), "summarise the PDF on my desktop")

	var undecided *UndecidedError
	if !errors.As(err, &undecided) {
		t.Fatalf("want UndecidedError, got %v", err)
	}
	if undecided.Why != "nothing here reads PDFs" {
		t.Errorf("reason lost: %q", undecided.Why)
	}
}

// A decision that cannot be filled from the instruction that produced it will
// never be fillable. Retry with the reason, and do not cache the bad one.
func TestAnUnfillableDecisionIsRetriedWithTheReasonAndNotCached(t *testing.T) {
	bad := Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u9}", "intent": "the stories"},
	}
	good := Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "the stories"},
	}

	d := &countingDecider{answers: []Decision{bad, good}}
	c := newMemCache()
	r := &Router{Decider: d, Cache: c}

	res, err := r.Route(context.Background(), "the stories from news.ycombinator.com")
	if err != nil {
		t.Fatal(err)
	}
	if res.DecisionCalls != 2 {
		t.Errorf("calls = %d, want 2", res.DecisionCalls)
	}
	if c.saves != 1 {
		t.Errorf("cache writes = %d, want only the good decision saved", c.saves)
	}

	// The retry has to know why the first was rejected, or it is a blind
	// repeat that will produce the same answer.
	if len(d.failures) < 2 || len(d.failures[1]) == 0 {
		t.Fatalf("second attempt was not told why the first failed: %#v", d.failures)
	}
}

// A decision stored before a kind's requirements changed is as unusable as a
// wrong one. It must be dropped rather than replayed forever.
func TestACachedDecisionThatNoLongerResolvesIsReplaced(t *testing.T) {
	c := newMemCache()
	c.m["the stories from {u0}"] = Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u7}", "intent": "the stories"},
	}

	d := &countingDecider{answer: Decision{
		Kind:  KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "the stories"},
	}}
	r := &Router{Decider: d, Cache: c}

	res, err := r.Route(context.Background(), "the stories from news.ycombinator.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.forgot) != 1 {
		t.Errorf("stale decision was not forgotten: %#v", c.forgot)
	}
	if res.Route.Slot("url") != "news.ycombinator.com" {
		t.Errorf("url = %q", res.Route.Slot("url"))
	}
}

// The model is told which placeholders exist rather than being left to parse
// them out of the shape, so it cannot invent one that was never there.
func TestTheModelIsToldWhichPlaceholdersExist(t *testing.T) {
	d := &countingDecider{answer: webExtract()}
	r := &Router{Decider: d, Cache: newMemCache()}

	if _, err := r.Route(context.Background(), "the top 10 stories from news.ycombinator.com"); err != nil {
		t.Fatal(err)
	}

	got := d.lastReq.Placeholders
	want := map[string]bool{"u0": true, "n0": true}
	if len(got) != len(want) {
		t.Fatalf("placeholders = %v, want u0 and n0", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected placeholder %q", p)
		}
	}
}

// A bare URL with no verb is not an instruction. Saying so beats guessing.
func TestAValueWithNoInstructionIsDeclined(t *testing.T) {
	d := &countingDecider{answer: webExtract()}
	r := &Router{Decider: d, Cache: newMemCache()}

	_, err := r.Route(context.Background(), "https://news.ycombinator.com")

	var undecided *UndecidedError
	if !errors.As(err, &undecided) {
		t.Fatalf("want UndecidedError, got %v", err)
	}
	if d.calls != 0 {
		t.Errorf("model was asked about a bare URL %d times, want 0", d.calls)
	}
}
