package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

const driveSystemPrompt = `You are working through a task on someone's Mac, one step at a time, by using the controls on screen exactly as a person would.

You are shown the task, what is on screen right now, and what has already been done. You choose the single next step. You will be shown the screen again after it happens, so do not plan ahead and do not batch actions.

What a step may be:

- click a control
- type text into a control, which clicks it first
- press Enter, Escape or Tab

Rules that decide whether your step is usable:

- Address a control by its role and title, spelled exactly as the screen lists them.
- If several controls share a role and title, the screen numbers them; say which with index.
- Only choose a control that is actually listed. One that is not there is a step that gets thrown away.
- Describe the step in the words a person would use. That line is shown while it happens, and it is how someone watching knows what you are doing.
- Do the task and nothing else. An extra click to "tidy up" is a change nobody asked for.

Set done when the task is finished, or when it cannot be carried on from what is on screen, and say why in one line. Stopping and saying why is a good answer. Clicking something plausible to appear productive is not.`

func driveSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"done": map[string]any{"type": "boolean"},
			"why":  map[string]any{"type": "string"},
			"step": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"do": map[string]any{"type": "string", "enum": []string{"click", "type", "press"}},
					"target": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"role":  map[string]any{"type": "string"},
							"title": map[string]any{"type": "string"},
							"index": map[string]any{"type": "integer"},
						},
						"required":             []string{"role"},
						"additionalProperties": false,
					},
					"text":     map[string]any{"type": "string"},
					"key":      map[string]any{"type": "string", "enum": []string{"Enter", "Escape", "Tab"}},
					"describe": map[string]any{"type": "string"},
				},
				"required":             []string{"do", "target", "describe"},
				"additionalProperties": false,
			},
		},
		"required":             []string{"done", "why"},
		"additionalProperties": false,
	}
}

type StepRequest struct {
	Task     string
	Tree     uiact.Tree
	Done     []string
	Failures []string
}

type StepAnswer struct {
	Step uiact.Step
	Done bool
	Why  string
}

func BuildStepPrompt(req StepRequest) string {
	var b strings.Builder

	b.WriteString("THE TASK\n")
	b.WriteString(req.Task)

	if len(req.Done) > 0 {
		b.WriteString("\n\nWHAT YOU HAVE DONE SO FAR\n")
		for i, d := range req.Done {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, d)
		}
	} else {
		b.WriteString("\n\nNothing has been done yet.\n")
	}

	b.WriteString("\nWHAT IS ON SCREEN NOW\n")
	b.WriteString(req.Tree.TextWithin(uiact.MaxTreeBytes))

	if len(req.Failures) > 0 {
		b.WriteString("\nYOUR LAST STEP WAS REJECTED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Choose a different step.\n")
	}

	return b.String()
}

func (c *OpenAICompatible) WriteStep(ctx context.Context, req StepRequest) (StepAnswer, error) {
	if c.BaseURL == "" || c.Model == "" {
		return StepAnswer{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system := driveSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(driveSchema(), "", "  ")
		if err != nil {
			return StepAnswer{}, fmt.Errorf("synth: encode step schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	raw, err := c.post(ctx, chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildStepPrompt(req)},
		},
		ResponseFormat: c.driveResponseFormat(),
	})
	if err != nil {
		return StepAnswer{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return StepAnswer{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		Step *uiact.Step `json:"step"`
		Done bool        `json:"done"`
		Why  string      `json:"why"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return StepAnswer{}, fmt.Errorf("synth: decode step: %w (from %q)", err, truncate(text, 300))
	}

	out := StepAnswer{Done: answer.Done, Why: answer.Why}
	if answer.Step != nil {
		out.Step = *answer.Step
	}
	if !out.Done && answer.Step == nil {
		return StepAnswer{}, fmt.Errorf("synth: neither a step nor a reason to stop")
	}
	return out, nil
}

func (c *OpenAICompatible) driveResponseFormat() json.RawMessage {
	var v any
	switch c.SchemaMode {
	case SchemaJSON:
		v = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "next_step",
				"strict": false,
				"schema": driveSchema(),
			},
		}
	case SchemaObject:
		v = map[string]any{"type": "json_object"}
	default:
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
