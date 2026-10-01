package synth

import (
	"fmt"
	"strings"

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
			"do":     map[string]any{"type": "string", "enum": []string{"click", "type", "press"}},
			"target": selector,
			"text":   map[string]any{"type": "string"},
			"key":    map[string]any{"type": "string", "enum": uiact.KeyNames()},
			"modifiers": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string", "enum": uiact.ModifierNames()},
			},
			"describe": map[string]any{"type": "string"},
			"final":    map[string]any{"type": "boolean"},
		},
		"required":             required,
		"additionalProperties": false,
	}
}

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
