package uiact

import (
	"fmt"
	"strings"
)

func Preview(p Program) string {
	lines := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		if d := strings.TrimSpace(s.Describe); d != "" {
			lines = append(lines, d)
			continue
		}
		switch verb(s.Do) {
		case "click":
			lines = append(lines, "Click "+describe(s.Target))
		case "type":
			lines = append(lines, fmt.Sprintf("Type %q into %s", s.Text, describe(s.Target)))
		case "press":
			lines = append(lines, fmt.Sprintf("Press %s in %s", s.Key, describe(s.Target)))
		}
	}
	return strings.Join(lines, "\n")
}

func describe(sel Selector) string {
	switch {
	case sel.Title != "":
		return fmt.Sprintf("the %s %q", sel.Role, sel.Title)
	case sel.Index > 0:
		return fmt.Sprintf("%s number %d", sel.Role, sel.Index)
	default:
		return "the " + sel.Role
	}
}
