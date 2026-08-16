package extract

import (
	"strings"
	"testing"
)

// hnLike reproduces the shape that exposed the gap: a story's title in one
// row, its score in the next, and a spacer between records. No selector
// relative to a single row can reach the score.
const hnLike = `<!doctype html><html><body><table id="hnmain"><tbody>
<tr id="bigbox"><td><table><tbody>
  <tr class="athing submission"><td class="title"><span class="titleline"><a href="/s/1">First story</a></span></td></tr>
  <tr><td class="subtext"><span class="score">318 points</span> by <a href="/u/alice">alice</a></td></tr>
  <tr class="spacer"></tr>
  <tr class="athing submission"><td class="title"><span class="titleline"><a href="/s/2">Second story</a></span></td></tr>
  <tr><td class="subtext"><span class="score">201 points</span> by <a href="/u/bob">bob</a></td></tr>
  <tr class="spacer"></tr>
  <tr class="athing submission"><td class="title"><span class="titleline"><a href="/s/3">Third story</a></span></td></tr>
  <tr><td class="subtext"><span class="score">97 points</span> by <a href="/u/carol">carol</a></td></tr>
  <tr class="spacer"></tr>
  <tr class="athing submission"><td class="title"><span class="titleline"><a href="/s/4">Fourth story</a></span></td></tr>
  <tr><td class="subtext"><span class="score">12 points</span> by <a href="/u/dave">dave</a></td></tr>
  <tr class="spacer"></tr>
</tbody></table></td></tr>
</tbody></table></body></html>`

func findSpanned(t *testing.T, src string) Candidate {
	t.Helper()

	got, err := FindCandidates(src, Options{})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	for _, c := range got {
		if c.Span > 1 {
			return c
		}
	}
	t.Fatalf("no multi-element record was found among %d candidates: %+v", len(got), got)
	return Candidate{}
}

func TestFindsRecordsSpanningSiblings(t *testing.T) {
	c := findSpanned(t, hnLike)

	if c.Span != 3 {
		t.Errorf("Span = %d, want 3 (title, subtext, spacer)", c.Span)
	}
	if c.RecordCount != 4 {
		t.Errorf("RecordCount = %d, want 4", c.RecordCount)
	}
	if !strings.Contains(c.Selector, "athing") {
		t.Errorf("Selector = %q, want it to start at the title row", c.Selector)
	}
}

// The whole point: the model has to be able to see the score, or it cannot
// write a selector for it.
func TestSpannedSkeletonShowsTheWholeRecord(t *testing.T) {
	c := findSpanned(t, hnLike)

	if !strings.Contains(c.Skeleton, "titleline") {
		t.Errorf("skeleton lost the title:\n%s", c.Skeleton)
	}
	if !strings.Contains(c.Skeleton, "score") {
		t.Errorf("skeleton lost the score, which is the reason spans exist:\n%s", c.Skeleton)
	}
}

func TestSpannedSamplesCoverTheWholeRecord(t *testing.T) {
	c := findSpanned(t, hnLike)

	if len(c.Samples) == 0 {
		t.Fatal("no samples")
	}
	if !strings.Contains(c.Samples[0], "First story") || !strings.Contains(c.Samples[0], "318 points") {
		t.Errorf("sample did not join the record's elements: %q", c.Samples[0])
	}
}

// Every rotation of the period is tried. The one starting on the spacer must
// lose to the one starting on content, or records begin in the wrong place.
func TestRecordBoundaryStartsOnContent(t *testing.T) {
	c := findSpanned(t, hnLike)

	if strings.Contains(c.Selector, "spacer") {
		t.Errorf("Selector = %q, want the record to start on a row with content", c.Selector)
	}
}

// A root that matches more elements than there are records would produce
// overlapping windows on replay.
func TestRejectsAnIndistinctRecordStart(t *testing.T) {
	got, err := FindCandidates(hnLike, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.Span > 1 && strings.HasSuffix(c.Selector, "> tr") {
			t.Errorf("a bare tr was accepted as a record start: %+v", c)
		}
	}
}

// The ordinary single-element case must be unaffected, and must still be found
// when siblings are not adjacent.
func TestSingleElementRecordsStillWork(t *testing.T) {
	const src = `<div class="results">
	  <article class="card"><h3>One</h3></article>
	  <article class="card"><h3>Two</h3></article>
	  <div class="ad">sponsored</div>
	  <article class="card"><h3>Three</h3></article>
	</div>`

	got, err := FindCandidates(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("no candidates")
	}
	top := got[0]
	if top.Span > 1 {
		t.Errorf("a simple list was read as spanning records: %+v", top)
	}
	if top.RecordCount != 3 {
		t.Errorf("RecordCount = %d, want 3 despite the ad between them", top.RecordCount)
	}
}

func TestSpanIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("<div>")
	for i := 0; i < 40; i++ {
		b.WriteString(`<p class="a">a</p><p class="b">b</p><p class="c">c</p><p class="d">d</p>`)
		b.WriteString(`<p class="e">e</p><p class="f">f</p><p class="g">g</p><p class="h">h</p>`)
	}
	b.WriteString("</div>")

	got, err := FindCandidates(b.String(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.Span > MaxRootSpan {
			t.Errorf("Span = %d, above the bound of %d", c.Span, MaxRootSpan)
		}
	}
}

// A table whose first row is a header must not shift every record by one.
func TestOffsetRegionIsFound(t *testing.T) {
	const src = `<table><tbody>
	  <tr class="head"><th>Name</th></tr>
	  <tr class="row"><td>One</td></tr>
	  <tr class="detail"><td>first detail</td></tr>
	  <tr class="row"><td>Two</td></tr>
	  <tr class="detail"><td>second detail</td></tr>
	  <tr class="row"><td>Three</td></tr>
	  <tr class="detail"><td>third detail</td></tr>
	</tbody></table>`

	c := findSpanned(t, src)
	if c.Span != 2 {
		t.Errorf("Span = %d, want 2", c.Span)
	}
	if c.RecordCount != 3 {
		t.Errorf("RecordCount = %d, want 3 — the header is not a record", c.RecordCount)
	}
	if !strings.Contains(c.Selector, "row") {
		t.Errorf("Selector = %q, want it anchored on the row", c.Selector)
	}
}
