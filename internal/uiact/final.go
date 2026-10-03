package uiact

import (
	"regexp"
	"strconv"
	"strings"
)

var commits = regexp.MustCompile(`(?i)\b(pay|pay now|buy|buy now|purchase|place (your |the )?order|order now|checkout|check out|` +
	`book now|confirm (and|&) pay|confirm (your |the )?(order|booking|payment|purchase)|complete (your |the )?(order|booking|payment|purchase)|` +
	`publish|post|tweet|send|submit|delete|remove|erase|discard|unsubscribe|subscribe|transfer|withdraw|donate)\b`)

func Commits(step Step, el Element) (bool, string) {
	if step.Final && verb(step.Do) != "type" {
		return true, "the step is marked as one that cannot be taken back"
	}
	switch verb(step.Do) {
	case "choose":
		return true, "it hands a file over"
	case "click", "doubleclick":
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

func EntersAValue(step Step, el Element) bool {
	if final, _ := Commits(step, el); final {
		return false
	}
	return verb(step.Do) == "type" || verb(step.Do) == "set" || step.Fill
}

var messageFields = regexp.MustCompile(`(?i)\b(message|reply|comment|chat|compose|say something|write a)\b`)

func looksLikeAMessage(el Element) bool {
	return messageFields.MatchString(el.Label())
}

var spending = regexp.MustCompile(`(?i)\b(pay|buy|purchase|order|checkout|check out|book|donate|transfer|subscribe)\b`)

func Spends(step Step, el Element) bool {
	if verb(step.Do) != "click" {
		return false
	}
	name := strings.TrimSpace(el.Label())
	if name == "" {
		name = strings.TrimSpace(step.Target.Title)
	}
	return spending.MatchString(name)
}

var money = regexp.MustCompile(`(?i)(?:\x{20B9}|rs\.?\s?|inr\s?|\$|usd\s?|\x{20AC}|eur\s?|\x{00A3}|gbp\s?)\s?(\d[\d,]*(?:\.\d+)?)`)

func Amounts(t Tree) []float64 {
	var out []float64
	read := func(text string) {
		for _, m := range money.FindAllStringSubmatch(text, -1) {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64); err == nil && v > 0 {
				out = append(out, v)
			}
		}
	}
	for _, el := range t.Elements {
		read(el.Title)
		read(el.Value)
	}
	return out
}
