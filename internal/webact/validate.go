package webact

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html"
)

// MaxSteps bounds a program.
//
// A page is not a scripting language and this is not a macro recorder. Long
// sequences are where a plan stops resembling the sentence that produced it,
// and where a preview stops being readable — which is the same thing as an
// unconfirmed side effect.
const MaxSteps = 8

// InvalidError is a program that will not run, with every reason at once.
type InvalidError struct{ Reasons []string }

func (e *InvalidError) Error() string {
	return "webact: this program will not run against the page: " + strings.Join(e.Reasons, "; ")
}

// typeable are the elements text can be typed into. Typing into anything else
// silently does nothing, which is worse than refusing: the plan is confirmed,
// the click lands, and the message is empty.
var typeable = map[string]bool{"input": true, "textarea": true}

// Validate checks a program against the page it was written for.
//
// The page is the authority, the way an application's dictionary is in the
// tier above. A selector that matches nothing is refused here rather than
// discovered after the user has confirmed a plan that said it would work.
func Validate(p Program, doc *html.Node) error {
	var reasons []string
	add := func(format string, args ...any) {
		reasons = append(reasons, fmt.Sprintf(format, args...))
	}

	if len(p.Steps) == 0 {
		add("it has no steps")
	}
	if len(p.Steps) > MaxSteps {
		add("it has %d steps, and %d is the most a single request may take", len(p.Steps), MaxSteps)
	}

	for i, s := range p.Steps {
		where := fmt.Sprintf("step %d", i+1)

		if !Verbs[verb(s.Do)] {
			add("%s: %q is not something VANS does to a page", where, s.Do)
			continue
		}

		if strings.TrimSpace(s.Selector) == "" {
			add("%s: it names no element", where)
			continue
		}

		sel, err := cascadia.Compile(s.Selector)
		if err != nil {
			add("%s: %q is not a usable selector: %v", where, s.Selector, err)
			continue
		}

		matches := cascadia.QueryAll(doc, sel)
		switch {
		case len(matches) == 0:
			add("%s: nothing on the page matches %q", where, s.Selector)
			continue
		case len(matches) > 1:
			// Ambiguity is the failure a confirmation cannot catch: the plan
			// says "click Send" and there are four of them, so the user
			// confirms one thing and a different one happens.
			add("%s: %d elements match %q, so it does not say which one",
				where, len(matches), s.Selector)
			continue
		}

		if verb(s.Do) == "type" {
			node := matches[0]
			if !typeable[node.Data] && !isEditable(node) {
				add("%s: %q is a <%s>, which text cannot be typed into",
					where, s.Selector, node.Data)
			}
			if strings.TrimSpace(s.Text) == "" {
				add("%s: it types nothing", where)
			}
		}

		if verb(s.Do) == "press" && !Keys[s.Key] {
			add("%s: %q is not a key VANS presses", where, s.Key)
		}
	}

	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return &InvalidError{Reasons: reasons}
}

func isEditable(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key == "contenteditable" && a.Val != "false" {
			return true
		}
	}
	return false
}

// Baked reports values written into the program instead of referenced.
//
// The same floor as D-039 and D-042: the program is cached by shape and
// replayed, so a message typed into it is the message every later request of
// that shape sends.
func Baked(p Program, literals map[string]string) []string {
	var out []string
	seen := map[string]bool{}

	for _, value := range p.Values() {
		for _, literal := range literals {
			literal = strings.TrimSpace(literal)
			if len(literal) < 3 || seen[literal] {
				continue
			}
			if strings.Contains(strings.ToLower(value), strings.ToLower(literal)) {
				seen[literal] = true
				out = append(out, literal)
			}
		}
	}
	sort.Strings(out)
	return out
}
