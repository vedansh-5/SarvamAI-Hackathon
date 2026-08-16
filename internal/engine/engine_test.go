package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

const shopPage = `<!doctype html><html><body><div id="main"><div class="results">
  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>
  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price">$20</span></article>
  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="price">$30</span></article>
</div></div></body></html>`

const pageURL = "https://shop.example/search?q=widget"

func goodProgram() extract.Program {
	return extract.Program{
		Root: "div.results > article.card",
		Fields: []extract.Field{
			{Name: "name", Selector: "h3 a", Attr: "text"},
			{Name: "link", Selector: "h3 a", Attr: "href"},
			{Name: "price", Selector: "span.price", Attr: "text"},
		},
		Expect: extract.Expectation{MinRows: 1, Required: []string{"name", "link", "price"}},
	}
}

func brokenProgram() extract.Program {
	p := goodProgram()
	p.Fields[2].Selector = "span.cost" // matches nothing
	return p
}

// --- fakes ---

type fakeBrowser struct {
	html string
	url  string

	// failures are returned in order before any call succeeds.
	failures []error

	navigations int
	captures    int
}

func (f *fakeBrowser) next() error {
	if len(f.failures) == 0 {
		return nil
	}
	err := f.failures[0]
	f.failures = f.failures[1:]
	return err
}

func (f *fakeBrowser) Navigate(_ context.Context, u string) error {
	f.navigations++
	if err := f.next(); err != nil {
		return err
	}
	if f.url == "" {
		f.url = u
	}
	return nil
}

func (f *fakeBrowser) URL(context.Context) (string, error) { return f.url, nil }

func (f *fakeBrowser) OuterHTML(context.Context) (string, error) {
	f.captures++
	return f.html, nil
}

type fakeSink struct {
	writes []Document
	err    error
}

func (s *fakeSink) Write(_ context.Context, doc Document) (Receipt, error) {
	s.writes = append(s.writes, doc)
	if s.err != nil {
		return Receipt{}, s.err
	}
	return Receipt{ID: "note-1", Title: doc.Title, Verified: true}, nil
}

type synthFunc func(context.Context, synth.Request) (extract.Program, error)

func (f synthFunc) Synthesize(ctx context.Context, r synth.Request) (extract.Program, error) {
	return f(ctx, r)
}

type harness struct {
	engine  *Engine
	browser *fakeBrowser
	sink    *fakeSink
	synth   *synth.Counter
	store   *store.Store

	// requests records what the model was asked, so the failure ladder's
	// feedback can be asserted rather than assumed.
	requests []synth.Request
}

func newHarness(t *testing.T, respond func(int) (extract.Program, error)) *harness {
	t.Helper()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	h := &harness{
		browser: &fakeBrowser{html: shopPage, url: pageURL},
		sink:    &fakeSink{},
		store:   st,
	}

	h.synth = synth.NewCounter(synthFunc(func(_ context.Context, r synth.Request) (extract.Program, error) {
		h.requests = append(h.requests, r)
		return respond(len(h.requests) - 1)
	}))

	h.engine = &Engine{
		Browser: h.browser,
		Store:   st,
		Synth:   h.synth,
		Sink:    h.sink,
		Model:   "test",
		Log:     func(string, ...any) {},
	}
	return h
}

func always(p extract.Program) func(int) (extract.Program, error) {
	return func(int) (extract.Program, error) { return p, nil }
}

func task() Task {
	return Task{URL: pageURL, Intent: "product names, links and prices", Title: "Widgets"}
}

// --- the happy path and the claim ---

func TestColdRunThenWarmRuns(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(goodProgram()))

	plan, err := h.engine.Prepare(ctx, task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(plan.Rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(plan.Rows))
	}
	if plan.FromCache {
		t.Error("the first run reported a cache hit")
	}
	if plan.SynthesisCalls != 1 {
		t.Errorf("cold run cost %d model calls, want 1", plan.SynthesisCalls)
	}

	for i := 0; i < 5; i++ {
		warm, err := h.engine.Prepare(ctx, task())
		if err != nil {
			t.Fatalf("warm Prepare %d: %v", i, err)
		}
		if !warm.FromCache {
			t.Errorf("warm run %d missed the cache", i)
		}
		if warm.SynthesisCalls != 0 {
			t.Errorf("warm run %d cost %d model calls, want 0", i, warm.SynthesisCalls)
		}
	}

	if n := h.synth.Calls(); n != 1 {
		t.Errorf("the model was called %d times across 6 runs, want 1", n)
	}
}

func TestRelativeLinksAreResolvedAgainstThePage(t *testing.T) {
	h := newHarness(t, always(goodProgram()))

	plan, err := h.engine.Prepare(context.Background(), task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got := plan.Rows[0]["link"]; got != "https://shop.example/p/1" {
		t.Errorf("link = %q, want it resolved against the page URL", got)
	}
}

// The URL is read back rather than assumed. A redirect to a login wall is the
// case that matters: keying the cache on the requested URL would file the
// login page's program under the shop's origin.
func TestTheCacheKeyUsesWhereTheBrowserActuallyLanded(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	h.browser.url = "https://login.example/sso?next=/search"

	plan, err := h.engine.Prepare(context.Background(), task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.PageURL != "https://login.example/sso?next=/search" {
		t.Errorf("PageURL = %q, want the redirected location", plan.PageURL)
	}
	if got := plan.Rows[0]["link"]; !strings.HasPrefix(got, "https://login.example/") {
		t.Errorf("link = %q, want it resolved against the real location", got)
	}
}

// --- rung 0: transient ---

type flaky struct{ msg string }

func (f flaky) Error() string   { return f.msg }
func (f flaky) Transient() bool { return true }

func TestTransientFailuresAreRetriedWithoutTheModel(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	h.browser.failures = []error{flaky{"socket closed"}, flaky{"socket closed"}}

	plan, err := h.engine.Prepare(context.Background(), task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if h.browser.navigations != 3 {
		t.Errorf("navigated %d times, want 3 (1 + 2 retries)", h.browser.navigations)
	}
	if plan.SynthesisCalls != 1 {
		t.Errorf("a reconnect cost %d model calls beyond the cold one", plan.SynthesisCalls-1)
	}
}

func TestTransientRetriesAreBounded(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	h.browser.failures = []error{
		flaky{"nope"}, flaky{"nope"}, flaky{"nope"}, flaky{"nope"},
	}

	_, err := h.engine.Prepare(context.Background(), task())
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "did not respond after 3 attempts") {
		t.Errorf("error did not report the budget: %v", err)
	}
	if n := h.synth.Calls(); n != 0 {
		t.Errorf("a browser that never answered still cost %d model calls", n)
	}
}

// A permanent error must not consume the transient budget — retrying a bad URL
// three times only delays telling the user.
func TestPermanentFailuresAreNotRetried(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	h.browser.failures = []error{errors.New("net::ERR_NAME_NOT_RESOLVED")}

	if _, err := h.engine.Prepare(context.Background(), task()); err == nil {
		t.Fatal("want an error")
	}
	if h.browser.navigations != 1 {
		t.Errorf("navigated %d times, want 1", h.browser.navigations)
	}
}

func TestCancellationIsNotTransient(t *testing.T) {
	if IsTransient(context.Canceled) {
		t.Error("a cancelled context was treated as retryable")
	}
	if IsTransient(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)) {
		t.Error("a deadline was treated as retryable")
	}
	if !IsTransient(&TransientError{Err: errors.New("x")}) {
		t.Error("TransientError was not recognized")
	}
}

// --- rungs 1 and 2: verification failures ---

// A cached program that stops working must be replaced, and that is the one
// case where a warm run is allowed to cost a model call.
func TestStaleCachedProgramIsReplaced(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(goodProgram()))

	// Seed the cache with a program that no longer matches.
	sig, err := extract.SignatureOf(shopPage, extract.NewSigOptions())
	if err != nil {
		t.Fatal(err)
	}
	key, err := store.NewKey(pageURL, sig, task().Intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Put(ctx, key, brokenProgram(), task().Intent, "test"); err != nil {
		t.Fatal(err)
	}

	plan, err := h.engine.Prepare(ctx, task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.SynthesisCalls != 1 {
		t.Errorf("SynthesisCalls = %d, want 1", plan.SynthesisCalls)
	}
	if len(plan.Rows) != 3 {
		t.Errorf("got %d rows", len(plan.Rows))
	}

	// The model must have been told why the cached program was rejected,
	// otherwise it is guessing blind.
	if len(h.requests) != 1 {
		t.Fatalf("got %d requests", len(h.requests))
	}
	if len(h.requests[0].Failures) == 0 {
		t.Fatal("the cached program's rejection was not passed to the model")
	}
	joined := strings.Join(h.requests[0].Failures, " ")
	if !strings.Contains(joined, "cache") || !strings.Contains(joined, `"price"`) {
		t.Errorf("failure reasons were not specific: %v", h.requests[0].Failures)
	}

	// And the next run is warm again.
	warm, err := h.engine.Prepare(ctx, task())
	if err != nil {
		t.Fatalf("second Prepare: %v", err)
	}
	if warm.SynthesisCalls != 0 || !warm.FromCache {
		t.Errorf("the replacement was not cached: calls=%d fromCache=%v", warm.SynthesisCalls, warm.FromCache)
	}
}

func TestRejectedSynthesisIsRetriedWithReasons(t *testing.T) {
	h := newHarness(t, func(n int) (extract.Program, error) {
		if n == 0 {
			return brokenProgram(), nil
		}
		return goodProgram(), nil
	})

	plan, err := h.engine.Prepare(context.Background(), task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if plan.SynthesisCalls != 2 {
		t.Errorf("SynthesisCalls = %d, want 2", plan.SynthesisCalls)
	}
	if len(h.requests) != 2 {
		t.Fatalf("got %d requests", len(h.requests))
	}
	if len(h.requests[0].Failures) != 0 {
		t.Error("the first attempt was given failure reasons it could not have")
	}
	if len(h.requests[1].Failures) == 0 {
		t.Fatal("the retry was not told why the first attempt failed")
	}
	if !strings.Contains(strings.Join(h.requests[1].Failures, " "), "synthesis") {
		t.Errorf("the retry's reasons did not say where the program came from: %v", h.requests[1].Failures)
	}
}

// A program that never verified must not be cached — it would sit at the front
// of the next lookup and fail there too.
func TestOnlyVerifiedProgramsAreCached(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(brokenProgram()))

	if _, err := h.engine.Prepare(ctx, task()); err == nil {
		t.Fatal("want an error")
	}

	st, err := h.store.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Programs != 0 {
		t.Errorf("%d programs were cached despite never verifying", st.Programs)
	}
}

// --- rung 3 ---

func TestUnresolvedCarriesWhatWasTried(t *testing.T) {
	h := newHarness(t, always(brokenProgram()))

	_, err := h.engine.Prepare(context.Background(), task())
	if err == nil {
		t.Fatal("want an error")
	}

	var unresolved *UnresolvedError
	if !errors.As(err, &unresolved) {
		t.Fatalf("got %T, want *UnresolvedError: %v", err, err)
	}
	if len(unresolved.Attempts) != 2 {
		t.Errorf("recorded %d attempts, want 2", len(unresolved.Attempts))
	}
	if len(unresolved.Candidates) == 0 {
		t.Error("the candidates found were not carried")
	}
	if unresolved.Signature == "" || unresolved.PageURL == "" {
		t.Error("the page was not identified")
	}
	if !strings.Contains(unresolved.Error(), "attempt 1") {
		t.Errorf("the message does not show what was tried: %v", unresolved)
	}
}

func TestSynthesisBudgetIsRespected(t *testing.T) {
	h := newHarness(t, always(brokenProgram()))
	h.engine.MaxSynthesis = 1

	if _, err := h.engine.Prepare(context.Background(), task()); err == nil {
		t.Fatal("want an error")
	}
	if n := h.synth.Calls(); n != 1 {
		t.Errorf("the model was called %d times, want 1", n)
	}
}

// --- the confirmation gate ---

func TestPrepareNeverWrites(t *testing.T) {
	h := newHarness(t, always(goodProgram()))

	if _, err := h.engine.Prepare(context.Background(), task()); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(h.sink.writes) != 0 {
		t.Errorf("Prepare performed %d writes; the gate is the API shape", len(h.sink.writes))
	}
}

func TestCommitWrites(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(goodProgram()))

	plan, err := h.engine.Prepare(ctx, task())
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	receipt, err := h.engine.Commit(ctx, plan)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !receipt.Verified {
		t.Error("the write was not read back")
	}
	if len(h.sink.writes) != 1 {
		t.Fatalf("wrote %d documents", len(h.sink.writes))
	}
	doc := h.sink.writes[0]
	if doc.Title != "Widgets" || len(doc.Rows) != 3 {
		t.Errorf("document = %+v", doc)
	}
}

// D-020's hard rule: an executed write is never retried.
func TestCommitIsOneShot(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(goodProgram()))

	plan, _ := h.engine.Prepare(ctx, task())
	if _, err := h.engine.Commit(ctx, plan); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := h.engine.Commit(ctx, plan); err == nil {
		t.Fatal("a second Commit succeeded and would have duplicated the note")
	}
	if len(h.sink.writes) != 1 {
		t.Errorf("the sink saw %d writes", len(h.sink.writes))
	}
}

// A write that failed may still have happened. Verification ambiguity is not
// grounds for writing twice.
func TestAFailedCommitIsStillSpent(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(goodProgram()))
	h.sink.err = errors.New("osascript timed out")

	plan, _ := h.engine.Prepare(ctx, task())
	if _, err := h.engine.Commit(ctx, plan); err == nil {
		t.Fatal("want an error")
	}
	if !plan.Committed() {
		t.Error("the plan was not marked spent after a failed write")
	}
	if _, err := h.engine.Commit(ctx, plan); err == nil {
		t.Fatal("a failed write was retried")
	}
	if len(h.sink.writes) != 1 {
		t.Errorf("the sink saw %d writes", len(h.sink.writes))
	}
}

func TestCommitRefusesAnEmptyDocument(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	if _, err := h.engine.Commit(context.Background(), &Plan{Task: task()}); err == nil {
		t.Fatal("an empty document was written")
	}
}

func TestTaskValidation(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	ctx := context.Background()

	if _, err := h.engine.Prepare(ctx, Task{Intent: "things"}); err == nil {
		t.Error("a task with no URL was accepted")
	}
	if _, err := h.engine.Prepare(ctx, Task{URL: pageURL}); err == nil {
		t.Error("a task with no intent was accepted")
	}
}

func TestDefaultTitleNamesTheSite(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, always(goodProgram()))

	untitled := task()
	untitled.Title = ""

	plan, err := h.engine.Prepare(ctx, untitled)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := h.engine.Commit(ctx, plan); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got := h.sink.writes[0].Title
	if !strings.Contains(got, "shop.example") || !strings.Contains(got, "product names") {
		t.Errorf("title = %q", got)
	}
}
