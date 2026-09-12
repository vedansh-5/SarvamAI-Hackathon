package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

const uiActSystemPrompt = `You write a program that does one thing inside a macOS window, by driving the controls a person would click.

You are given the window's accessibility tree: every control on screen, what kind it is, what it is called, and what it currently holds. That list is what exists. Everything you write is checked against it before anything runs, so a control that is not listed is not a near miss — it is a program that will be thrown away.

You do not write code. You write steps. Each step names what to do, and which control to do it to.

What you may do:

- click a control
- type text into a control that can hold text
- press Enter, Escape or Tab

Rules that decide whether your answer is usable:

- Address a control by its role and its title, spelled exactly as the tree spells them.
- If several controls share a role and title, the tree numbers them; say which one with index.
- Text can only be typed into a text field, text area, combo box or search field. Typing into anything else does nothing at all.
- A disabled control cannot be used.
- Where the request contains a placeholder like {s0}, put that placeholder in the value. Your program is written fresh for this window, but the placeholder is how the request says which part is the variable one.
- Say what the user asked for and nothing more. An extra click that "makes sure" is a side effect nobody asked for and nobody confirmed.
- Write a short describe for each step, in the words a person would use. It is the only thing the user sees before confirming.

If the window genuinely does not contain what the request needs, return no steps and say why in one line. That is a useful answer. A program built from controls you hoped were there is not.`

func uiActSchema() map[string]any {
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

	step := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"do":       map[string]any{"type": "string", "enum": []string{"click", "type", "press"}},
			"target":   selector,
			"text":     map[string]any{"type": "string"},
			"key":      map[string]any{"type": "string", "enum": []string{"Enter", "Escape", "Tab"}},
			"describe": map[string]any{"type": "string"},
		},
		"required":             []string{"do", "target"},
		"additionalProperties": false,
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{"type": "array", "items": step},
			"why":   map[string]any{"type": "string"},
		},
		"required":             []string{"steps", "why"},
		"additionalProperties": false,
	}
}

type UIActRequest struct {
	Action string
	Target string
	Tree   uiact.Tree

	Placeholders []string
	Failures     []string
}

func BuildUIActPrompt(req UIActRequest) string {
	var b strings.Builder

	b.WriteString("WHAT TO DO\n")
	b.WriteString(req.Action)
	if req.Target != "" {
		b.WriteString("\n\nWHAT TO DO IT TO\n")
		b.WriteString(req.Target)
	}

	if len(req.Placeholders) > 0 {
		b.WriteString("\n\nPLACEHOLDERS\n")
		b.WriteString("These stand for the parts of the request that change. Copy them into your answer exactly as they appear.\n")
		for _, p := range req.Placeholders {
			fmt.Fprintf(&b, "  {%s}\n", p)
		}
	}

	b.WriteString("\nWHAT IS ON SCREEN\n")
	b.WriteString(req.Tree.TextWithin(uiact.MaxTreeBytes))

	if len(req.Failures) > 0 {
		b.WriteString("\nYOUR PREVIOUS ANSWER WAS REJECTED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and answer again.\n")
	}

	return b.String()
}

func (c *OpenAICompatible) WriteUIAction(ctx context.Context, req UIActRequest) (uiact.Program, error) {
	if c.BaseURL == "" || c.Model == "" {
		return uiact.Program{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system := uiActSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(uiActSchema(), "", "  ")
		if err != nil {
			return uiact.Program{}, fmt.Errorf("synth: encode program schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildUIActPrompt(req)},
		},
		ResponseFormat: c.uiActResponseFormat(),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return uiact.Program{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return uiact.Program{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		Steps []uiact.Step `json:"steps"`
		Why   string       `json:"why"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return uiact.Program{}, fmt.Errorf("synth: decode program: %w (from %q)", err, truncate(text, 300))
	}

	if len(answer.Steps) == 0 {
		return uiact.Program{}, &UnwritableError{App: req.Tree.App, Why: answer.Why}
	}

	return uiact.Program{App: req.Tree.App, Steps: answer.Steps}, nil
}

func (c *OpenAICompatible) uiActResponseFormat() json.RawMessage {
	var v any
	switch c.SchemaMode {
	case SchemaJSON:
		v = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "ui_action",
				"strict": false,
				"schema": uiActSchema(),
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
