package runner

import (
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/extract"
)

func reasonsFor(t *testing.T, src string, p extract.Program) []string {
	t.Helper()
	_, err := Execute(src, p, Options{})
	if err == nil {
		return nil
	}
	fs := Failures(err)
	if fs == nil {
		t.Fatalf("error carried no failure reasons: %v", err)
	}
	return fs
}

func containsAny(reasons []string, substr string) bool {
	for _, r := range reasons {
		if strings.Contains(r, substr) {
			return true
		}
	}
	return false
}

func TestVerifyPassesAGoodRun(t *testing.T) {
	if rs := reasonsFor(t, shopHTML, shopProgram()); rs != nil {
		t.Errorf("a correct program was rejected: %v", rs)
	}
}

func TestRootMatchingNothingIsExplained(t *testing.T) {
	p := shopProgram()
	p.Root = "div.results > article.product"

	rs := reasonsFor(t, shopHTML, p)
	if !containsAny(rs, "matched no elements") {
		t.Errorf("reasons = %v", rs)
	}
	// One clear reason beats three restatements of the same problem in
	// field-shaped language.
	if len(rs) != 1 {
		t.Errorf("got %d reasons for one root problem: %v", len(rs), rs)
	}
}

func TestTooFewRows(t *testing.T) {
	p := shopProgram()
	p.Expect.MinRows = 10

	rs := reasonsFor(t, shopHTML, p)
	if !containsAny(rs, "got 3 rows, expected at least 10") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestRequiredFieldMissingInSomeRows(t *testing.T) {
	const src = `<div class="results">
	  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>
	  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price"></span></article>
	  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="price"></span></article>
	</div>`

	rs := reasonsFor(t, src, shopProgram())
	if !containsAny(rs, `field "price" is marked required but is empty in 2 of 3 rows`) {
		t.Errorf("reasons = %v", rs)
	}
}

// The floor the model cannot lower: a field it did not mark required, empty
// everywhere, is still a broken selector and still a wrong answer.
func TestFieldEmptyEverywhereFailsEvenWhenNotRequired(t *testing.T) {
	p := shopProgram()
	p.Fields = append(p.Fields, extract.Field{Name: "stock", Selector: "span.stock", Attr: "text"})

	rs := reasonsFor(t, shopHTML, p)
	if !containsAny(rs, `field "stock"`) || !containsAny(rs, "matched nothing in any of the 3 records") {
		t.Errorf("reasons = %v", rs)
	}
}

// "Matched nothing" and "matched something blank" send the model to different
// fixes, so the reason has to say which happened.
func TestEmptyFieldReasonDistinguishesMatchFromNoMatch(t *testing.T) {
	const src = `<div class="results">
	  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price"></span></article>
	  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price"></span></article>
	  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="price"></span></article>
	</div>`

	p := shopProgram()
	p.Expect.Required = []string{"name"} // price is no longer the model's problem

	rs := reasonsFor(t, src, p)
	if !containsAny(rs, "matched an element in 3 records but its text was empty in all of them") {
		t.Errorf("reasons = %v", rs)
	}
}

// A vacuous contract is the case the runtime's floor exists for.
func TestWeakContractStillFailsOnEmptyData(t *testing.T) {
	const src = `<div class="results">
	  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>
	  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price">$20</span></article>
	  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="price">$30</span></article>
	</div>`

	p := extract.Program{
		Root: "div.results > article.card",
		Fields: []extract.Field{
			{Name: "name", Selector: "h3 a", Attr: "text"},
			{Name: "sku", Selector: "span.sku", Attr: "text"},
		},
		Expect: extract.Expectation{MinRows: 1}, // no required fields at all
	}

	rs := reasonsFor(t, src, p)
	if !containsAny(rs, `field "sku"`) {
		t.Errorf("a weak contract let a broken selector through: %v", rs)
	}
}

// Every reason at once, so one re-synthesis fixes everything rather than
// costing a model call per defect.
func TestReportsEveryReasonNotJustTheFirst(t *testing.T) {
	p := shopProgram()
	p.Fields = append(p.Fields,
		extract.Field{Name: "stock", Selector: "span.stock", Attr: "text"},
		extract.Field{Name: "rating", Selector: "span.rating", Attr: "text"},
	)

	rs := reasonsFor(t, shopHTML, p)
	if !containsAny(rs, `field "stock"`) || !containsAny(rs, `field "rating"`) {
		t.Errorf("reasons stopped early: %v", rs)
	}
}

// A root selecting a container class rather than the records still yields a
// few good rows, so row count alone will not catch it.
func TestBroadRootIsCaughtByTheDropRate(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<div class="results">`)
	for i := 0; i < 3; i++ {
		b.WriteString(`<article class="card"><h3><a href="/p/x">Widget</a></h3><span class="price">$1</span></article>`)
	}
	for i := 0; i < 7; i++ {
		b.WriteString(`<article class="card"><h3><a></a></h3><span class="price"></span></article>`)
	}
	b.WriteString(`</div>`)

	p := shopProgram()
	p.Expect.MinRows = 1

	rs := reasonsFor(t, b.String(), p)
	if !containsAny(rs, "broader than the records") {
		t.Errorf("reasons = %v", rs)
	}
}

// A handful of placeholder cards is normal and must not fail an otherwise
// correct program.
func TestAFewEmptyRecordsAreTolerated(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<div class="results">`)
	for i := 0; i < 9; i++ {
		b.WriteString(`<article class="card"><h3><a href="/p/x">Widget</a></h3><span class="price">$1</span></article>`)
	}
	b.WriteString(`<article class="card"><h3><a></a></h3><span class="price"></span></article>`)
	b.WriteString(`</div>`)

	if rs := reasonsFor(t, b.String(), shopProgram()); rs != nil {
		t.Errorf("one placeholder in ten failed the run: %v", rs)
	}
}

// min_rows below 1 must not make verification vacuous — a zero-row result is
// never a successful extraction.
func TestZeroRowsAlwaysFails(t *testing.T) {
	p := shopProgram()
	p.Root = "div.nothing"
	p.Expect.MinRows = 0 // Validate would reject this; Verify must not rely on that

	res := Result{Fields: p.FieldNames(), Stats: map[string]FieldStat{}}
	if err := Verify(p, res); err == nil {
		t.Fatal("zero rows passed verification")
	}
}

func TestFailuresIgnoresUnrelatedErrors(t *testing.T) {
	if fs := Failures(errUnrelated{}); fs != nil {
		t.Errorf("Failures = %v, want nil for an error the runtime did not author", fs)
	}
}

type errUnrelated struct{}

func (errUnrelated) Error() string { return "something else went wrong" }
