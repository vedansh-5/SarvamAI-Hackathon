package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/route"
)

// Routing lives here rather than in the route package so that the claim in
// this package's doc comment stays true: this is the only place in the runtime
// that talks to a language model. Everything else is deterministic, and that
// boundary is easier to keep when there is exactly one file to look in.

const routeSystemPrompt = `You decide what a user's instruction is asking for, so a runtime can carry it out.

You are shown the instruction twice: once as the user typed it, and once as a SHAPE with its values replaced by placeholders like {u0} (a web address), {n0} (a number) and {s0} (quoted text).

Answer about the SHAPE, not about this one instruction. Your answer is cached and reused for every future instruction of the same shape, so slot values must be written as templates that reference placeholders — "{u0}", or "top {n0} stories" — never the literal values you can see.

Only use placeholders listed as available. Inventing one that is not there makes the answer unusable.

Pick the kind that genuinely fits. If none does, answer with kind "unknown" and say why in one line: that is a useful answer, and guessing at the nearest kind is not.`

// routeSchema constrains the decision. Slots is an open map because each kind
// has its own, and enumerating them all here would be a second copy of
// route.Specs to keep in step.
func routeSchema() map[string]any {
	kinds := make([]string, 0, len(route.Specs)+1)
	for _, s := range route.Specs {
		kinds = append(kinds, string(s.Kind))
	}
	kinds = append(kinds, string(route.KindUnknown))

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind": map[string]any{"type": "string", "enum": kinds},
			"slots": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"why": map[string]any{"type": "string"},
		},
		"required":             []string{"kind", "slots", "why"},
		"additionalProperties": false,
	}
}

// BuildRoutePrompt renders the user half of the routing request.
//
// The kinds are described from route.Specs rather than written out here, so a
// capability added to the runtime is offered to the model without anyone
// remembering to update a prompt.
func BuildRoutePrompt(req route.Request) string {
	var b strings.Builder

	b.WriteString("INSTRUCTION (as typed)\n")
	b.WriteString(req.Instruction)
	b.WriteString("\n\nSHAPE (answer about this)\n")
	b.WriteString(req.Shape)

	b.WriteString("\n\nAVAILABLE PLACEHOLDERS\n")
	if len(req.Placeholders) == 0 {
		b.WriteString("(none — this shape carries no values, so slot templates must be plain text)\n")
	} else {
		for _, p := range req.Placeholders {
			fmt.Fprintf(&b, "  {%s}\n", p)
		}
	}

	b.WriteString("\nKINDS\n")
	for _, spec := range route.Specs {
		fmt.Fprintf(&b, "\n  %s — %s\n", spec.Kind, spec.Describe)
		for _, slot := range spec.Slots {
			need := "optional"
			if slot.Required {
				need = "required"
			}
			fmt.Fprintf(&b, "      %s (%s): %s\n", slot.Name, need, slot.Describe)
		}
	}
	fmt.Fprintf(&b, "\n  %s — none of the above fits.\n", route.KindUnknown)

	if len(req.Failures) > 0 {
		b.WriteString("\nYOUR PREVIOUS ANSWER WAS REJECTED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and answer again.\n")
	}

	return b.String()
}

// Decide routes an instruction. It implements route.Decider.
func (c *OpenAICompatible) Decide(ctx context.Context, req route.Request) (route.Decision, error) {
	if c.BaseURL == "" || c.Model == "" {
		return route.Decision{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	system := routeSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(routeSchema(), "", "  ")
		if err != nil {
			return route.Decision{}, fmt.Errorf("synth: encode routing schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildRoutePrompt(req)},
		},
		ResponseFormat: c.routeResponseFormat(),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return route.Decision{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return route.Decision{}, fmt.Errorf("synth: no JSON object in routing response (got %q)", truncate(raw, 300))
	}

	var d route.Decision
	if err := json.Unmarshal([]byte(text), &d); err != nil {
		return route.Decision{}, fmt.Errorf("synth: decode decision: %w (output %q)", err, truncate(text, 300))
	}
	if err := d.Validate(); err != nil {
		return route.Decision{}, fmt.Errorf("synth: %w", err)
	}
	return d, nil
}

func (c *OpenAICompatible) routeResponseFormat() json.RawMessage {
	switch c.SchemaMode {
	case SchemaJSON:
		b, err := json.Marshal(map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name": "routing_decision",
				// Not strict. strict mode forbids open maps, and slots must
				// stay open because each kind carries its own set — pinning
				// them here would be a second copy of route.Specs.
				"strict": false,
				"schema": routeSchema(),
			},
		})
		if err != nil {
			return nil
		}
		return b
	case SchemaObject:
		return json.RawMessage(`{"type":"json_object"}`)
	default:
		return nil
	}
}
