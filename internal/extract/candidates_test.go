package extract

import (
	"os"
	"strings"
	"testing"
)

func loadHTML(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func find(t *testing.T, name string) []Candidate {
	t.Helper()
	got, err := FindCandidates(loadHTML(t, name), Options{})
	if err != nil {
		t.Fatalf("FindCandidates: %v", err)
	}
	return got
}

func TestFindsProductCards(t *testing.T) {
	got := find(t, "products.html")
	if len(got) == 0 {
		t.Fatal("no candidates found")
	}

	top := got[0]
	if top.RecordCount != 4 {
		t.Errorf("RecordCount = %d, want 4", top.RecordCount)
	}
	if !strings.Contains(top.Selector, "article.card") {
		t.Errorf("Selector = %q, want it to name article.card", top.Selector)
	}
	if !strings.Contains(top.Selector, "results") {
		t.Errorf("Selector = %q, want it anchored on the container", top.Selector)
	}
}

// Layout scaffolding repeats too. Without a text check it outranks real data,
// because there is often more of it.
func TestEmptyRepeatsRejected(t *testing.T) {
	for _, c := range find(t, "products.html") {
		if strings.Contains(c.Selector, "i.sp") {
			t.Errorf("empty spacer run reported as data: %+v", c)
		}
	}
}

func TestSkipsScriptAndStyle(t *testing.T) {
	for _, c := range find(t, "products.html") {
		if strings.Contains(c.Selector, "style") || strings.Contains(c.Selector, "script") {
			t.Errorf("candidate inside script/style: %q", c.Selector)
		}
	}
}

func TestSamplesCarryRecordText(t *testing.T) {
	top := find(t, "products.html")[0]
	if len(top.Samples) == 0 {
		t.Fatal("no samples")
	}
	if !strings.Contains(top.Samples[0], "Widget") {
		t.Errorf("first sample = %q, want the first record's text", top.Samples[0])
	}
}

func TestSkeletonShowsStructureNotJustText(t *testing.T) {
	top := find(t, "products.html")[0]
	for _, want := range []string{"<article", "<h3>", "href=", "price"} {
		if !strings.Contains(top.Skeleton, want) {
			t.Errorf("skeleton missing %q:\n%s", want, top.Skeleton)
		}
	}
}

// Build-hashed class names change on every deploy. If they leak into the
// fingerprint, siblings still group (they share the hash) — but the selector
// rots the moment the site rebuilds.
func TestGeneratedClassesStrippedFromSelector(t *testing.T) {
	got := find(t, "hashed.html")
	if len(got) == 0 {
		t.Fatal("no candidates found")
	}
	if strings.Contains(got[0].Selector, "9f3a2b") || strings.Contains(got[0].Selector, "1c4d5e") {
		t.Errorf("build hash leaked into selector: %q", got[0].Selector)
	}
	if !strings.Contains(got[0].Selector, "row") {
		t.Errorf("selector lost the readable stem: %q", got[0].Selector)
	}
}

func TestNormalizeClass(t *testing.T) {
	cases := []struct{ in, want string }{
		{"card", "card"},
		{"product-card", "product-card"},
		{"row_1c4d5e", "row"},
		{"card-3f9a2b", "card"},
		{"css-1a2b3c4d", "css"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := normalizeClass(tc.in); got != tc.want {
			t.Errorf("normalizeClass(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMinRecordsRespected(t *testing.T) {
	src := `<html><body><div class="x"><p class="r">a</p><p class="r">b</p></div></body></html>`
	got, err := FindCandidates(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if strings.Contains(c.Selector, "p.r") {
			t.Errorf("2 siblings reported as a data region: %+v", c)
		}
	}
}

func TestNestedRegionsDeduped(t *testing.T) {
	got := find(t, "products.html")
	seen := map[string]bool{}
	for _, c := range got {
		if seen[c.Selector] {
			t.Errorf("duplicate selector %q", c.Selector)
		}
		seen[c.Selector] = true
	}
}

func TestEmptyDocument(t *testing.T) {
	got, err := FindCandidates("", Options{})
	if err != nil {
		t.Fatalf("FindCandidates on empty input: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d candidates from empty input", len(got))
	}
}
