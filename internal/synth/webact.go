package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/webact"
)

const webActSystemPrompt = `You drive a web page the user already has open, by writing the steps a person would take.

You are given the page's own HTML. Every selector you write is checked against it before anything runs, so a selector that matches nothing — or matches several things — is not a near miss. It is a program that gets thrown away.

You may only click, type, and press Enter, Escape or Tab.

Rules that decide whether your answer is usable:

- Every selector must match exactly one element. Prefer an id. Where there is none, use whatever attribute the page gives you that is unique — aria-label, name, data-testid, placeholder, role.
- Type only into an input, a textarea, or something contenteditable. Typing into anything else does nothing at all, and the request looks like it worked.
- Where the request contains a placeholder like {s0}, put that placeholder in the text. Your program is kept and reused for every future request of this shape, so text you write in by hand becomes the text every one of them sends.
- Write a describe for each step, in the words a person would use: "Type the message into the composer", "Click Send". That line is what the user is shown before they confirm, and it is the only thing standing between them and a message they did not mean to send.
- Do exactly what was asked and nothing else. No extra click to "make sure", no navigating anywhere, no dismissing anything on the way.

If the page does not offer what the request needs, return no steps and say why in one line. That is a real answer. A program built from selectors you hoped were there is not.`

func webActSchema() map[string]any {
	step := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"do":       map[string]any{"type": "string", "enum": []string{"click", "type", "press"}},
			"selector": map[string]any{"type": "string"},
			"text":     map[string]any{"type": "string"},
			"key":      map[string]any{"type": "string", "enum": []string{"Enter", "Escape", "Tab"}},
			"describe": map[string]any{"type": "string"},
		},
		"required":             []string{"do", "selector", "describe"},
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

// WebActRequest is one attempt at driving a page.
type WebActRequest struct {
	// Action is what to do, in the user's words, with values still as
	// placeholders.
	Action string

	// URL and Page are where it happens and what is there.
	URL  string
	Page string

	// Placeholders stand for the values that differ between requests.
	Placeholders []string

	// Failures are why the previous attempt was rejected.
	Failures []string
}

// MaxPageBytes bounds how much of a page is sent.
//
// A modern application's DOM is megabytes, most of it inline styles and
// framework bookkeeping. What matters for driving it is the interactive
// elements, which is what the trimming keeps.
const MaxPageBytes = 60000

// BuildWebActPrompt renders the request.
func BuildWebActPrompt(req WebActRequest) string {
	var b strings.Builder

	b.WriteString("WHAT TO DO\n")
	b.WriteString(req.Action)
	fmt.Fprintf(&b, "\n\nWHERE\n%s\n", req.URL)

	if len(req.Placeholders) > 0 {
		b.WriteString("\nPLACEHOLDERS\n")
		b.WriteString("These stand for values that change between requests. Copy them into your answer exactly as they appear.\n")
		for _, p := range req.Placeholders {
			fmt.Fprintf(&b, "  {%s}\n", p)
		}
	}

	b.WriteString("\nTHE PAGE\n")
	b.WriteString(truncate(req.Page, MaxPageBytes))

	if len(req.Failures) > 0 {
		b.WriteString("\n\nYOUR PREVIOUS ANSWER WAS REJECTED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and answer again.\n")
	}

	return b.String()
}

// WriteWebAction asks for a program. Not validated here: the caller owns the
// ladder.
func (c *OpenAICompatible) WriteWebAction(ctx context.Context, req WebActRequest) (webact.Program, error) {
	if c.BaseURL == "" || c.Model == "" {
		return webact.Program{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system := webActSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(webActSchema(), "", "  ")
		if err != nil {
			return webact.Program{}, fmt.Errorf("synth: encode program schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildWebActPrompt(req)},
		},
		ResponseFormat: c.webActResponseFormat(),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return webact.Program{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return webact.Program{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		Steps []webact.Step `json:"steps"`
		Why   string        `json:"why"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return webact.Program{}, fmt.Errorf("synth: decode program: %w (from %q)", err, truncate(text, 300))
	}
	if len(answer.Steps) == 0 {
		return webact.Program{}, &UnwritableError{App: req.URL, Why: answer.Why}
	}

	return webact.Program{URL: req.URL, Steps: answer.Steps}, nil
}

func (c *OpenAICompatible) webActResponseFormat() json.RawMessage {
	var v any
	switch c.SchemaMode {
	case SchemaJSON:
		v = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "web_action",
				"strict": false,
				"schema": webActSchema(),
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
