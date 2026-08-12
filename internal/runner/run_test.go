package runner

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/extract"
)

const shopHTML = `<!doctype html>
<html><body>
  <div id="main"><div class="results">
    <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>
    <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price">$20</span></article>
    <article class="card"><h3><a href="/p/3">Doohickey</a></h3><span class="price">$30</span></article>
  </div></div>
</body></html>`

func shopProgram() extract.Program {
	return extract.Program{
		Root: "div.results > article.card",
		Fields: []extract.Field{
			{Name: "name", Selector: "h3 a", Attr: "text"},
			{Name: "link", Selector: "h3 a", Attr: "href"},
			{Name: "price", Selector: "span.price", Attr: "text"},
		},
		Expect: extract.Expectation{MinRows: 3, Required: []string{"name", "link", "price"}},
	}
}

func mustRun(t *testing.T, src string, p extract.Program, opts Options) Result {
	t.Helper()
	res, err := RunHTML(src, p, opts)
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}
	return res
}

func TestRunExtractsRows(t *testing.T) {
	res := mustRun(t, shopHTML, shopProgram(), Options{})

	if len(res.Rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(res.Rows), res.Rows)
	}
	if got := res.Rows[0]["name"]; got != "Widget" {
		t.Errorf("row 0 name = %q, want %q", got, "Widget")
	}
	if got := res.Rows[2]["price"]; got != "$30" {
		t.Errorf("row 2 price = %q, want %q", got, "$30")
	}
	if got := res.Fields; strings.Join(got, ",") != "name,link,price" {
		t.Errorf("field order = %v, want the program's order", got)
	}
}

// A link that stays relative is useless the moment it leaves the browser, and
// the destination for this data is a note.
func TestResolvesRelativeURLs(t *testing.T) {
	base, err := url.Parse("https://shop.example/search?q=w")
	if err != nil {
		t.Fatal(err)
	}

	res := mustRun(t, shopHTML, shopProgram(), Options{Base: base})

	want := "https://shop.example/p/1"
	if got := res.Rows[0]["link"]; got != want {
		t.Errorf("link = %q, want %q", got, want)
	}
}

func TestLeavesRelativeURLsAloneWithoutBase(t *testing.T) {
	res := mustRun(t, shopHTML, shopProgram(), Options{})
	if got := res.Rows[0]["link"]; got != "/p/1" {
		t.Errorf("link = %q, want the raw attribute", got)
	}
}

// Absolute URLs and non-URL attributes must survive resolution untouched.
func TestResolutionLeavesOtherValuesAlone(t *testing.T) {
	const src = `<ul><li class="r"><a href="https://other.example/x" title="Keep /me">A</a></li>
	<li class="r"><a href="https://other.example/y" title="Keep /me">B</a></li>
	<li class="r"><a href="https://other.example/z" title="Keep /me">C</a></li></ul>`

	base, _ := url.Parse("https://shop.example/search")
	p := extract.Program{
		Root: "li.r",
		Fields: []extract.Field{
			{Name: "href", Selector: "a", Attr: "href"},
			{Name: "title", Selector: "a", Attr: "title"},
		},
		Expect: extract.Expectation{MinRows: 3},
	}

	res := mustRun(t, src, p, Options{Base: base})
	if got := res.Rows[0]["href"]; got != "https://other.example/x" {
		t.Errorf("absolute href was rewritten to %q", got)
	}
	if got := res.Rows[0]["title"]; got != "Keep /me" {
		t.Errorf("title was resolved as a URL: %q", got)
	}
}

// An empty selector means the record element itself. Without the special case
// it would be compiled as a selector and fail, or match a descendant.
func TestEmptySelectorMeansTheRecord(t *testing.T) {
	const src = `<div><p class="q">one</p><p class="q">two</p><p class="q">three</p></div>`

	p := extract.Program{
		Root:   "p.q",
		Fields: []extract.Field{{Name: "line", Selector: "", Attr: "text"}},
		Expect: extract.Expectation{MinRows: 3, Required: []string{"line"}},
	}

	res := mustRun(t, src, p, Options{})
	if len(res.Rows) != 3 || res.Rows[1]["line"] != "two" {
		t.Fatalf("rows = %+v", res.Rows)
	}
}

// Field selectors must be scoped to their own record. A selector matching in
// several records would otherwise copy record 1's value into every row.
func TestFieldSelectorsAreScopedToTheirRecord(t *testing.T) {
	res := mustRun(t, shopHTML, shopProgram(), Options{})

	seen := map[string]bool{}
	for _, row := range res.Rows {
		if seen[row["name"]] {
			t.Fatalf("duplicate value %q across rows: %+v", row["name"], res.Rows)
		}
		seen[row["name"]] = true
	}
}

func TestTextIsCollapsed(t *testing.T) {
	const src = `<div><section class="r"><p>  hello

	world  </p></section><section class="r"><p>a b</p></section><section class="r"><p>c</p></section></div>`

	p := extract.Program{
		Root:   "section.r",
		Fields: []extract.Field{{Name: "t", Selector: "p", Attr: "text"}},
		Expect: extract.Expectation{MinRows: 3},
	}

	res := mustRun(t, src, p, Options{})
	if got := res.Rows[0]["t"]; got != "hello world" {
		t.Errorf("text = %q, want %q", got, "hello world")
	}
}

// Placeholder cards sharing the records' shape are common on real listings.
// They carry no data, so they must not become blank rows.
func TestDropsEmptyRows(t *testing.T) {
	const src = `<div class="results">
	  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>
	  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price">$20</span></article>
	  <article class="card"><h3><a>  </a></h3><span class="price"></span></article>
	</div>`

	res := mustRun(t, src, shopProgram(), Options{})

	if len(res.Rows) != 2 {
		t.Errorf("got %d rows, want the placeholder dropped: %+v", len(res.Rows), res.Rows)
	}
	if res.Matched != 3 {
		t.Errorf("Matched = %d, want 3 — the drop must stay visible", res.Matched)
	}
	if res.Dropped != 1 {
		t.Errorf("Dropped = %d, want 1", res.Dropped)
	}
}

func TestStatsSeparateNoMatchFromEmptyMatch(t *testing.T) {
	const src = `<div class="results">
	  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price"></span></article>
	  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price"></span></article>
	  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="price"></span></article>
	</div>`

	p := shopProgram()
	p.Fields = append(p.Fields, extract.Field{Name: "stock", Selector: "span.stock", Attr: "text"})
	p.Expect.Required = []string{"name"}

	res := mustRun(t, src, p, Options{})

	if st := res.Stats["price"]; st.Matched != 3 || st.NonEmpty != 0 {
		t.Errorf("price stats = %+v, want matched but empty", st)
	}
	if st := res.Stats["stock"]; st.Matched != 0 || st.NonEmpty != 0 {
		t.Errorf("stock stats = %+v, want no match at all", st)
	}
	if st := res.Stats["name"]; st.NonEmpty != 3 {
		t.Errorf("name stats = %+v, want 3 non-empty", st)
	}
}

// A selector the runtime cannot compile is a defect in what the model wrote.
// It has to be reported as such, with the parse error, or re-synthesis has
// nothing to correct.
func TestBadRootSelectorIsAProgramError(t *testing.T) {
	p := shopProgram()
	p.Root = "div.results >>> article"

	_, err := RunHTML(shopHTML, p, Options{})
	if err == nil {
		t.Fatal("want an error")
	}
	if fs := Failures(err); len(fs) != 1 || !strings.Contains(fs[0], "does not parse") {
		t.Errorf("failures = %v, want a parse explanation", fs)
	}
}

func TestBadFieldSelectorIsAProgramError(t *testing.T) {
	p := shopProgram()
	p.Fields[1].Selector = "a[href"

	_, err := RunHTML(shopHTML, p, Options{})
	if err == nil {
		t.Fatal("want an error")
	}
	if fs := Failures(err); len(fs) != 1 || !strings.Contains(fs[0], `field "link"`) {
		t.Errorf("failures = %v, want the offending field named", fs)
	}
}

func TestInvalidProgramIsRejectedBeforeRunning(t *testing.T) {
	p := shopProgram()
	p.Fields[0].Attr = "innerHTML" // not in AllowedAttrs

	if _, err := RunHTML(shopHTML, p, Options{}); err == nil {
		t.Fatal("want an error for an attr the runner does not implement")
	}
}

// A root broad enough to match a thousand elements is matching layout, and
// extracting from all of it would put nonsense in front of the user.
func TestRunawayRootIsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("<div>")
	for i := 0; i < 20; i++ {
		b.WriteString(`<span class="x">v</span>`)
	}
	b.WriteString("</div>")

	p := extract.Program{
		Root:   "span.x",
		Fields: []extract.Field{{Name: "v", Selector: "", Attr: "text"}},
		Expect: extract.Expectation{MinRows: 1},
	}

	res, err := RunHTML(b.String(), p, Options{MaxRows: 5})
	if err == nil {
		t.Fatal("want an error")
	}
	if res.Matched != 20 {
		t.Errorf("Matched = %d, want the count that triggered the cap", res.Matched)
	}
	if fs := Failures(err); len(fs) != 1 || !strings.Contains(fs[0], "above the limit") {
		t.Errorf("failures = %v", fs)
	}
}

// The runner has to work on the same fixture the candidate finder and the
// synthesizer were built against, or the pipeline does not join up.
func TestRunsAgainstTheSharedFixture(t *testing.T) {
	src, err := os.ReadFile("../extract/testdata/products.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	base, _ := url.Parse("https://shop.example/search")
	res, err := Execute(string(src), shopProgram(), Options{Base: base})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(res.Rows) != 4 {
		t.Fatalf("got %d rows, want the fixture's 4 products: %+v", len(res.Rows), res.Rows)
	}
	if got := res.Rows[3]["link"]; got != "https://shop.example/p/4" {
		t.Errorf("link = %q", got)
	}
}
