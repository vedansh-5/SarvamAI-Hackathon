package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/webstyle"
)

const styleSystemPrompt = `You restyle a web page the user is looking at, to show them what they asked to see.

You are given a summary of what the page is made of: selectors, how many elements each one matches, and a sample of what one of them contains. Every selector you write is checked against the page before anything runs. A selector that matches nothing is not a near miss — the restyle is thrown away.

You write rules. A rule is a selector and an emphasis — how strongly to mark what it matches. You never choose a colour: the runtime owns every colour, and it picks one that suits the page's own background, on a light page and a dark one alike.

The emphases are:

- highlight — a soft tint and a hairline ring. The default for "show me which ones".
- attention — the same, in a warmer colour. For something wrong, overdue, or over a limit.
- accent — the same, in a cooler colour. For something merely worth noticing.
- mark — a thin bar down the left edge. For marking many items without tinting the page.
- muted — faded and desaturated. For pushing the things the user did *not* ask about into the background.

Rules that decide whether your answer is usable:

- Use the selectors you were given. Do not invent a class that was not in the summary.
- You are not shown what the page says, only what it is made of. Style the elements the request points at; do not try to match on wording you cannot see.
- Prefer one rule. Marking the thing the user asked about is the whole job, and a page with five different tints on it says nothing.
- To show which items match, mark those items. Do not mute everything else unless the request asks you to.
- Say in one line what the restyle will do, in the words a person would use.

If the page summary does not contain what the request needs, return no rules and say why. That is a real answer. Rules written against selectors you hoped were there are not.`

func styleSchema() map[string]any {
	rule := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selector": map[string]any{"type": "string"},
			"emphasis": map[string]any{"type": "string", "enum": webstyle.EmphasisNames()},
		},
		"required":             []string{"selector", "emphasis"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"rules": map[string]any{"type": "array", "items": rule},
			"why":   map[string]any{"type": "string"},
		},
		"required":             []string{"rules", "why"},
		"additionalProperties": false,
	}
}

// StyleRequest is one attempt at restyling a page.
type StyleRequest struct {
	// Intent is what the user asked to see, in their words.
	Intent string

	// URL and Structure are where it happens and what is there.
	URL       string
	Structure string

	// Failures are why the previous attempt was refused.
	Failures []string
}

// BuildStylePrompt renders the request.
func BuildStylePrompt(req StyleRequest) string {
	var b strings.Builder

	b.WriteString("WHAT TO SHOW\n")
	b.WriteString(req.Intent)
	fmt.Fprintf(&b, "\n\nWHERE\n%s\n", req.URL)

	b.WriteString("\nWHAT THE PAGE IS MADE OF\n")
	b.WriteString("Each line is a selector and how many elements it matches. What the page says is deliberately not shown.\n\n")
	b.WriteString(req.Structure)

	if len(req.Failures) > 0 {
		b.WriteString("\n\nYOUR PREVIOUS ANSWER WAS REFUSED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and answer again.\n")
	}

	return b.String()
}

// StyleAnswer is a restyle and why, or no restyle and why not.
type StyleAnswer struct {
	Rules []webstyle.Rule `json:"rules"`
	Why   string          `json:"why"`
}

// WriteStyle asks for a restyle of the page.
func (c *OpenAICompatible) WriteStyle(ctx context.Context, req StyleRequest) (StyleAnswer, error) {
	if c.BaseURL == "" || c.Model == "" {
		return StyleAnswer{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system := styleSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(styleSchema(), "", "  ")
		if err != nil {
			return StyleAnswer{}, fmt.Errorf("synth: encode style schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildStylePrompt(req)},
		},
		ResponseFormat: c.styleResponseFormat(),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return StyleAnswer{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return StyleAnswer{}, fmt.Errorf("synth: no JSON object in the restyle (got %q)", truncate(raw, 300))
	}

	var a StyleAnswer
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return StyleAnswer{}, fmt.Errorf("synth: decode restyle: %w (from %q)", err, truncate(text, 300))
	}
	return a, nil
}

func (c *OpenAICompatible) styleResponseFormat() json.RawMessage {
	var v any
	switch c.SchemaMode {
	case SchemaJSON:
		v = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "restyle",
				"strict": false,
				"schema": styleSchema(),
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
