package webstyle

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

const shopHTML = `<div class="results">
  <article class="card"><h3>Widget</h3><span class="price">$250</span></article>
  <article class="card"><h3>Gadget</h3><span class="price">$80</span></article>
</div>`

func page(t *testing.T, src string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc
}

func highlight() Program {
	return Program{Rules: []Rule{{Selector: "span.price", Emphasis: "highlight"}}}
}

func reasons(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var invalid *InvalidError
	if !asInvalid(err, &invalid) {
		t.Fatalf("error was not a refusal: %v", err)
	}
	return invalid.Reasons
}

func asInvalid(err error, out **InvalidError) bool {
	if e, ok := err.(*InvalidError); ok {
		*out = e
		return true
	}
	return false
}

func mentions(rs []string, want string) bool {
	for _, r := range rs {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func TestAGoodRestylePasses(t *testing.T) {
	if err := Validate(highlight(), page(t, shopHTML)); err != nil {
		t.Errorf("a workable restyle was refused: %v", err)
	}
}

// The floor that matters: a restyle whose selector matches nothing reports
// success and leaves the page looking exactly as it did.
func TestASelectorMatchingNothingIsRefused(t *testing.T) {
	p := highlight()
	p.Rules[0].Selector = "span.discount"

	rs := reasons(t, Validate(p, page(t, shopHTML)))
	if !mentions(rs, "matched nothing on the page") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestNonsenseSelectorIsRefused(t *testing.T) {
	p := highlight()
	p.Rules[0].Selector = "span[[["

	rs := reasons(t, Validate(p, page(t, shopHTML)))
	if !mentions(rs, "is not a selector") {
		t.Errorf("reasons = %v", rs)
	}
}

// The model picks what to mark and how strongly. It never writes a colour, a
// length, or any other value, so there is nothing to escape and nothing that
// can carry a url() off the page.
func TestAnUnknownEmphasisIsRefused(t *testing.T) {
	p := highlight()
	p.Rules[0].Emphasis = "background: url(https://example.com/leak.png)"

	rs := reasons(t, Validate(p, page(t, shopHTML)))
	if !mentions(rs, "is not a way to mark something") {
		t.Errorf("reasons = %v", rs)
	}
}

func TestEveryEmphasisRendersSomething(t *testing.T) {
	for _, name := range EmphasisNames() {
		p := Program{Rules: []Rule{{Selector: "span.price", Emphasis: name}}}
		if err := Validate(p, page(t, shopHTML)); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
		if css := Sheet(p); !strings.Contains(css, "span.price {") {
			t.Errorf("%s rendered nothing: %q", name, css)
		}
	}
}

// Colour reaches the page as a token, so the same rule can wear a different
// palette depending on what it is laid over.
func TestSheetUsesTokensRatherThanColours(t *testing.T) {
	css := Sheet(highlight())
	if !strings.Contains(css, "span.price {") {
		t.Errorf("sheet = %q", css)
	}
	if !strings.Contains(css, "var(--vans-tint)") {
		t.Errorf("the tint is not a token: %q", css)
	}
	if strings.Contains(css, "box-shadow") || strings.Contains(css, "border:") || strings.Contains(css, "outline") {
		t.Errorf("a highlight drew a border around itself: %q", css)
	}
	if strings.Contains(css, "#") || strings.Contains(css, "rgba(") {
		t.Errorf("a literal colour reached the sheet: %q", css)
	}
	if !strings.Contains(css, "transition:") {
		t.Errorf("the mark does not ease in: %q", css)
	}
}

func TestBothPalettesReachTheScript(t *testing.T) {
	script, err := Render(highlight(), page(t, shopHTML))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, "--vans-tint:rgba(250,204,21,.28)") {
		t.Errorf("the light palette is missing:\n%s", script)
	}
	if !strings.Contains(script, "--vans-tint:rgba(250,204,21,.20)") {
		t.Errorf("the dark palette is missing:\n%s", script)
	}
	if !strings.Contains(script, "pageIsDark") {
		t.Errorf("the page is never measured:\n%s", script)
	}
}

func TestRenderCarriesTheSheetAsAString(t *testing.T) {
	script, err := Render(highlight(), page(t, shopHTML))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, "el.textContent =") {
		t.Errorf("the sheet did not reach textContent:\n%s", script)
	}
	if !strings.Contains(script, `"vans-style"`) {
		t.Errorf("the style element has no stable id:\n%s", script)
	}
	if !strings.Contains(script, "span.price") {
		t.Errorf("the selector did not reach the script:\n%s", script)
	}
}

func TestRenderRefusesWhatValidateRefuses(t *testing.T) {
	p := highlight()
	p.Rules[0].Selector = "span.nothing"

	if _, err := Render(p, page(t, shopHTML)); err == nil {
		t.Fatal("a restyle that matches nothing was rendered anyway")
	}
}

func TestClearRemovesTheElement(t *testing.T) {
	if !strings.Contains(Clear(), "removeChild") {
		t.Error("clearing does not remove the style element")
	}
}

func TestPreviewSaysWhatWillChange(t *testing.T) {
	got := Preview(highlight())
	if !strings.Contains(got, "span.price") || !strings.Contains(got, "highlight") {
		t.Errorf("preview = %q", got)
	}
}
