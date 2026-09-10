package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/capability"
)

const appActSystemPrompt = `You write a program that does one thing inside a macOS application.

You are given the application's own scripting dictionary: the commands it answers and the objects it holds. That list is complete and it is the truth. Everything you write is checked against it before anything runs, so a command or property that is not listed is not a near miss — it is a program that will be thrown away.

You do not write code. You write steps. Each step names a command, what it acts on, and what to set.

Rules that decide whether your answer is usable:

- Use only commands and objects from the dictionary you were given.
- Spell them exactly as the dictionary spells them, including spaces in names like "file transfer".
- Set only properties that the dictionary marks writable. A read-only property is refused.
- To act on something inside something else, bind the outer one first and refer to that binding by name.
- Where the request contains a placeholder like {s0}, put that placeholder in the value. Your program is kept and reused for every future request of this shape, so a value you write in by hand becomes the value every one of them gets.
- Say what the user asked for and nothing more. An extra step that "tidies up" or "makes sure" is a side effect nobody asked for and nobody confirmed.

If the dictionary genuinely cannot express what was asked, return no steps and say why in one line. That is a useful answer. A program built from commands you hoped existed is not.`

func appActSchema() map[string]any {
	selector := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"class": map[string]any{"type": "string"},
			"in":    map[string]any{"type": "string"},
			"name":  map[string]any{"type": "string"},
			"index": map[string]any{"type": "integer"},
		},
		"required":             []string{"class"},
		"additionalProperties": false,
	}

	step := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
			"target":  selector,
			"make":    map[string]any{"type": "string"},
			"with": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"bind": map[string]any{"type": "string"},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"app":   map[string]any{"type": "string"},
			"steps": map[string]any{"type": "array", "items": step},
			"why":   map[string]any{"type": "string"},
		},
		"required":             []string{"app", "steps", "why"},
		"additionalProperties": false,
	}
}

// AppActRequest is one attempt at writing a program.
type AppActRequest struct {
	// Action is what to do, in the user's words.
	Action string

	// Target is the specific thing to act on, when the instruction named one.
	Target string

	// Surface is what the application declares it can do.
	Surface capability.Surface

	// Placeholders are the value-slots in the request. They stand for values
	// that differ between requests of this shape and must be passed through
	// rather than replaced with what this one instruction happens to say.
	Placeholders []string

	// Failures are the reasons the previous attempt was rejected. Rung 2 of
	// the ladder (D-020): the model is told what was wrong rather than asked
	// again and hoped over.
	Failures []string
}

// BuildAppActPrompt renders the user half of the request.
func BuildAppActPrompt(req AppActRequest) string {
	var b strings.Builder

	b.WriteString("WHAT TO DO\n")
	b.WriteString(req.Action)
	if req.Target != "" {
		b.WriteString("\n\nWHAT TO DO IT TO\n")
		b.WriteString(req.Target)
	}

	if len(req.Placeholders) > 0 {
		b.WriteString("\n\nPLACEHOLDERS\n")
		b.WriteString("These stand for values that change between requests. Copy them into your answer exactly as they appear — do not replace them with anything, and do not invent new ones.\n")
		for _, p := range req.Placeholders {
			fmt.Fprintf(&b, "  {%s}\n", p)
		}
	}

	b.WriteString("\nWHAT THIS APPLICATION CAN DO\n")
	b.WriteString(req.Surface.TextWithin(capability.MaxSurfaceBytes))

	if len(req.Failures) > 0 {
		b.WriteString("\nYOUR PREVIOUS ANSWER WAS REJECTED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and answer again.\n")
	}

	return b.String()
}

// UnwritableError is the model saying the dictionary cannot express the
// request. Distinct from a rejected program: asking again will not help.
type UnwritableError struct {
	App string
	Why string
}

func (e *UnwritableError) Error() string {
	return fmt.Sprintf("synth: %s cannot be asked to do that: %s", e.App, e.Why)
}

// WriteAction asks for a program. It does not validate the answer — the caller
// owns the retry ladder, and judging here would return a fixable problem
// shaped like a transport failure (D-039).
func (c *OpenAICompatible) WriteAction(ctx context.Context, req AppActRequest) (appact.Program, error) {
	if c.BaseURL == "" || c.Model == "" {
		return appact.Program{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system := appActSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(appActSchema(), "", "  ")
		if err != nil {
			return appact.Program{}, fmt.Errorf("synth: encode program schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildAppActPrompt(req)},
		},
		ResponseFormat: c.appActResponseFormat(),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return appact.Program{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return appact.Program{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		App   string        `json:"app"`
		Steps []appact.Step `json:"steps"`
		Why   string        `json:"why"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return appact.Program{}, fmt.Errorf("synth: decode program: %w (from %q)", err, truncate(text, 300))
	}

	if len(answer.Steps) == 0 {
		return appact.Program{}, &UnwritableError{App: req.Surface.App, Why: answer.Why}
	}

	// The application is the runtime's, not the model's. It was resolved from
	// an installed bundle before the prompt was built, and a model that names
	// a different one here would otherwise be choosing which application to
	// act in.
	return appact.Program{App: req.Surface.App, Steps: answer.Steps}, nil
}

func (c *OpenAICompatible) appActResponseFormat() json.RawMessage {
	var v any
	switch c.SchemaMode {
	case SchemaJSON:
		v = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "app_action",
				"strict": false,
				"schema": appActSchema(),
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
