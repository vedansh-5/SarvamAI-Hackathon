package uiact

import (
	"fmt"
	"strings"
)

var actedOn = map[string]bool{
	"button": true, "link": true, "textfield": true, "textarea": true, "searchfield": true,
	"checkbox": true, "radiobutton": true, "popupbutton": true, "combobox": true,
	"menuitem": true, "menubutton": true, "row": true, "cell": true, "tab": true,
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
		if !actedOn[role(el.Role)] {
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
