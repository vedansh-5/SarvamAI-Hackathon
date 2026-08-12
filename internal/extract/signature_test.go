package extract

import (
	"strings"
	"testing"
)

func sig(t *testing.T, htmlSrc string) string {
	t.Helper()
	s, err := SignatureOf(htmlSrc, NewSigOptions())
	if err != nil {
		t.Fatalf("SignatureOf: %v", err)
	}
	return s
}

// page builds a results page with n records, each carrying the given text.
func page(n int, text string) string {
	var b strings.Builder
	b.WriteString(`<html><body><div id="main"><div class="results">`)
	for i := 0; i < n; i++ {
		b.WriteString(`<article class="card"><h3><a href="/p">` + text + `</a></h3><span class="price">$1</span></article>`)
	}
	b.WriteString(`</div></div></body></html>`)
	return b.String()
}

func TestDeterministic(t *testing.T) {
	src := page(3, "Widget")
	if sig(t, src) != sig(t, src) {
		t.Error("signature is not deterministic")
	}
}

// The whole point: content changes must not invalidate a learned program.
func TestSameStructureDifferentContent(t *testing.T) {
	if a, b := sig(t, page(3, "Widget")), sig(t, page(3, "Gadget")); a != b {
		t.Errorf("text change moved the signature: %s vs %s", a, b)
	}
}

// A search returning 10 results is the same page as one returning 25. Without
// collapsing, the cache would miss on nearly every real visit.
func TestRecordCountDoesNotChangeSignature(t *testing.T) {
	if a, b := sig(t, page(3, "x")), sig(t, page(25, "x")); a != b {
		t.Errorf("record count moved the signature: %s vs %s", a, b)
	}
}

// Zero records is genuinely a different page — an empty-state view usually has
// different markup, and a program built for records should not replay on it.
func TestEmptyResultsDiffersFromPopulated(t *testing.T) {
	if a, b := sig(t, page(0, "")), sig(t, page(5, "x")); a == b {
		t.Error("empty and populated pages signed identically")
	}
}

// Build hashes churn on every deploy. If they reached the signature, every
// release would invalidate every cached program on the site.
func TestBuildHashesDoNotChangeSignature(t *testing.T) {
	v1 := `<html><body><div class="list_9f3a2b"><p class="row_1c4d5e">a</p><p class="row_1c4d5e">b</p><p class="row_1c4d5e">c</p></div></body></html>`
	v2 := `<html><body><div class="list_77c11e"><p class="row_ab0932">a</p><p class="row_ab0932">b</p><p class="row_ab0932">c</p></div></body></html>`

	if a, b := sig(t, v1), sig(t, v2); a != b {
		t.Errorf("rebuild moved the signature: %s vs %s", a, b)
	}
}

// The mutation cases from the test strategy: each is a redesign that must
// invalidate the cache, because a program written for the old shape would
// return wrong data against the new one.
func TestMutationsChangeSignature(t *testing.T) {
	base := page(4, "x")

	mutations := map[string]string{
		"renamed class": strings.ReplaceAll(base, `class="card"`, `class="product"`),
		"wrapper div": strings.ReplaceAll(
			strings.ReplaceAll(base, `<article class="card">`, `<div class="wrap"><article class="card">`),
			`</article>`, `</article></div>`),
		"removed field": strings.ReplaceAll(base, `<span class="price">$1</span>`, ``),
		"changed tag":   strings.ReplaceAll(base, `<article class="card">`, `<section class="card">`),
	}

	baseSig := sig(t, base)
	for name, mutated := range mutations {
		if got := sig(t, mutated); got == baseSig {
			t.Errorf("%s did not change the signature", name)
		}
	}
}

// Scripts, styles and analytics tags are injected and removed constantly. They
// carry no structure worth keying on.
func TestScriptsAndStylesIgnored(t *testing.T) {
	base := page(3, "x")
	noisy := strings.Replace(base, `<div class="results">`,
		`<script>var x=1</script><style>.a{color:red}</style><div class="results">`, 1)

	if a, b := sig(t, base), sig(t, noisy); a != b {
		t.Errorf("script/style injection moved the signature: %s vs %s", a, b)
	}
}

func TestDepthLimitBoundsSignature(t *testing.T) {
	deep := `<html><body>` + strings.Repeat(`<div class="x">`, 40) + `hi` + strings.Repeat(`</div>`, 40) + `</body></html>`

	shallow, err := SignatureOf(deep, SigOptions{MaxDepth: 3, CollapseRepeats: true})
	if err != nil {
		t.Fatal(err)
	}
	full, err := SignatureOf(deep, SigOptions{MaxDepth: 30, CollapseRepeats: true})
	if err != nil {
		t.Fatal(err)
	}
	if shallow == full {
		t.Error("MaxDepth had no effect")
	}
}

func TestSignatureShape(t *testing.T) {
	s := sig(t, page(2, "x"))
	if len(s) != 16 {
		t.Errorf("signature length = %d, want 16", len(s))
	}
	if strings.ContainsAny(s, "ghijklmnopqrstuvwxyz") {
		t.Errorf("signature is not hex: %q", s)
	}
}

func TestEmptyInput(t *testing.T) {
	if _, err := SignatureOf("", NewSigOptions()); err != nil {
		t.Errorf("SignatureOf(\"\"): %v", err)
	}
}
