package uiact

import (
	"fmt"
	"strings"
)

type Element struct {
	Role     string   `json:"role"`
	Title    string   `json:"title,omitempty"`
	Value    string   `json:"value,omitempty"`
	Enabled  bool     `json:"enabled"`
	Focused  bool     `json:"focused,omitempty"`
	Position []int    `json:"position,omitempty"`
	Size     []int    `json:"size,omitempty"`
	Actions  []string `json:"actions,omitempty"`
	Path     []int    `json:"path"`
}

type Tree struct {
	App      string    `json:"app"`
	Window   string    `json:"window"`
	Elements []Element `json:"elements"`
}

const MaxTreeBytes = 24000

var typeable = map[string]bool{
	"textfield":   true,
	"textarea":    true,
	"combobox":    true,
	"searchfield": true,
}

func role(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "AX")
	return strings.ToLower(s)
}

func (e Element) Typeable() bool { return typeable[role(e.Role)] }

func (e Element) Pressable() bool {
	if len(e.Actions) == 0 {
		return true
	}
	for _, a := range e.Actions {
		if strings.EqualFold(a, "AXPress") {
			return true
		}
	}
	return false
}

func (t Tree) Find(sel Selector) []Element {
	var out []Element
	for _, el := range t.Elements {
		if role(el.Role) != role(sel.Role) {
			continue
		}
		if sel.Title != "" && !strings.EqualFold(strings.TrimSpace(el.Title), strings.TrimSpace(sel.Title)) {
			continue
		}
		out = append(out, el)
	}
	return out
}

func (t Tree) Text() string { return t.TextWithin(MaxTreeBytes) }

func (t Tree) TextWithin(budget int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — window %q\n\n", t.App, t.Window)

	counts := map[string]int{}
	for _, el := range t.Elements {
		key := role(el.Role) + "\x00" + strings.ToLower(el.Title)
		counts[key]++
	}

	seen := map[string]int{}
	for _, el := range t.Elements {
		key := role(el.Role) + "\x00" + strings.ToLower(el.Title)
		seen[key]++

		line := "  " + el.Role
		if el.Title != "" {
			line += fmt.Sprintf(" %q", el.Title)
		}
		if counts[key] > 1 {
			line += fmt.Sprintf(" (index %d of %d)", seen[key], counts[key])
		}
		if el.Value != "" {
			line += fmt.Sprintf(" value=%q", truncate(el.Value, 60))
		}
		if !el.Enabled {
			line += " [disabled]"
		}
		line += "\n"

		if b.Len()+len(line) > budget {
			b.WriteString("  … more elements than fit here\n")
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func samePath(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (t Tree) SelectorFor(el Element) Selector {
	sel := Selector{Role: el.Role, Title: el.Title}

	var seen, total int
	for _, other := range t.Elements {
		if role(other.Role) != role(el.Role) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(other.Title), strings.TrimSpace(el.Title)) {
			continue
		}
		total++
		if samePath(other.Path, el.Path) {
			seen = total
		}
	}
	if total > 1 && seen > 0 {
		sel.Index = seen
	}
	return sel
}

func (t Tree) TypingTarget() (Element, string) {
	var typeables []Element
	for _, el := range t.Elements {
		if !el.Typeable() || !el.Enabled {
			continue
		}
		if el.Focused {
			return el, ""
		}
		typeables = append(typeables, el)
	}

	switch len(typeables) {
	case 0:
		return Element{}, "nothing on screen can hold typed text"
	case 1:
		return typeables[0], ""
	default:
		return Element{}, fmt.Sprintf(
			"%d controls can hold typed text and none of them has the cursor, "+
				"so there is no way to say which one was meant", len(typeables))
	}
}

func (e Element) Center() (x, y int, ok bool) {
	if len(e.Position) != 2 || len(e.Size) != 2 {
		return 0, 0, false
	}
	if e.Size[0] <= 0 || e.Size[1] <= 0 {
		return 0, 0, false
	}
	return e.Position[0] + e.Size[0]/2, e.Position[1] + e.Size[1]/2, true
}
