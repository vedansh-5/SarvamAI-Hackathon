package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

var driveSystemPrompt = fmt.Sprintf(driveSystemPromptFormat, pressingAKey())

const driveSystemPromptFormat = `You are working through a task on someone's Mac, one step at a time, by using the controls on screen exactly as a person would.

You are shown the task, what is on screen right now, and what has already been done. You choose the single next step. You will be shown the screen again after it happens, so do not plan ahead and do not batch actions.

What a step may be:

- click a control
- type text into a control, which clicks it first
- %s

Rules that decide whether your step is usable:

- Address a control by its role and title, spelled exactly as the screen lists them.
- If several controls share a role and title, the screen numbers them; say which with index.
- A control marked [out of view] is scrolled out of sight. If one in view does the same job, choose that. If the one you need is out of view, name it anyway: it is scrolled to before the step happens. Never describe scrolling as a step of its own.
- On a web page, a list of choices is shown as an AXComboBox: type the name of the choice into it to pick it. The AXWebArea is the page itself, never a step's target.
- Set final to true on a step that commits something that cannot be taken back: paying, placing an order, booking, publishing, posting, sending a message, deleting. The person is asked before that step runs, so mark it rather than avoid it. Mark only the step that commits it, such as the click on Send or the Enter that sends; typing the text is never final.
- Never type a password, a card number, a CVV, a PIN or a one-time code, and never solve a captcha. Those fields are listed as AXSecureTextField and cannot be typed into. When the task reaches one, stop there and say what the person needs to enter.
- Set fill to true on a step that only enters or picks a value the person gave: typing their text, choosing their date, picking their city or option from a list. Never on a step that opens, navigates, searches, submits or sends.
- AXMenuBarItem controls are the menu bar at the top of the screen (File, Edit, View and so on). Click one to open it; its rows are listed as AXMenuItem on the next screen, and that is when to click one.
- Only choose a control that is actually listed. One that is not there is a step that gets thrown away.
- Describe the step in the words a person would use. That line is shown while it happens, and it is how someone watching knows what you are doing.
- Do the task and nothing else. An extra click to "tidy up" is a change nobody asked for.
- Never give a step that is already listed under what has been done. Being listed there means it happened, whatever the screen looks like now — a search field still holding the word that was typed into it is the result of that step, not a reason to type it again. Give the step that follows it.

` + askingRule + `

Every answer is a step, a question or a stop. If done is false you must give a step; an answer with neither is not an answer, and it is thrown away.

Set done only when every part of the task has been done. A task with several parts, such as "open the panel and search for python", is done when the last part is, not the first. Say in one line what shows it.

If part of the task remains and nothing on screen can do it, set stuck, not done, and say what is missing in one line. Stopping and saying why is a good answer. Calling a half-finished task done is not, and neither is clicking something plausible to appear productive. Stuck is for a screen that cannot do it. When what is missing is a detail only the person knows, that is a question: set ask, not stuck.`

func driveSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"done":  map[string]any{"type": "boolean"},
			"why":   map[string]any{"type": "string"},
			"step":  screenStepSchema("do", "target", "describe"),
			"ask":   askSchema(),
			"stuck": map[string]any{"type": "boolean"},
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

	Question *Question

	Stuck bool
}

func BuildStepPrompt(req StepRequest) string {
	var b strings.Builder

	b.WriteString("THE TASK\n")
	b.WriteString(req.Task)
	fmt.Fprintf(&b, "\n\nTODAY IS %s", today())

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

	system, err := c.systemFor(driveSystemPrompt, "step", driveSchema)
	if err != nil {
		return StepAnswer{}, err
	}

	raw, err := c.post(ctx, chatRequest{
		Model:     c.Model,
		MaxTokens: maxTokens,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: BuildStepPrompt(req)},
		},
		ResponseFormat: c.responseFormat("next_step", driveSchema, false),
	})
	if err != nil {
		return StepAnswer{}, err
	}

	text := extractJSONObject(raw)
	if text == "" {
		return StepAnswer{}, fmt.Errorf("synth: no JSON object in the response (got %q)", truncate(raw, 300))
	}

	var answer struct {
		Step  *uiact.Step `json:"step"`
		Done  bool        `json:"done"`
		Why   string      `json:"why"`
		Ask   *askedJSON  `json:"ask"`
		Stuck bool        `json:"stuck"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return StepAnswer{}, fmt.Errorf("synth: decode step: %w (from %q)", err, truncate(text, 300))
	}

	if q := answer.Ask.question(); q != nil && answer.Step == nil && !answer.Done {
		return StepAnswer{Question: q, Why: answer.Why}, nil
	}

	if answer.Stuck && answer.Step == nil {
		return StepAnswer{Stuck: true, Why: answer.Why}, nil
	}

	out := StepAnswer{Done: answer.Done, Why: answer.Why}
	if answer.Step != nil {
		out.Step = *answer.Step
	}
	if !out.Done && answer.Step == nil {
		return StepAnswer{}, &NoStepError{Why: strings.TrimSpace(answer.Why)}
	}
	return out, nil
}

type NoStepError struct {
	Why string
}

func (e *NoStepError) Error() string {
	return "synth: neither a step nor a reason to stop"
}

const NoStepNote = "Your last answer gave no step, no question and no stop, so it was thrown away. If you need a detail from the person, put the question in ask. If nothing on screen can do what is left, set stuck. Otherwise give the step."
