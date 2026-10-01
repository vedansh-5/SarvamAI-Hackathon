package uiact

import (
	"regexp"
	"strings"
)

var commits = regexp.MustCompile(`(?i)\b(pay|pay now|buy|buy now|purchase|place (your |the )?order|order now|checkout|check out|` +
	`book now|confirm (and|&) pay|confirm (your |the )?(order|booking|payment|purchase)|complete (your |the )?(order|booking|payment|purchase)|` +
	`publish|post|tweet|send|submit|delete|remove|erase|discard|unsubscribe|subscribe|transfer|withdraw|donate)\b`)

func Commits(step Step, el Element) (bool, string) {
	if step.Final {
		return true, "the step is marked as one that cannot be taken back"
	}
	switch verb(step.Do) {
	case "click":
		name := strings.TrimSpace(el.Label())
		if name == "" {
			name = strings.TrimSpace(step.Target.Title)
		}
		if word := commits.FindString(name); word != "" {
			return true, "the control says " + strings.ToLower(word)
		}
	case "press":
		if step.Key == "Enter" && el.Typeable() && role(el.Role) != "searchfield" && looksLikeAMessage(el) {
			return true, "Enter sends what was typed"
		}
	}
	return false, ""
}

var messageFields = regexp.MustCompile(`(?i)\b(message|reply|comment|chat|compose|say something|write a)\b`)

func looksLikeAMessage(el Element) bool {
	return messageFields.MatchString(el.Label())
}
