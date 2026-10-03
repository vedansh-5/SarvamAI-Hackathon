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
	if typed, decided := typingLanded(after, step); decided {
		return typed
	}
	return windowMovedOn(before, after, step.Target)
}

// PersonActedOn is the coach's test, and it is deliberately stricter than the
// drive loop's.
//
// The difference is who acted. The drive loop performed the step itself, so any
// change in the window is evidence it landed. The coach is watching somebody
// else, and it already has a direct observation of them — the shell reports a
// click landing inside the ring. Counting controls that came or went is a second
// answer to a question that is already answered, and it wins wrongly whenever an
// application is still drawing: System Settings' Wallpaper pane loads thumbnails
// for seconds, and the coach walked from step 1 to step 3 with nobody touching
// the machine (D-109).
//
// So: evidence about the control itself, never the general churn of the window.
// A key press is the exception — there is no click to observe, and pressing a key
// need not touch the control at all — so it keeps the older, looser test.
const typingSeenAround = 3

func typedWhereItCannotBeRead(before, after Tree, step Step) bool {
	el, why := after.resolve(step.Target)
	if why != "" || !el.Blind {
		return false
	}
	return upheaval(before, after) >= typingSeenAround
}

func PersonActedOn(before, after Tree, step Step) bool {
	if typed, decided := typingLanded(after, step); decided {
		return typed || typedWhereItCannotBeRead(before, after, step)
	}
	if verb(step.Do) == "press" {
		return windowMovedOn(before, after, step.Target)
	}
	return targetMovedOn(before, after, step.Target)
}

// Typing is the one step whose completion is not a guess. Half-typed text changes
// the window on the first keystroke, and the coach moved on from "type Wallpaper"
// the moment the W appeared — then asked for the rest as a step of its own.
func typingLanded(after Tree, step Step) (landed, decided bool) {
	if verb(step.Do) != "type" || strings.TrimSpace(step.Text) == "" {
		return false, false
	}
	return holds(after, step.Text), true
}

func windowMovedOn(before, after Tree, target Selector) bool {
	if targetMovedOn(before, after, target) {
		return true
	}
	return upheaval(before, after) >= minimumUpheaval
}

// What the control itself did: it went, it took focus, or what it holds changed.
// A window retitling counts too — nothing renames a window by itself, and it is
// how navigation shows up in applications that publish a title at all.
func targetMovedOn(before, after Tree, target Selector) bool {
	if before.Window != "" && after.Window != "" && before.Window != after.Window {
		return true
	}

	el, why := before.resolve(target)
	if why != "" {
		return false
	}

	now := map[string]Element{}
	for _, e := range after.Elements {
		now[identity(e)] = e
	}

	current, still := now[identity(el)]
	switch {
	case !still:
		return true
	case !el.Focused && current.Focused:
		return true
	case el.Value != current.Value:
		return true
	default:
		return false
	}
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
