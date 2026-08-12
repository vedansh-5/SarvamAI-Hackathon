package store_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/runner"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

// This file is the runtime's central claim, written as a test.
//
// A cold run calls the model once. Every subsequent run on the same page shape
// calls it zero times. Everything else in the design — the page signature, the
// program primitive, the verifier, this store — exists to make that sentence
// true, so it is asserted rather than described.
//
// The pipeline here is the real one minus the browser: signature, cache
// lookup, program run, verification, statistics. Only the model is faked, and
// it is faked precisely so the call count means something.

const pageIntent = "the product name, link and price for each item"

// fixedSynth returns a known-good program for the shared fixture.
type fixedSynth struct{ program extract.Program }

func (f fixedSynth) Synthesize(context.Context, synth.Request) (extract.Program, error) {
	return f.program, nil
}

func productsProgram() extract.Program {
	return extract.Program{
		Root: "div.results > article.card",
		Fields: []extract.Field{
			{Name: "product_name", Selector: "h3 a", Attr: "text"},
			{Name: "product_link", Selector: "h3 a", Attr: "href"},
			{Name: "price", Selector: "span.price", Attr: "text"},
		},
		Expect: extract.Expectation{
			MinRows:  1,
			Required: []string{"product_name", "product_link", "price"},
		},
	}
}

// pipeline is the warm/cold path: look the program up, run it, verify it, and
// only call the model when nothing cached works.
type pipeline struct {
	t     *testing.T
	store *store.Store
	synth *synth.Counter
	base  *url.URL
}

func (p *pipeline) extract(ctx context.Context, pageURL, src string) []runner.Row {
	p.t.Helper()

	sig, err := extract.SignatureOf(src, extract.NewSigOptions())
	if err != nil {
		p.t.Fatalf("SignatureOf: %v", err)
	}
	key, err := store.NewKey(pageURL, sig, pageIntent)
	if err != nil {
		p.t.Fatalf("NewKey: %v", err)
	}

	cached, err := p.store.Lookup(ctx, key)
	if err != nil {
		p.t.Fatalf("Lookup: %v", err)
	}

	var failures []string
	for _, e := range cached {
		res, err := runner.Execute(src, e.Program, runner.Options{Base: p.base})
		if err == nil {
			if err := p.store.RecordRun(ctx, e.ID, true); err != nil {
				p.t.Fatalf("RecordRun: %v", err)
			}
			return res.Rows
		}
		if err := p.store.RecordRun(ctx, e.ID, false); err != nil {
			p.t.Fatalf("RecordRun: %v", err)
		}
		failures = append(failures, runner.Failures(err)...)
	}

	// Nothing cached worked. This is the only path that may call the model.
	candidates, err := extract.FindCandidates(src, extract.Options{})
	if err != nil {
		p.t.Fatalf("FindCandidates: %v", err)
	}

	program, err := p.synth.Synthesize(ctx, synth.Request{
		Intent:     pageIntent,
		URL:        pageURL,
		Candidates: candidates,
		Failures:   failures,
	})
	if err != nil {
		p.t.Fatalf("Synthesize: %v", err)
	}

	id, err := p.store.Put(ctx, key, program, pageIntent, "test")
	if err != nil {
		p.t.Fatalf("Put: %v", err)
	}

	res, err := runner.Execute(src, program, runner.Options{Base: p.base})
	if err != nil {
		p.t.Fatalf("fresh program failed verification: %v", err)
	}
	if err := p.store.RecordRun(ctx, id, true); err != nil {
		p.t.Fatalf("RecordRun: %v", err)
	}
	return res.Rows
}

func newPipeline(t *testing.T, s synth.Synthesizer) *pipeline {
	t.Helper()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	base, err := url.Parse("https://shop.example/search")
	if err != nil {
		t.Fatal(err)
	}
	return &pipeline{t: t, store: st, synth: synth.NewCounter(s), base: base}
}

func fixture(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("../extract/testdata/products.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(src)
}

func TestWarmRunsCallTheModelZeroTimes(t *testing.T) {
	ctx := context.Background()
	src := fixture(t)
	p := newPipeline(t, fixedSynth{productsProgram()})

	const runs = 8
	for i := 0; i < runs; i++ {
		rows := p.extract(ctx, "https://shop.example/search?q=w", src)
		if len(rows) != 4 {
			t.Fatalf("run %d returned %d rows, want 4", i, len(rows))
		}
		if rows[0]["product_link"] != "https://shop.example/p/1" {
			t.Fatalf("run %d: link = %q", i, rows[0]["product_link"])
		}
	}

	if n := p.synth.Calls(); n != 1 {
		t.Errorf("the model was called %d times across %d runs, want exactly 1", n, runs)
	}
}

// Different products, different prices, same page. This is the ordinary case —
// a search run again tomorrow — and it must not cost a synthesis.
func TestNewContentInTheSameShapeStaysWarm(t *testing.T) {
	ctx := context.Background()
	src := fixture(t)
	p := newPipeline(t, fixedSynth{productsProgram()})

	p.extract(ctx, "https://shop.example/search?q=w", src)

	changed := strings.NewReplacer(
		"Widget", "Sprocket",
		"$10", "$99",
		"/p/1", "/p/517",
	).Replace(src)

	rows := p.extract(ctx, "https://shop.example/search?q=sprocket", changed)
	if rows[0]["product_name"] != "Sprocket" {
		t.Fatalf("extracted stale content: %+v", rows[0])
	}
	if n := p.synth.Calls(); n != 1 {
		t.Errorf("the model was called %d times, want 1 — new text is not a new page shape", n)
	}
}

// A different number of results is the same page shape. Without this the cache
// would miss on almost every real visit, since result counts vary constantly.
func TestMoreResultsStayWarm(t *testing.T) {
	ctx := context.Background()
	src := fixture(t)
	p := newPipeline(t, fixedSynth{productsProgram()})

	p.extract(ctx, "https://shop.example/search?q=w", src)

	extra := strings.Repeat(
		`<article class="card"><h3><a href="/p/9">More</a></h3><span class="price">$9</span></article>`, 20)
	bigger := strings.Replace(src, "</div>\n  </div>", extra+"</div>\n  </div>", 1)
	if bigger == src {
		t.Fatal("fixture layout changed; the injection point no longer matches")
	}

	rows := p.extract(ctx, "https://shop.example/search?q=w", bigger)
	if len(rows) != 24 {
		t.Fatalf("got %d rows, want 24", len(rows))
	}
	if n := p.synth.Calls(); n != 1 {
		t.Errorf("the model was called %d times, want 1 — a longer list is the same shape", n)
	}
}

// The other half of the claim: when the page really is different, the runtime
// has to notice. A cache that never invalidates is worse than no cache.
func TestARedesignedPageIsSynthesizedAgain(t *testing.T) {
	ctx := context.Background()
	src := fixture(t)

	redesigned := `<!doctype html>
<html><body><main id="content"><section class="listing">
  <div class="tile"><span class="label">Widget</span><b class="cost">$10</b></div>
  <div class="tile"><span class="label">Gadget</span><b class="cost">$20</b></div>
  <div class="tile"><span class="label">Thing</span><b class="cost">$30</b></div>
</section></main></body></html>`

	// The model answers according to the shape it is shown.
	responder := func(_ context.Context, req synth.Request) (extract.Program, error) {
		if strings.Contains(req.Candidates[0].Skeleton, "tile") {
			return extract.Program{
				Root: "section.listing > div.tile",
				Fields: []extract.Field{
					{Name: "product_name", Selector: "span.label", Attr: "text"},
					{Name: "price", Selector: "b.cost", Attr: "text"},
				},
				Expect: extract.Expectation{MinRows: 1, Required: []string{"product_name", "price"}},
			}, nil
		}
		return productsProgram(), nil
	}

	p := newPipeline(t, synthFunc(responder))

	if rows := p.extract(ctx, "https://shop.example/search", src); len(rows) != 4 {
		t.Fatalf("got %d rows from the original page", len(rows))
	}
	if rows := p.extract(ctx, "https://shop.example/search", redesigned); len(rows) != 3 {
		t.Fatalf("got %d rows from the redesigned page", len(rows))
	}
	if n := p.synth.Calls(); n != 2 {
		t.Errorf("the model was called %d times, want 2 — one per page shape", n)
	}

	// And the redesign is now cached in its own right.
	p.extract(ctx, "https://shop.example/search", redesigned)
	if n := p.synth.Calls(); n != 2 {
		t.Errorf("the model was called %d times, want 2 — the new shape should now be warm", n)
	}
}

// variantPage builds a page whose price class carries a build hash. Two hashes
// produce the same signature — normalizing generated class names is what lets
// a signature survive a deploy (D-006) — but a program that anchored on one
// hash fails against the other.
//
// This is the case that justifies keeping more than one program per key: two
// pages the cache cannot tell apart, needing different programs. A/B tests and
// staged rollouts produce exactly this.
func variantPage(priceClass string) string {
	return `<!doctype html><html><body><div id="main"><div class="results">
	  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="` + priceClass + `">$10</span></article>
	  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="` + priceClass + `">$20</span></article>
	  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="` + priceClass + `">$30</span></article>
	</div></div></body></html>`
}

func variantProgram(priceClass string) extract.Program {
	return extract.Program{
		Root: "div.results > article.card",
		Fields: []extract.Field{
			{Name: "product_name", Selector: "h3 a", Attr: "text"},
			{Name: "price", Selector: "span." + priceClass, Attr: "text"},
		},
		Expect: extract.Expectation{MinRows: 1, Required: []string{"product_name", "price"}},
	}
}

// Keeping the older program is only worth the storage if it is actually tried.
// Two page variants sharing a signature must not call the model on every
// alternating visit.
func TestAlternatingVariantsDoNotThrash(t *testing.T) {
	ctx := context.Background()

	const classA, classB = "price-3f9a2", "price-8b1c4"
	variantA, variantB := variantPage(classA), variantPage(classB)

	// The premise: if these signed differently they would be separate cache
	// keys and this test would prove nothing about keeping several programs.
	sigA, err := extract.SignatureOf(variantA, extract.NewSigOptions())
	if err != nil {
		t.Fatal(err)
	}
	sigB, err := extract.SignatureOf(variantB, extract.NewSigOptions())
	if err != nil {
		t.Fatal(err)
	}
	if sigA != sigB {
		t.Fatalf("premise broken: the variants sign differently (%s vs %s)", sigA, sigB)
	}

	// The model answers according to the markup it is shown, hashed class and
	// all — which is how a program ends up anchored to one deploy.
	responder := func(_ context.Context, req synth.Request) (extract.Program, error) {
		for _, c := range req.Candidates {
			if strings.Contains(c.Skeleton, classB) {
				return variantProgram(classB), nil
			}
		}
		return variantProgram(classA), nil
	}

	p := newPipeline(t, synthFunc(responder))

	// Two cold runs, one per variant. The second is a cache hit that fails
	// verification, so the ladder synthesizes a replacement.
	p.extract(ctx, "https://shop.example/s", variantA)
	p.extract(ctx, "https://shop.example/s", variantB)
	if n := p.synth.Calls(); n != 2 {
		t.Fatalf("setup: model called %d times, want 2", n)
	}

	// Now alternate. Both programs are cached under the same key, so every
	// visit should find a working one without asking the model.
	for i := 0; i < 6; i++ {
		src := variantA
		if i%2 == 1 {
			src = variantB
		}
		rows := p.extract(ctx, "https://shop.example/s", src)
		if len(rows) != 3 {
			t.Fatalf("visit %d returned %d rows", i, len(rows))
		}
		if rows[0]["price"] != "$10" {
			t.Fatalf("visit %d extracted price %q", i, rows[0]["price"])
		}
	}

	if n := p.synth.Calls(); n != 2 {
		t.Errorf("the model was called %d times, want 2 — alternating variants thrash the cache", n)
	}
}

type synthFunc func(context.Context, synth.Request) (extract.Program, error)

func (f synthFunc) Synthesize(ctx context.Context, req synth.Request) (extract.Program, error) {
	return f(ctx, req)
}
