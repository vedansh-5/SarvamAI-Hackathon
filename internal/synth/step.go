package synth

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

// One description of a step on screen, shared by the two tiers that write them:
// a program for a whole action, and the next step of a task. They differ in what
// they require of a step, not in what a step is.
func screenStepSchema(required ...string) map[string]any {
	selector := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"role":  map[string]any{"type": "string"},
			"title": map[string]any{"type": "string"},
			"index": map[string]any{"type": "integer"},
		},
		"required":             []string{"role"},
		"additionalProperties": false,
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"do":     map[string]any{"type": "string", "enum": []string{"click", "type", "press", "choose", "switch"}},
			"app":    map[string]any{"type": "string"},
			"target": selector,
			"text":   map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string", "enum": uiact.KeyNames()},
			"modifiers": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "enum": uiact.ModifierNames()},
			},
			"describe": map[string]any{"type": "string"},
			"final":    map[string]any{"type": "boolean"},
			"fill":     map[string]any{"type": "boolean"},
		},
		"required":             required,
		"additionalProperties": false,
	}
}

type Question struct {
	Ask     string
	Options []string
}

const MostOptions = 8

func askSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string"},
			"options":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required":             []string{"question", "options"},
		"additionalProperties": false,
	}
}

type askedJSON struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

func (a *askedJSON) question() *Question {
	if a == nil || strings.TrimSpace(a.Question) == "" {
		return nil
	}
	q := &Question{Ask: strings.TrimSpace(a.Question)}
	for _, o := range a.Options {
		if o = strings.TrimSpace(o); o != "" && len(q.Options) < MostOptions {
			q.Options = append(q.Options, o)
		}
	}
	return q
}

const askingRule = `If what to do next depends on a choice only the person can make, do not guess. Give no step, and set ask: a short question, and as options the choices exactly as the screen shows them (at most 8). This is for things like which of several flights, which of two contacts with the same name, which file, or a detail the task needs and does not say. Do not ask about anything the task already states, and do not ask for permission; a step that cannot be taken back is already confirmed separately. If an answer is listed under what has been done, use it and do not ask again.

The question goes in ask, never in why: an answer that only explains in why what it needs to know is thrown away. Ask before the first step that needs the detail, not after it; opening a field you cannot fill is not progress.

A value you type or pick must come from the task, from an answer the person gave, or from the screen. Never make one up to keep going: a city, a date, a name or an amount the task does not give is a question, not a guess.

Ask once. When several details are missing, put all of them in one question ("Where from, where to, and on what date?") and leave options empty. Never ask about something the screen already has a value for: a form that opens on one way, economy or one traveller is left on those unless the task says otherwise.

You are told today's date. Work out "tomorrow", "next Friday" and the like from it yourself, and never ask for a date the person has already given in words.`

func today() string {
	return Now().Format("Monday 2 January 2006")
}

var Now = time.Now

// What the prompts say a press can be. Written from the vocabulary the runtime
// actually accepts, so the two cannot drift apart into a step that reads as
// allowed and is refused.
func pressingAKey() string {
	return fmt.Sprintf("press %s, on its own or with %s held down",
		orList(uiact.KeyNames()), orList(uiact.ModifierNames()))
}

func orList(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

const MostOpenAppsListed = 16

func writeOpenApps(b *strings.Builder, open []string, here string) {
	var others []string
	for _, name := range open {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(here)) || len(others) == MostOpenAppsListed {
			continue
		}
		others = append(others, name)
	}
	if len(others) == 0 {
		return
	}
	b.WriteString("\nTHE OTHER OPEN APPLICATIONS\n  ")
	b.WriteString(strings.Join(others, ", "))
	b.WriteString("\n")
}

const (
	MostTokensForAStep = 1200
	MostTokensForAPlan = 3500
)

var asksThePerson = regexp.MustCompile(`(?i)(\?\s*$|^\s*(please|which|what|where|who|when|could you|can you|would you)\b|\b(please (provide|specify|tell|give|say|confirm|choose)|let me know|need(s)? to know|i need the|which (file|one|date|city|contact|account))\b)`)

const longestQuestionFromWhy = 240

func questionIn(why string) *Question {
	why = strings.TrimSpace(why)
	if why == "" || len([]rune(why)) > longestQuestionFromWhy || !asksThePerson.MatchString(why) {
		return nil
	}
	return &Question{Ask: why}
}
