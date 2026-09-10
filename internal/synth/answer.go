package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/runner"
)

const answerSystemPrompt = `You answer a question using only the search results you are given.

The results are numbered. Every result is something that was actually on the page — you are not being asked to recall anything, and you have no way to check anything you remember.

Rules:

- Answer from the results and from nothing else. If they do not contain the answer, say that they do not, and say what they do cover. That is a real answer and a useful one.
- Cite the numbers of the results you used. An answer with no citations is thrown away.
- Do not write a URL unless it appears in a result exactly as you write it.
- Be short. Two or three sentences for most questions, one for a simple one. The results are on screen underneath you; repeating them is wasted reading.
- Do not hedge about being an AI, do not describe the results as "the search results", and do not preface. Say the thing.`

func answerSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text":  map[string]any{"type": "string"},
			"cites": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		},
		"required":             []string{"text", "cites"},
		"additionalProperties": false,
	}
}

// AnswerRequest is a question and what was found for it.
type AnswerRequest struct {
	Question string
	Fields   []string
	Rows     []runner.Row

	// Failures are why the previous answer was refused.
	Failures []string
}

// BuildAnswerPrompt renders the rows as the numbered list the model cites
// against.
func BuildAnswerPrompt(req AnswerRequest) string {
	var b strings.Builder

	b.WriteString("QUESTION\n")
	b.WriteString(req.Question)
	b.WriteString("\n\nRESULTS\n")

	for i, row := range req.Rows {
		fmt.Fprintf(&b, "\n[%d]\n", i+1)
		for _, f := range req.Fields {
			v := strings.TrimSpace(row[f])
			if v == "" {
				continue
			}
			fmt.Fprintf(&b, "  %s: %s\n", f, truncate(v, 500))
		}
	}

	if len(req.Failures) > 0 {
		b.WriteString("\nYOUR PREVIOUS ANSWER WAS REFUSED\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
		b.WriteString("Fix that and answer again.\n")
	}

	return b.String()
}

// Answer replies to a question from rows already extracted.
//
// Not validated here, for the same reason routing is not: the caller owns the
// retry ladder, and a refusal returned from inside this function arrives
// shaped like a transport failure and skips it.
func (c *OpenAICompatible) Answer(ctx context.Context, req AnswerRequest) (answer.Answer, error) {
	if c.BaseURL == "" || c.Model == "" {
		return answer.Answer{}, fmt.Errorf("synth: BaseURL and Model are required")
	}
	if len(req.Rows) == 0 {
		return answer.Answer{}, fmt.Errorf("synth: nothing to answer from")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system := answerSystemPrompt
	if c.SchemaMode != SchemaJSON {
		schema, err := json.MarshalIndent(answerSchema(), "", "  ")
		if err != nil {
			return answer.Answer{}, fmt.Errorf("synth: encode answer schema: %w", err)
		}
		system += "\n\nReply with a single JSON object and nothing else — no prose, no markdown fences. It must satisfy this JSON Schema:\n\n" + string(schema)
	}

	body := chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildAnswerPrompt(req)},
		},
		ResponseFormat: c.answerResponseFormat(),
	}

	raw, err := c.post(ctx, body)
	if err != nil {
		return answer.Answer{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return answer.Answer{}, fmt.Errorf("synth: no JSON object in the answer (got %q)", truncate(raw, 300))
	}

	var a answer.Answer
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return answer.Answer{}, fmt.Errorf("synth: decode answer: %w (from %q)", err, truncate(text, 300))
	}
	return a, nil
}

func (c *OpenAICompatible) answerResponseFormat() json.RawMessage {
	var v any
	switch c.SchemaMode {
	case SchemaJSON:
		v = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "answer",
				"strict": false,
				"schema": answerSchema(),
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
