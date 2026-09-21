package uiact

import (
	"fmt"
	"sort"
	"strings"
)

const MaxChanges = 8

func identity(e Element) string {
	return role(e.Role) + "\x00" + strings.ToLower(strings.TrimSpace(e.Title))
}

func Changes(before, after Tree) []string {
	was := map[string]Element{}
	for _, el := range before.Elements {
		was[identity(el)] = el
	}
	now := map[string]Element{}
	for _, el := range after.Elements {
		now[identity(el)] = el
	}

	var out []string
	if before.Window != "" && after.Window != "" && before.Window != after.Window {
		out = append(out, fmt.Sprintf("window title changed to %q", after.Window))
	}
	for k, el := range now {
		prev, had := was[k]
		if !had {
			out = append(out, fmt.Sprintf("a %s %q appeared", el.Role, el.Title))
			continue
		}
		if prev.Value != el.Value {
			out = append(out, fmt.Sprintf("the %s %q now holds %q",
				el.Role, el.Title, truncate(el.Value, 60)))
		}
		if !prev.Focused && el.Focused {
			out = append(out, fmt.Sprintf("the %s %q is now focused", el.Role, el.Title))
		}
	}
	for k, el := range was {
		if _, still := now[k]; !still {
			out = append(out, fmt.Sprintf("the %s %q is gone", el.Role, el.Title))
		}
	}

	sort.Strings(out)
	if len(out) > MaxChanges {
		out = append(out[:MaxChanges], fmt.Sprintf("… and %d more", len(out)-MaxChanges))
	}
	return out
}

func Landed(before, after Tree, p Program) bool {
	typed := false
	for _, s := range p.Steps {
		if verb(s.Do) != "type" || s.Text == "" {
			continue
		}
		typed = true
		if !holds(after, s.Text) {
			return false
		}
	}
	if typed {
		return true
	}
	return len(Changes(before, after)) > 0
}

func holds(t Tree, text string) bool {
	for _, el := range t.Elements {
		if strings.Contains(el.Value, text) {
			return true
		}
	}
	return false
}

// How much has to have come or gone before a window counts as having moved on.
// One control changing on its own is a clock ticking or a list refreshing itself;
// a pane that answered a click replaces most of what was in it.
const minimumUpheaval = 2

// ActedOn reports whether the window changed in a way that means the person used
// the control the coach pointed at.
//
// Stricter than Changes, and for a different question. Changes describes what
// happened, for the step writer to read; this decides whether to move on, and a
// battery percentage or a refreshing list must not be enough to do that — the
// coach would skip a step nobody had done.
func ActedOn(before, after Tree, step Step) bool {
	// Typing is the one step whose completion is not a guess. Half-typed text
	// changes the window on the first keystroke, and the coach moved on from
	// "type Wallpaper" the moment the W appeared — then asked for the rest as a
	// step of its own.
	if verb(step.Do) == "type" && strings.TrimSpace(step.Text) != "" {
		return holds(after, step.Text)
	}
	return windowMovedOn(before, after, step.Target)
}

func windowMovedOn(before, after Tree, target Selector) bool {
	if before.Window != "" && after.Window != "" && before.Window != after.Window {
		return true
	}

	now := map[string]Element{}
	for _, el := range after.Elements {
		now[identity(el)] = el
	}

	if el, why := before.resolve(target); why == "" {
		current, still := now[identity(el)]
		if !still {
			return true
		}
		if !el.Focused && current.Focused {
			return true
		}
		if el.Value != current.Value {
			return true
		}
	}

	return upheaval(before, after) >= minimumUpheaval
}

// How many controls came or went, which is what a click that opens something
// looks like from the outside.
func upheaval(before, after Tree) int {
	was := map[string]bool{}
	for _, el := range before.Elements {
		was[identity(el)] = true
	}
	seen := map[string]bool{}
	count := 0
	for _, el := range after.Elements {
		id := identity(el)
		seen[id] = true
		if !was[id] {
			count++
		}
	}
	for id := range was {
		if !seen[id] {
			count++
		}
	}
	return count
}
