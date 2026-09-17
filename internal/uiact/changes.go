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
