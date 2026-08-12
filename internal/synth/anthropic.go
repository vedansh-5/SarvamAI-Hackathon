package synth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/vedansh/vans/internal/extract"
)

// DefaultModel is the model synthesis runs on.
const DefaultModel anthropic.Model = "claude-opus-5"

// DefaultMaxTokens leaves room for adaptive thinking plus the program itself.
// The program is small; the thinking is not.
const DefaultMaxTokens int64 = 16000

// Anthropic synthesizes programs with the Claude API.
//
// Structured outputs constrain the response to the program schema, so a
// well-formed reply is guaranteed by the API rather than by parsing and
// retrying. Validate still runs afterwards — the schema guarantees shape, not
// that the program makes sense.
type Anthropic struct {
	Client anthropic.Client

	// Model defaults to DefaultModel.
	Model anthropic.Model

	// MaxTokens defaults to DefaultMaxTokens.
	MaxTokens int64

	// Effort is the reasoning budget. Empty uses the API default. Lower it to
	// trade synthesis quality for cold-run latency and cost; it has no effect
	// on warm runs, which never reach this code.
	Effort anthropic.OutputConfigEffort
}

// NewAnthropic builds a synthesizer using ambient credentials.
func NewAnthropic() *Anthropic {
	return &Anthropic{Client: anthropic.NewClient()}
}

func (a *Anthropic) Synthesize(ctx context.Context, req Request) (extract.Program, error) {
	if len(req.Candidates) == 0 {
		return extract.Program{}, fmt.Errorf("synth: no candidates to choose from")
	}

	model := a.Model
	if model == "" {
		model = DefaultModel
	}
	maxTokens := a.MaxTokens
	if maxTokens == 0 {
		maxTokens = DefaultMaxTokens
	}

	adaptive := anthropic.ThinkingConfigAdaptiveParam{}

	resp, err := a.Client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Thinking:  anthropic.ThinkingConfigParamUnion{OfAdaptive: &adaptive},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: a.Effort,
			Format: anthropic.JSONOutputFormatParam{Schema: extract.ProgramSchema()},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(BuildPrompt(req))),
		},
	})
	if err != nil {
		return extract.Program{}, fmt.Errorf("synth: %w", err)
	}

	// A refusal is a successful HTTP response with no usable content. Reading
	// the content blocks first would panic or silently produce nonsense.
	if resp.StopReason == anthropic.StopReasonRefusal {
		return extract.Program{}, fmt.Errorf("synth: model declined the request (%s)", resp.StopDetails.Category)
	}
	if resp.StopReason == anthropic.StopReasonMaxTokens {
		return extract.Program{}, fmt.Errorf("synth: response hit the token limit before completing")
	}

	raw := responseText(resp)
	if raw == "" {
		return extract.Program{}, fmt.Errorf("synth: model returned no text (stop reason %q)", resp.StopReason)
	}

	var p extract.Program
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return extract.Program{}, fmt.Errorf("synth: decode program: %w (output %q)", err, raw)
	}
	if err := p.Validate(); err != nil {
		return extract.Program{}, fmt.Errorf("synth: %w", err)
	}
	return p, nil
}

// responseText concatenates the text blocks, skipping thinking blocks.
func responseText(resp *anthropic.Message) string {
	var out string
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			out += t.Text
		}
	}
	return out
}
