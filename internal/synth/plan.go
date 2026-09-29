package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

var planSystemPrompt = fmt.Sprintf(planSystemPromptFormat, pressingAKey())

const planSystemPromptFormat = `You are planning a whole task on someone's Mac, to be carried out by driving the controls on screen exactly as a person would.

You are shown the task and what is on screen right now. You give the full sequence of steps you believe does the task, in order.

You can only see the screen as it is at this moment. Every step after the first acts on a screen you have not seen, so those are your best expectation of what will be there. That is understood and it is fine. Each step is checked against the real screen immediately before it runs, and anything that no longer fits is planned again from that point with the screen in view. So a plan that is right about the first few steps and wrong later is useful; a plan padded with steps you do not believe in is not.

What a step may be:

- click a control
- type text into a control, which clicks it first
- %s

Rules that decide whether a step is usable:

- Address a control by its role and title, spelled exactly as the screen lists them.
- If several controls share a role and title, the screen numbers them; say which with index.
- The first step must name a control that is listed on screen right now. Later steps may name controls you expect to appear.
- Describe each step in the words a person would use. That line is shown while it happens, and it is how someone watching knows what is being done.
- Do the task and nothing else. An extra click to "tidy up" is a change nobody asked for.

Give the shortest sequence that does the task. Stop the plan where you stop being able to predict the screen — a short honest plan costs one look at the screen to continue, where a long invented one gets thrown away.

If the task cannot be started from what is on screen, give no steps and say why in one line. That is a useful answer.`

func planSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{"type": "array", "items": screenStepSchema("do", "target", "describe")},
			"why":   map[string]any{"type": "string"},
		},
		"required":             []string{"steps", "why"},
		"additionalProperties": false,
	}
}

// PlanRequest asks for a whole task at once.
type PlanRequest struct {
	Task string
	Tree uiact.Tree

	// Most is the longest plan worth giving, which is the same limit the run
	// itself will stop at.
	Most int
}

// PlanAnswer is the sequence the model expects to do the task, and its own
// account of why if there is none.
type PlanAnswer struct {
	Steps []uiact.Step
	Why   string
}

func BuildPlanPrompt(req PlanRequest) string {
	var b strings.Builder

	b.WriteString("THE TASK\n")
	b.WriteString(req.Task)

	b.WriteString("\n\nWHAT IS ON SCREEN NOW\n")
	b.WriteString(req.Tree.TextWithin(uiact.MaxTreeBytes))

	if req.Most > 0 {
		fmt.Fprintf(&b, "\nAt most %d steps.\n", req.Most)
	}

	return b.String()
}

func (c *OpenAICompatible) WritePlan(ctx context.Context, req PlanRequest) (PlanAnswer, error) {
	if c.BaseURL == "" || c.Model == "" {
		return PlanAnswer{}, fmt.Errorf("synth: BaseURL and Model are required")
	}

	maxTokens := c.MaxTokens
	if maxTokens == 0 {
		maxTokens = int(DefaultMaxTokens)
	}

	system, err := c.systemFor(planSystemPrompt, "plan", planSchema)
	if err != nil {
		return PlanAnswer{}, err
	}

	raw, err := c.post(ctx, chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildPlanPrompt(req)},
		},
		ResponseFormat: c.responseFormat("whole_plan", planSchema, false),
	})
	if err != nil {
		return PlanAnswer{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return PlanAnswer{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		Steps []uiact.Step `json:"steps"`
		Why   string       `json:"why"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return PlanAnswer{}, fmt.Errorf("synth: decode plan: %w (from %q)", err, truncate(text, 300))
	}

	if req.Most > 0 && len(answer.Steps) > req.Most {
		answer.Steps = answer.Steps[:req.Most]
	}
	return PlanAnswer{Steps: answer.Steps, Why: answer.Why}, nil
}
