//go:build integration

package appleevents

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"

	"github.com/vedansh/vans/internal/webact"
)

// Run with: go test -tags integration -run Live.*Page ./internal/adapter/appleevents/
//
// This drives the browser the user is signed into. It acts on the tab VANS
// opened for itself, addressed by id, and never on the tab in front — the
// point of the exercise is to prove the execute step works, not to type into
// whatever someone is reading.
//
// Nothing is submitted. The program types and reads the value back; no click
// on a search button, no Enter, so nothing leaves the machine.
func liveChrome(t *testing.T) (*Chrome, context.Context) {
	t.Helper()

	c := &Chrome{Runner: Osascript{}, Timeout: 25 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	if err := c.Navigate(ctx, "https://duckduckgo.com"); err != nil {
		if Fix(err) != "" {
			t.Skipf("%v — %s", err, Fix(err))
		}
		t.Skipf("no browser to drive: %v", err)
	}
	t.Cleanup(func() {
		if c.tab == "" {
			return
		}
		id, err := jsLiteral(c.tab)
		if err != nil {
			return
		}
		_ = c.run(context.Background(), fmt.Sprintf(
			"reply(function () {\n  const app = chrome();\n  tabById(app, %s).close();\n  return 'closed';\n});", id), nil)
	})

	return c, ctx
}

// onOwnTab executes a snippet against VANS's tab and returns what it said.
//
// The product runs a program on the tab in front (Do). A test may not, so it
// goes through the same execute call aimed at the tab VANS owns.
func onOwnTab(ctx context.Context, c *Chrome, js string) (string, error) {
	source, err := jsLiteral(js)
	if err != nil {
		return "", err
	}
	var out string
	err = c.run(ctx, c.onTab(fmt.Sprintf(
		"return String(app.execute(tab, { javascript: %s }));", source)), &out)
	return out, err
}

func TestLiveProgramRunsInAPage(t *testing.T) {
	c, ctx := liveChrome(t)

	src, err := c.OuterHTML(ctx)
	if err != nil {
		t.Fatalf("read the page: %v", err)
	}
	doc, err := webact.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	const typed = "vans live check"

	t.Logf("page: %d bytes", len(src))
	if u, err := c.URL(ctx); err == nil {
		t.Logf("settled at: %s", u)
	}

	var program webact.Program
	for _, selector := range []string{"textarea[name=q]", "#searchbox_input", "textarea", "input[name=q]", "input[type=text]"} {
		candidate := webact.Program{
			URL: "https://duckduckgo.com",
			Steps: []webact.Step{{
				Do: "type", Selector: selector, Text: typed,
				Describe: "Type into the search box",
			}},
		}
		err := webact.Validate(candidate, doc)
		if err == nil {
			t.Logf("selector that survived validation: %s", selector)
			program = candidate
			break
		}
		t.Logf("selector %-22s refused: %v", selector, err)
	}
	if len(program.Steps) == 0 {
		for i, n := range cascadia.QueryAll(doc, cascadia.MustCompile("input, textarea, [contenteditable]")) {
			if i >= 15 {
				break
			}
			t.Logf("input[%d]: %s", i, attrs(n))
		}
		t.Fatal("no search box on the page passed validation; the page changed shape")
	}

	script, err := webact.Render(program, doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	got, err := onOwnTab(ctx, c, script)
	if err != nil {
		t.Fatalf("execute the program: %v", err)
	}
	if !strings.Contains(got, `"ok":true`) {
		t.Fatalf("the page refused the program: %s", got)
	}

	selector := program.Steps[0].Selector
	readBack, err := onOwnTab(ctx, c, fmt.Sprintf(
		"document.querySelector(%q).value", selector))
	if err != nil {
		t.Fatalf("read the value back: %v", err)
	}
	if strings.TrimSpace(readBack) != typed {
		t.Fatalf("the box holds %q, typed %q", readBack, typed)
	}
	t.Logf("typed and read back from the live page: %q", readBack)

	if _, err := onOwnTab(ctx, c, fmt.Sprintf(
		"(function(){var e=document.querySelector(%q);e.value='';return 'cleared';})()", selector)); err != nil {
		t.Logf("could not clear the box: %v", err)
	}
}

func attrs(n *html.Node) string {
	var b strings.Builder
	b.WriteString("<" + n.Data)
	for _, a := range n.Attr {
		if a.Key == "id" || a.Key == "name" || a.Key == "type" || a.Key == "class" || a.Key == "aria-label" {
			fmt.Fprintf(&b, " %s=%q", a.Key, a.Val)
		}
	}
	b.WriteString(">")
	return b.String()
}
