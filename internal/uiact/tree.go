package uiact

import (
	"fmt"
	"strings"
)

type Element struct {
	Role    string   `json:"role"`
	Title   string   `json:"title,omitempty"`
	Value   string   `json:"value,omitempty"`
	Enabled bool     `json:"enabled"`
	Actions []string `json:"actions,omitempty"`
	Path    []int    `json:"path"`
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
