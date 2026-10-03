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
		case "doubleclick":
			lines = append(lines, "Double-click "+describe(s.Target))
		case "rightclick":
			lines = append(lines, "Right-click "+describe(s.Target))
		case "hover":
			lines = append(lines, "Point at "+describe(s.Target))
		case "scroll":
			lines = append(lines, fmt.Sprintf("Scroll %s in %s", s.Text, describe(s.Target)))
		case "drag":
			if s.To != nil {
				lines = append(lines, fmt.Sprintf("Drag %s to %s", describe(s.Target), describe(*s.To)))
			}
		case "set":
			lines = append(lines, fmt.Sprintf("Set %s to %s", describe(s.Target), s.Text))
		case "wait":
			lines = append(lines, fmt.Sprintf("Wait until %s %s", describe(s.Target), s.WaitsFor()))
		case "choose":
			lines = append(lines, fmt.Sprintf("Choose the file %q with %s", s.Text, describe(s.Target)))
		case "switch":
			lines = append(lines, "Switch to "+s.App)
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
