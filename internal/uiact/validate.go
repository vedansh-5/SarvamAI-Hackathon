package uiact

import (
	"fmt"
	"sort"
	"strings"
)

type InvalidError struct {
	App     string
	Reasons []string
}

func (e *InvalidError) Error() string {
	return fmt.Sprintf("uiact: this program will not run against %s: %s",
		e.App, strings.Join(e.Reasons, "; "))
}

func (t Tree) resolve(sel Selector) (Element, string) {
	if strings.TrimSpace(sel.Role) == "" {
		return Element{}, "it names no kind of element"
	}

	matches := t.Find(sel)
	named := ""
	if sel.Title != "" {
		named = fmt.Sprintf(" named %q", sel.Title)
	}

	switch {
	case len(matches) == 0:
		return Element{}, fmt.Sprintf("nothing on screen is a %s%s", sel.Role, named)
	case sel.Index > 0:
		if sel.Index > len(matches) {
			return Element{}, fmt.Sprintf("it asks for %s%s number %d and there are %d",
				sel.Role, named, sel.Index, len(matches))
		}
		return matches[sel.Index-1], ""
	case len(matches) > 1:
		return Element{}, fmt.Sprintf("%d elements are a %s%s, so it does not say which one",
			len(matches), sel.Role, named)
	default:
		return matches[0], ""
	}
}

func Validate(p Program, t Tree) error {
	var reasons []string
	add := func(format string, args ...any) {
		reasons = append(reasons, fmt.Sprintf(format, args...))
	}

	if p.App != "" && t.App != "" && !strings.EqualFold(p.App, t.App) {
		add("it addresses %q but was written for %q", p.App, t.App)
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
			add("%s: %q is not something VANS does to a window", where, s.Do)
			continue
		}

		el, why := t.resolve(s.Target)
		if why != "" {
			add("%s: %s", where, why)
			continue
		}

		if !el.Enabled {
			add("%s: the %s %q is disabled", where, el.Role, el.Title)
			continue
		}

		switch verb(s.Do) {
		case "type":
			if !el.Typeable() {
				add("%s: a %s cannot hold typed text", where, el.Role)
			}
			if strings.TrimSpace(s.Text) == "" {
				add("%s: it types nothing", where)
			}
		case "click":
			if !el.Pressable() {
				add("%s: the %s %q cannot be pressed", where, el.Role, el.Title)
			}
		case "press":
			if _, ok := Keys[s.Key]; !ok {
				add("%s: %q is not a key VANS presses", where, s.Key)
			}
			for _, name := range s.Modifiers {
				if _, ok := Modifiers[name]; !ok {
					add("%s: %q is not a modifier VANS holds down", where, name)
				}
			}
		}
	}

	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return &InvalidError{App: t.App, Reasons: reasons}
}

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
