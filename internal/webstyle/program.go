// Package webstyle restyles a page the user is looking at.
//
// The program is data, and the data is deliberately thin: a rule is a selector
// and an emphasis. The model chooses what to point at and how strongly; the
// runtime owns every colour, radius and shadow that reaches the page.
//
// That split is what keeps a restyle looking like part of the page rather than
// like a debugging tool, and it removes a whole class of question. There is no
// value from the model to validate, escape, or ban url() from, because there is
// no value from the model at all.
package webstyle

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

const (
	MaxRules            = 20
	MaxSelectorLength   = 200
	StyleElementID      = "vans-style"
	MinMatchingElements = 1
)

// Rule is one selector and how strongly to mark what it matches.
type Rule struct {
	Selector string `json:"selector"`
	Emphasis string `json:"emphasis"`
}

// Program is a whole restyle.
type Program struct {
	Rules []Rule `json:"rules"`
}

// Emphases are the ways a restyle may mark something.
//
// Each is a tint and nothing else — no border, no outline, no ring. A mark
// around a thing is a debugging tool; a mark over it is a highlighter, and the
// second is what someone means by "show me which ones". Nothing here changes an
// element's size, so the page does not move under the reader.
//
// The colours are tokens, so the same rule wears a different palette on a light
// page and a dark one.
var Emphases = map[string][]string{
	"highlight": {
		"background-color: var(--vans-tint)",
		"border-radius: 4px",
	},
	"attention": {
		"background-color: var(--vans-rose)",
		"border-radius: 4px",
	},
	"accent": {
		"background-color: var(--vans-blue)",
		"border-radius: 4px",
	},
	"mark": {
		"box-shadow: inset 3px 0 0 0 var(--vans-ring)",
		"border-radius: 3px",
	},
	"muted": {
		"opacity: 0.4",
		"filter: saturate(0.55)",
	},
}

// Marked elements ease in, so a restyle reads as the page answering rather
// than as the page having been swapped.
var eases = []string{
	"transition: background-color .25s ease, opacity .25s ease, filter .25s ease",
}

func EmphasisNames() []string {
	out := make([]string, 0, len(Emphases))
	for k := range Emphases {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// InvalidError is a restyle refused before it reached the page.
type InvalidError struct{ Reasons []string }

func (e *InvalidError) Error() string {
	return "webstyle: this restyle will not run: " + strings.Join(e.Reasons, "; ")
}

// Validate checks a program against the page it will run on.
//
// A selector that matches nothing is the failure worth catching: the restyle
// reports success, the page looks identical, and the user is told that
// something happened which did not.
func Validate(p Program, doc *html.Node) error {
	var reasons []string

	if len(p.Rules) == 0 {
		reasons = append(reasons, "it changes nothing")
	}
	if len(p.Rules) > MaxRules {
		reasons = append(reasons, fmt.Sprintf(
			"it has %d rules, and %d is the most a single restyle may have", len(p.Rules), MaxRules))
	}

	for i, r := range p.Rules {
		sel := strings.TrimSpace(r.Selector)
		if sel == "" {
			reasons = append(reasons, fmt.Sprintf("rule %d has no selector", i+1))
			continue
		}
		if len(sel) > MaxSelectorLength {
			reasons = append(reasons, fmt.Sprintf(
				"rule %d: the selector is longer than %d characters", i+1, MaxSelectorLength))
			continue
		}

		emphasis := strings.ToLower(strings.TrimSpace(r.Emphasis))
		if _, ok := Emphases[emphasis]; !ok {
			reasons = append(reasons, fmt.Sprintf(
				"rule %d: %q is not a way to mark something, which is one of: %s",
				i+1, r.Emphasis, strings.Join(EmphasisNames(), ", ")))
		}

		compiled, err := cascadia.Compile(sel)
		if err != nil {
			reasons = append(reasons, fmt.Sprintf(
				"rule %d: %q is not a selector this page can be asked for", i+1, sel))
			continue
		}
		if doc == nil {
			continue
		}
		found := cascadia.QueryAll(doc, compiled)
		if len(found) < MinMatchingElements {
			reasons = append(reasons, fmt.Sprintf("rule %d: %q matched nothing on the page", i+1, sel))
		} else if emphasis != "muted" && allWhole(found) {
			reasons = append(reasons, fmt.Sprintf(
				"rule %d: %q marks whole rows, lists or sections; mark the cell, name or label that makes the difference instead", i+1, sel))
		}
	}

	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return &InvalidError{Reasons: reasons}
}

var whole = map[string]bool{
	"html": true, "body": true, "main": true, "section": true, "article": true, "form": true,
	"nav": true, "header": true, "footer": true, "aside": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"ul": true, "ol": true, "dl": true,
}

func allWhole(nodes []*html.Node) bool {
	for _, n := range nodes {
		if !whole[n.Data] {
			return false
		}
	}
	return true
}

// Sheet renders the validated program as CSS.
//
// Every declaration comes from the emphasis table. The selector is the only
// thing written as given, and it has been compiled, so it is a selector.
func Sheet(p Program) string {
	var b strings.Builder
	for _, r := range p.Rules {
		emphasis := strings.ToLower(strings.TrimSpace(r.Emphasis))
		decls, ok := Emphases[emphasis]
		if !ok {
			continue
		}
		b.WriteString(strings.TrimSpace(r.Selector))
		b.WriteString(" {")
		for _, d := range append(append([]string{}, decls...), eases...) {
			fmt.Fprintf(&b, " %s !important;", d)
		}
		b.WriteString(" }\n")
	}
	return b.String()
}

// Preview says what the restyle will do, in the page's own terms.
func Preview(p Program) string {
	if len(p.Rules) == 0 {
		return "Change nothing"
	}
	var parts []string
	for _, r := range p.Rules {
		parts = append(parts, fmt.Sprintf("%s %s", strings.ToLower(strings.TrimSpace(r.Emphasis)),
			strings.TrimSpace(r.Selector)))
	}
	return strings.Join(parts, "; ")
}
