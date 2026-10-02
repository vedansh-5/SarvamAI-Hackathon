package uiact

import (
	"fmt"
	"strings"
)

var actedOn = map[string]bool{
	"button": true, "link": true, "textfield": true, "textarea": true, "searchfield": true,
	"checkbox": true, "radiobutton": true, "popupbutton": true, "combobox": true,
	"menuitem": true, "menubutton": true, "menubaritem": true, "row": true, "cell": true, "tab": true,
	"slider": true, "disclosuretriangle": true, "incrementor": true, "securetextfield": true,
}

const (
	Ready  = "ready"
	Partly = "partly"
	Blind  = "blind"
)

type Assessment struct {
	App      string
	Elements int
	Controls int
	Named    int
	Typeable int
	Verdict  string
	Why      string
}

func Assess(t Tree) Assessment {
	a := Assessment{App: t.App, Elements: len(t.Elements)}
	for _, el := range t.Elements {
		if !actedOn[role(el.Role)] || InMenuBar(el) {
			continue
		}
		a.Controls++
		if el.Label() != "" {
			a.Named++
		}
		if el.Typeable() {
			a.Typeable++
		}
	}

	switch {
	case a.Controls < 5:
		a.Verdict = Blind
		a.Why = fmt.Sprintf("only %d control(s) can be read, so there is almost nothing to point at or act on", a.Controls)
	case a.Named*100 < a.Controls*35:
		a.Verdict = Blind
		a.Why = fmt.Sprintf("%d of %d controls have a name, too few to tell them apart", a.Named, a.Controls)
	case a.Named*100 < a.Controls*70:
		a.Verdict = Partly
		a.Why = fmt.Sprintf("%d of %d controls have a name; the rest cannot be addressed", a.Named, a.Controls)
	default:
		a.Verdict = Ready
		a.Why = fmt.Sprintf("%d of %d controls have a name", a.Named, a.Controls)
	}
	return a
}

const longestBorrowedName = 60

func NameByContents(elements []Element) {
	for i := range elements {
		el := &elements[i]
		if !actedOn[role(el.Role)] || el.Label() != "" {
			continue
		}
		for j := i + 1; j < len(elements); j++ {
			inside := elements[j]
			if !encloses(el.Path, inside.Path) {
				break
			}
			if actedOn[role(inside.Role)] {
				continue
			}
			name := strings.TrimSpace(inside.Title)
			if name == "" && role(inside.Role) == "statictext" {
				name = strings.TrimSpace(inside.Value)
			}
			if name == "" || strings.Contains(name, "\n") || len([]rune(name)) > longestBorrowedName {
				continue
			}
			el.Title = name
			break
		}
	}
}

func IsControl(el Element) bool { return actedOn[role(el.Role)] }

func NewControls(before, after Tree) []Element {
	had := map[string]bool{}
	for _, el := range before.Elements {
		had[role(el.Role)+"\x00"+strings.ToLower(el.Label())] = true
	}
	seen := map[string]bool{}
	var out []Element
	for _, el := range after.Elements {
		label := el.Label()
		if !actedOn[role(el.Role)] || label == "" {
			continue
		}
		key := role(el.Role) + "\x00" + strings.ToLower(label)
		if had[key] || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, el)
	}
	return out
}

const fewestInAList = 3

func (t Tree) Lists() []string {
	type held struct {
		name  string
		depth int
	}
	holders := make([]held, len(t.Elements))
	counts := map[string]int{}
	key := func(i int) string {
		return role(t.Elements[i].Role) + "\x00" + holders[i].name + "\x00" + fmt.Sprint(holders[i].depth)
	}

	for i, el := range t.Elements {
		holders[i] = held{depth: -1}
		if !actedOn[role(el.Role)] {
			continue
		}
		for _, a := range t.Elements {
			if !encloses(a.Path, el.Path) || actedOn[role(a.Role)] {
				continue
			}
			if label := strings.TrimSpace(a.Label()); label != "" && len(a.Path) > holders[i].depth {
				holders[i] = held{name: label, depth: len(a.Path)}
			}
		}
		if holders[i].name != "" {
			counts[key(i)]++
		}
	}

	out := make([]string, len(t.Elements))
	for i := range t.Elements {
		if holders[i].name != "" && counts[key(i)] >= fewestInAList {
			out[i] = holders[i].name
		}
	}
	return out
}

func (t Tree) ListAround(el Element) string {
	lists := t.Lists()
	for i, other := range t.Elements {
		if samePath(other.Path, el.Path) && role(other.Role) == role(el.Role) {
			return lists[i]
		}
	}
	return ""
}

func menuIsOpen(rows []Element) bool {
	if len(rows) == 0 {
		return false
	}
	distinct := map[int]bool{}
	for _, r := range rows {
		if len(r.Position) == 2 {
			distinct[r.Position[1]] = true
		}
	}
	return len(distinct) >= 2 || len(rows) == 1
}

func InMenuBar(el Element) bool { return len(el.Path) > 0 && el.Path[0] < 0 }

func Slim(elements []Element, most int) []Element {
	if len(elements) <= MaxElements {
		return elements
	}
	seen := func(el Element) bool {
		return len(el.Size) == 2 && el.Size[0] > 1 && el.Size[1] > 1
	}
	var kept []Element
	for _, el := range elements {
		r := role(el.Role)
		switch {
		case InMenuBar(el):
		case actedOn[r]:
			if !seen(el) {
				continue
			}
		case r == "webarea" || r == "window" || r == "scrollarea":
		case (r == "group" || r == "list" || r == "table" || r == "outline" || r == "toolbar" || r == "tabgroup" || r == "heading") && strings.TrimSpace(el.Label()) != "":
			if !seen(el) {
				continue
			}
		default:
			continue
		}
		kept = append(kept, el)
	}

	screen, known := displays()
	onScreen := func(el Element) bool {
		if !known || InMenuBar(el) {
			return true
		}
		b, ok := el.box()
		return ok && b.clip(screen).valid()
	}
	var menus, here, away []Element
	for _, el := range kept {
		switch {
		case InMenuBar(el):
			menus = append(menus, el)
		case onScreen(el):
			here = append(here, el)
		default:
			away = append(away, el)
		}
	}
	kept = append(append(menus, here...), away...)
	if len(kept) > most {
		kept = kept[:most]
	}
	return kept
}

func MenusFirst(menus, window []Element) []Element {
	var open, bar []Element
	for _, el := range menus {
		if role(el.Role) == "menuitem" {
			open = append(open, el)
		} else {
			bar = append(bar, el)
		}
	}
	out := make([]Element, 0, len(menus)+len(window))
	out = append(out, open...)
	out = append(out, bar...)
	return append(out, window...)
}
