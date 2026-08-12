// Package extract turns a page's DOM into candidate data regions.
//
// This is deliberately the deterministic half of extraction. Finding repeated
// structure is an algorithm, not a judgement call, so it runs with no model
// involved. Its output is what shrinks a 500 KB page down to a handful of
// regions small enough to reason about — which is what makes synthesis cheap
// enough to be worth caching.
//
// The approach follows MDR (Liu, Grossman & Zhai, KDD 2003): data records are
// contiguous sibling subtrees that share a structure.
package extract

import (
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// skipTags never contain data records worth extracting.
var skipTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"svg":      true,
	"head":     true,
	"meta":     true,
	"link":     true,
	"template": true,
	"iframe":   true,
	"path":     true,
}

// generatedClass matches class names that carry a build hash — CSS modules and
// CSS-in-JS emit these, and they change on every deploy.
//
// Dropping them is what lets a fingerprint survive a rebuild. It matters twice:
// here for grouping siblings, and again for the page signature that keys the
// program cache.
var generatedClass = regexp.MustCompile(`(?i)^(.*?)[-_]?[0-9a-f]{5,}$|^[a-z]{1,3}[0-9]{3,}$`)

// Parse reads HTML into a node tree.
func Parse(htmlSrc string) (*html.Node, error) {
	return html.Parse(strings.NewReader(htmlSrc))
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// classes returns the element's stable class names, sorted.
func classes(n *html.Node) []string {
	raw := strings.Fields(attr(n, "class"))
	out := make([]string, 0, len(raw))
	for _, c := range raw {
		if c = normalizeClass(c); c != "" {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// normalizeClass drops class names that look machine-generated, returning ""
// for those. Stable names pass through unchanged.
func normalizeClass(c string) string {
	if c == "" {
		return ""
	}
	if m := generatedClass.FindStringSubmatch(c); m != nil {
		// Keep the human-readable stem if there was one: "card-3f9a2" -> "card".
		if len(m) > 1 && len(m[1]) >= 2 {
			return m[1]
		}
		return ""
	}
	return c
}

// fingerprint identifies an element's structural shape. Two siblings with the
// same fingerprint are candidates for being records of the same kind.
//
// It combines the tag, stable classes, and the tag sequence of the element's
// own children — one level of shape is enough to separate a list of product
// cards from the stray divs sitting beside them.
func fingerprint(n *html.Node) string {
	if n.Type != html.ElementNode {
		return ""
	}

	var b strings.Builder
	b.WriteString(n.Data)

	for _, c := range classes(n) {
		b.WriteByte('.')
		b.WriteString(c)
	}

	b.WriteByte('[')
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !skipTags[c.Data] {
			b.WriteString(c.Data)
			b.WriteByte(',')
		}
	}
	b.WriteByte(']')

	return b.String()
}

// text returns the element's visible text, collapsed.
func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
			return
		}
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// selectorFor builds a CSS selector describing the element's shape: its tag
// plus stable classes. Not guaranteed unique on its own — see cssPath.
func selectorFor(n *html.Node) string {
	if id := attr(n, "id"); id != "" && normalizeClass(id) != "" {
		return n.Data + "#" + id
	}
	s := n.Data
	for _, c := range classes(n) {
		s += "." + c
	}
	return s
}

// cssPath builds a selector from the nearest identifiable ancestor down to n.
// The walk stops at an element carrying an id, since that anchors the path.
func cssPath(n *html.Node) string {
	var parts []string
	for cur := n; cur != nil && cur.Type == html.ElementNode; cur = cur.Parent {
		if cur.Data == "html" || cur.Data == "body" {
			break
		}
		part := selectorFor(cur)
		parts = append([]string{part}, parts...)
		if strings.Contains(part, "#") {
			break
		}
	}
	if len(parts) == 0 {
		return selectorFor(n)
	}
	return strings.Join(parts, " > ")
}

// skeleton renders a compact structural outline of a record, keeping only the
// attributes a selector could use and truncating text.
//
// This is what the synthesizer sees. It has to convey shape precisely while
// staying small enough that a handful of records fit in a prompt.
func skeleton(n *html.Node, maxDepth int) string {
	var b strings.Builder
	var walk func(*html.Node, int)

	walk = func(n *html.Node, depth int) {
		if depth > maxDepth {
			return
		}
		switch n.Type {
		case html.TextNode:
			t := strings.Join(strings.Fields(n.Data), " ")
			if t == "" {
				return
			}
			if len(t) > 40 {
				t = t[:40] + "…"
			}
			b.WriteString(t)
		case html.ElementNode:
			if skipTags[n.Data] {
				return
			}
			b.WriteByte('<')
			b.WriteString(n.Data)
			for _, key := range []string{"id", "class", "href", "src"} {
				if v := attr(n, key); v != "" {
					if len(v) > 60 {
						v = v[:60] + "…"
					}
					b.WriteString(" " + key + `="` + v + `"`)
				}
			}
			b.WriteByte('>')
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, depth+1)
			}
			b.WriteString("</" + n.Data + ">")
		}
	}

	walk(n, 0)
	return b.String()
}
