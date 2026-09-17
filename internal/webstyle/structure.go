package webstyle

import (
	"fmt"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

const (
	MaxParts          = 60
	MaxStructureBytes = 6000
)

// Part is one kind of element on the page, and how to address it.
type Part struct {
	Selector string
	Count    int
}

var skipped = map[string]bool{
	"script": true, "style": true, "meta": true, "link": true,
	"head": true, "title": true, "noscript": true, "br": true, "svg": true, "path": true,
}

// Structure summarises what a page is made of, as selectors that address it.
//
// Counts travel and contents do not. A restyle decides which elements to
// change, not what they say, so the shape of the page is the whole of what the
// model needs — and the page in front of someone is as likely to be their mail
// or a conversation as it is a shop. This is D-058 applied to a second tier:
// the structure is the request, the text is not.
func Structure(doc *html.Node) []Part {
	counts := map[string]int{}

	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && !skipped[n.Data] {
			for _, sel := range selectorsFor(n) {
				counts[sel]++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	out := make([]Part, 0, len(counts))
	for sel, n := range counts {
		out = append(out, Part{Selector: sel, Count: n})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Selector < out[j].Selector
	})
	if len(out) > MaxParts {
		out = out[:MaxParts]
	}
	return out
}

func selectorsFor(n *html.Node) []string {
	var out []string
	for _, a := range n.Attr {
		if a.Key == "id" && strings.TrimSpace(a.Val) != "" && !generated(a.Val) {
			out = append(out, "#"+strings.TrimSpace(a.Val))
		}
	}
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, c := range strings.Fields(a.Val) {
			if generated(c) {
				continue
			}
			out = append(out, n.Data+"."+c)
		}
	}
	if len(out) == 0 {
		out = append(out, n.Data)
	}
	return out
}

// A class made of a hash changes on the next deploy, so a program written
// against it is a program that works once.
func generated(s string) bool {
	if len(s) < 6 {
		return false
	}
	digits := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits++
		}
	}
	return digits*3 >= len(s)
}

// StructureText renders the summary for the model.
func StructureText(parts []Part, budget int) string {
	var b strings.Builder
	for _, p := range parts {
		line := fmt.Sprintf("  %s  (%d)\n", p.Selector, p.Count)
		if b.Len()+len(line) > budget {
			b.WriteString("  … more of the page than fits here\n")
			break
		}
		b.WriteString(line)
	}
	return b.String()
}
