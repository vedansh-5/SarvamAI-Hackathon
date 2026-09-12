package webact

import (
	"fmt"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

const MaxControls = 120

const MaxControlsBytes = 12000

const MaxSelectorLength = 120

type Control struct {
	Selector string
	Kind     string
	Label    string
}

var interactive = cascadia.MustCompile(
	"input, textarea, select, button, a[href], [role=button], [contenteditable]")

func Controls(doc *html.Node) []Control {
	var out []Control
	seen := map[string]bool{}

	for _, n := range cascadia.QueryAll(doc, interactive) {
		if len(out) >= MaxControls {
			break
		}
		if !offerable(n) {
			continue
		}
		sel := selectorFor(doc, n)
		if sel == "" || seen[sel] {
			continue
		}
		seen[sel] = true

		out = append(out, Control{
			Selector: sel,
			Kind:     kindOf(n),
			Label:    labelOf(n),
		})
	}
	return out
}

func ControlsText(cs []Control, budget int) string {
	var b strings.Builder
	for _, c := range cs {
		line := "  " + c.Selector + "  (" + c.Kind + ")"
		if c.Label != "" {
			line += "  " + c.Label
		}
		line += "\n"
		if b.Len()+len(line) > budget {
			b.WriteString("  … more controls than fit here\n")
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func kindOf(n *html.Node) string {
	switch n.Data {
	case "input":
		if t := attr(n, "type"); t != "" {
			return "input " + t
		}
		return "input text"
	case "a":
		return "link"
	case "textarea", "select", "button":
		return n.Data
	}
	if attr(n, "contenteditable") != "" && attr(n, "contenteditable") != "false" {
		return "editable"
	}
	return "button"
}

// labelOf takes what the interface calls a control, never what it holds. The
// value of an input is the user's text, and it is not what choosing a control
// requires.
func labelOf(n *html.Node) string {
	for _, key := range []string{"aria-label", "placeholder", "title", "name"} {
		if v := strings.TrimSpace(attr(n, key)); v != "" {
			return truncate(v, 60)
		}
	}
	if n.Data == "button" || n.Data == "a" {
		if t := strings.TrimSpace(textOf(n)); t != "" {
			return truncate(t, 60)
		}
	}
	return ""
}

func selectorFor(doc *html.Node, n *html.Node) string {
	if id := strings.TrimSpace(attr(n, "id")); id != "" && safeValue(id) {
		if sel := "#" + id; matchesOne(doc, sel) {
			return sel
		}
	}
	for _, key := range []string{"name", "aria-label", "placeholder"} {
		v := strings.TrimSpace(attr(n, key))
		if v == "" || !safeValue(v) {
			continue
		}
		sel := fmt.Sprintf("%s[%s=%q]", n.Data, key, v)
		if matchesOne(doc, sel) {
			return sel
		}
	}
	if classes := strings.Fields(attr(n, "class")); len(classes) > 0 {
		var parts []string
		for _, c := range classes {
			if len(parts) == 3 || !safeValue(c) || generatedClass(c) {
				continue
			}
			parts = append(parts, "."+c)
		}
		if len(parts) > 0 {
			sel := n.Data + strings.Join(parts, "")
			if len(sel) <= MaxSelectorLength && matchesOne(doc, sel) {
				return sel
			}
		}
	}
	return ""
}

func matchesOne(doc *html.Node, selector string) bool {
	sel, err := cascadia.Compile(selector)
	if err != nil {
		return false
	}
	return len(cascadia.QueryAll(doc, sel)) == 1
}

func safeValue(v string) bool {
	if v == "" || len(v) > 80 {
		return false
	}
	return !strings.ContainsAny(v, "\"'\\<>\n\t")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(c *html.Node) {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
			return
		}
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			walk(k)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func offerable(n *html.Node) bool {
	if strings.EqualFold(attr(n, "type"), "hidden") {
		return false
	}
	if strings.EqualFold(attr(n, "aria-hidden"), "true") {
		return false
	}
	if _, disabled := lookup(n, "disabled"); disabled {
		return false
	}
	return true
}

func lookup(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func generatedClass(c string) bool {
	if len(c) > 30 || strings.Contains(c, "__") {
		return true
	}
	digits := 0
	for _, r := range c {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits >= 4
}
