package uiact

import (
	"fmt"
	"github.com/vedansh/vans/internal/names"
	"sort"
	"strings"
)

// Repair corrects a step the window can satisfy under a different name for the
// same control, and says what it changed.
//
// The case it exists for, seen live twice: the model asks for the AXButton
// "Wallpaper" and System Settings has an AXRow by that name. The refusal already
// names the right role, because nearMiss works it out in order to word the
// message — and then the step was re-asked, the model said AXButton again, and the
// run stopped. Two model calls to arrive nowhere, with the answer in hand the
// whole time.
//
// Only where there is nothing to decide. The name has to match exactly one
// control on screen, and only its role may be wrong. Several matches is a
// question about which one was meant, and guessing that is how something gets
// clicked that nobody asked for — that goes back to the model, with the options
// named (see nearMiss).
//
// A repaired step is not trusted any further than an asked-for one. It goes
// through Validate, through VerifyTargets against the live window, and through
// the read-back afterwards, exactly as it would have anyway.
func Repair(step Step, t Tree) (Step, string, bool) {
	if _, why := t.resolve(step.Target); why == "" {
		return step, "", false
	}

	title := strings.TrimSpace(step.Target.Title)
	if title == "" {
		return step, "", false
	}

	// Right name, right role, and a number saying which of several — when there is
	// only one of that kind. The model reads "(index 14 of 30)" off a list of rows
	// and carries the 14 over to a control that stands alone (seen live: "Displays
	// number 14, and there are 1"). Counted by role as well as name, because the
	// same word is usually on screen twice: the sidebar row, and the heading of
	// the page it opens.
	if step.Target.Index > 0 {
		sameKind := t.Find(Selector{Role: step.Target.Role, Title: title})
		if len(sameKind) == 1 {
			n := step.Target.Index
			step.Target.Index = 0
			return step, fmt.Sprintf("there is only one %s %q here, not a number %d",
				step.Target.Role, title, n), true
		}
	}

	var found []Element
	for _, el := range t.Elements {
		if strings.EqualFold(strings.TrimSpace(el.Label()), title) {
			found = append(found, el)
		}
	}
	if len(found) != 1 {
		return looselyNamed(step, t)
	}

	was := step.Target.Role
	if strings.EqualFold(role(was), role(found[0].Role)) {
		// The role was right and something else was wrong — disabled, not
		// pressable, off screen. None of those is a naming mistake to correct.
		return step, "", false
	}

	step.Target.Role = found[0].Role
	step.Target.Index = 0
	return step, fmt.Sprintf("%q is a %s here, not a %s", title, found[0].Role, was), true
}

// What was there under that name, when the kind was wrong.
//
// A rejected step is re-asked with the reason in hand, and "nothing on screen is
// a AXButton named Wallpaper" gives the model nothing to correct: the row is
// right there, as an AXRow. Naming what was found turns a wasted turn into a
// step that lands. Sidebar items are the common case — the same word is a
// button, a row or a static text depending on the application.
//
// The counts and the instruction are here because Repair takes the unambiguous
// case away. What reaches this message now is a name that matches several
// controls, so the model needs to know how many of each there are in order to
// pick one and say which — and a reason that describes the problem without saying
// what to write instead was answered identically twice, live.
func (t Tree) nearMiss(sel Selector) string {
	title := strings.ToLower(strings.TrimSpace(sel.Title))
	if title == "" {
		return ""
	}

	counts := map[string]int{}
	for _, el := range t.Elements {
		if strings.ToLower(el.Label()) == title {
			counts[el.Role]++
		}
	}
	if len(counts) == 0 {
		return ""
	}

	roles := make([]string, 0, len(counts))
	for r := range counts {
		roles = append(roles, r)
	}
	sort.Strings(roles)

	parts := make([]string, 0, len(roles))
	total := 0
	for _, r := range roles {
		total += counts[r]
		if counts[r] == 1 {
			parts = append(parts, r)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%d of them)", r, counts[r]))
	}

	out := fmt.Sprintf(" — that name is on screen as %s", strings.Join(parts, " and "))
	if total > 1 {
		return out + "; name one of those roles, and an index to say which one"
	}
	return out + "; name that role instead"
}

const shortestLooseName = 3

func looselyNamed(step Step, t Tree) (Step, string, bool) {
	want := names.Plain(step.Target.Title)
	if len(want) < shortestLooseName {
		return step, "", false
	}

	fits := func(el Element, wholeName bool) bool {
		have := names.Plain(el.Label())
		if have == "" {
			return false
		}
		if wholeName {
			return have == want
		}
		return strings.HasPrefix(have, want+" ")
	}
	pick := func(sameRole, wholeName bool) []Element {
		var out []Element
		for _, el := range t.Elements {
			if sameRole && role(el.Role) != role(step.Target.Role) {
				continue
			}
			if fits(el, wholeName) {
				out = append(out, el)
			}
		}
		return out
	}

	for _, attempt := range []struct{ sameRole, wholeName bool }{
		{true, true}, {false, true}, {true, false}, {false, false},
	} {
		found := pick(attempt.sameRole, attempt.wholeName)
		if len(found) == 0 {
			continue
		}
		if len(found) > 1 {
			return step, "", false
		}
		asked := step.Target.Title
		step.Target.Role = found[0].Role
		step.Target.Title = found[0].Label()
		step.Target.Index = 0
		return step, fmt.Sprintf("%q is called %q here", asked, found[0].Label()), true
	}
	return step, "", false
}
