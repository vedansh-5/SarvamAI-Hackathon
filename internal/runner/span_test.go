package runner

import (
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/extract"
)

const spannedHTML = `<table><tbody>
  <tr class="athing"><td class="title"><a href="/s/1">First story</a></td></tr>
  <tr><td class="subtext"><span class="score">318 points</span></td></tr>
  <tr class="spacer"></tr>
  <tr class="athing"><td class="title"><a href="/s/2">Second story</a></td></tr>
  <tr><td class="subtext"><span class="score">201 points</span></td></tr>
  <tr class="spacer"></tr>
  <tr class="athing"><td class="title"><a href="/s/3">Third story</a></td></tr>
  <tr><td class="subtext"><span class="score">97 points</span></td></tr>
  <tr class="spacer"></tr>
</tbody></table>`

func spannedProgram() extract.Program {
	return extract.Program{
		Root:     "tr.athing",
		RootSpan: 3,
		Fields: []extract.Field{
			{Name: "title", Selector: "td.title a", Attr: "text"},
			{Name: "link", Selector: "td.title a", Attr: "href"},
			{Name: "score", Selector: "span.score", Attr: "text"},
		},
		Expect: extract.Expectation{MinRows: 1, Required: []string{"title", "link", "score"}},
	}
}

// The case the whole change exists for: a field that lives in a sibling of the
// element the root matched.
func TestExtractsAFieldFromASiblingElement(t *testing.T) {
	res, err := RunHTML(spannedHTML, spannedProgram(), Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}

	if len(res.Rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(res.Rows), res.Rows)
	}
	if got := res.Rows[0]["score"]; got != "318 points" {
		t.Errorf("row 0 score = %q, want it read from the following row", got)
	}
	if got := res.Rows[2]["title"]; got != "Third story" {
		t.Errorf("row 2 title = %q", got)
	}
	if got := res.Rows[1]["score"]; got != "201 points" {
		t.Errorf("row 1 score = %q — scores leaked between records", got)
	}
}

// Each record must take its own sibling, not the first one on the page.
func TestSpannedFieldsDoNotLeakBetweenRecords(t *testing.T) {
	res, err := RunHTML(spannedHTML, spannedProgram(), Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}

	seen := map[string]bool{}
	for _, row := range res.Rows {
		if seen[row["score"]] {
			t.Fatalf("duplicate score %q across records: %+v", row["score"], res.Rows)
		}
		seen[row["score"]] = true
	}
}

// A root matching more often than the records repeat would otherwise build
// windows that share elements, reporting the same data several times.
func TestWindowsDoNotOverlap(t *testing.T) {
	const src = `<div>
	  <p class="r">one</p><p class="r">two</p>
	  <p class="r">three</p><p class="r">four</p>
	  <p class="r">five</p><p class="r">six</p>
	</div>`

	p := extract.Program{
		Root:     "p.r",
		RootSpan: 2,
		Fields:   []extract.Field{{Name: "text", Selector: "", Attr: "text"}},
		Expect:   extract.Expectation{MinRows: 1, Required: []string{"text"}},
	}

	res, err := RunHTML(src, p, Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("got %d rows, want 3 non-overlapping windows: %+v", len(res.Rows), res.Rows)
	}
	want := []string{"one", "three", "five"}
	for i, w := range want {
		if res.Rows[i]["text"] != w {
			t.Errorf("row %d = %q, want %q", i, res.Rows[i]["text"], w)
		}
	}
}

// An empty selector means the record, and a record is more than its first
// element once it spans several.
func TestEmptySelectorTakesTheFirstElementOfTheWindow(t *testing.T) {
	p := spannedProgram()
	p.Fields = []extract.Field{{Name: "row", Selector: "", Attr: "text"}}
	p.Expect = extract.Expectation{MinRows: 1, Required: []string{"row"}}

	res, err := RunHTML(spannedHTML, p, Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}
	if got := res.Rows[0]["row"]; got != "First story" {
		t.Errorf("row = %q, want the first element's text", got)
	}
}

// A selector naming a window element itself, not a descendant of one.
func TestSelectorCanMatchAWindowElementDirectly(t *testing.T) {
	p := spannedProgram()
	p.Fields = append(p.Fields, extract.Field{Name: "spacer_tag", Selector: "tr.spacer", Attr: "text"})

	res, err := RunHTML(spannedHTML, p, Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}
	if st := res.Stats["spacer_tag"]; st.Matched != 3 {
		t.Errorf("a window element was not selectable directly: %+v", st)
	}
}

// The last record in a list often lacks the trailing separator the others
// have. A short window is still a record.
func TestAShortFinalWindowIsStillARecord(t *testing.T) {
	const src = `<table><tbody>
	  <tr class="athing"><td class="title"><a href="/s/1">First story</a></td></tr>
	  <tr><td class="subtext"><span class="score">318 points</span></td></tr>
	  <tr class="spacer"></tr>
	  <tr class="athing"><td class="title"><a href="/s/2">Second story</a></td></tr>
	  <tr><td class="subtext"><span class="score">201 points</span></td></tr>
	</tbody></table>`

	res, err := RunHTML(src, spannedProgram(), Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(res.Rows), res.Rows)
	}
	if got := res.Rows[1]["score"]; got != "201 points" {
		t.Errorf("the truncated final record lost its score: %q", got)
	}
}

// Programs cached before spans existed decode with RootSpan zero and must
// keep working — every one of them describes a single-element record.
func TestZeroSpanIsASingleElement(t *testing.T) {
	p := shopProgram()
	if p.RootSpan != 0 {
		t.Fatal("fixture no longer exercises the zero case")
	}
	if p.Span() != 1 {
		t.Fatalf("Span() = %d, want 1", p.Span())
	}

	res, err := RunHTML(shopHTML, p, Options{})
	if err != nil {
		t.Fatalf("RunHTML: %v", err)
	}
	if len(res.Rows) != 3 {
		t.Errorf("got %d rows, want the pre-span behaviour unchanged", len(res.Rows))
	}
}

func TestSpanIsValidated(t *testing.T) {
	p := spannedProgram()
	p.RootSpan = extract.MaxRootSpan + 1

	_, err := RunHTML(spannedHTML, p, Options{})
	if err == nil {
		t.Fatal("an unbounded span was accepted")
	}
	if fs := Failures(err); len(fs) == 0 || !strings.Contains(fs[0], "root_span") {
		t.Errorf("failures = %v", fs)
	}
}
