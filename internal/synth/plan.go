package synth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

func planSystemPrompt() string { return fmt.Sprintf(planSystemPromptFormat, pressingAKey()) }

const planSystemPromptFormat = `You are planning a whole task on someone's Mac, to be carried out by driving the controls on screen exactly as a person would.

You are shown the task and what is on screen right now. You give the full sequence of steps you believe does the task, in order.

You can only see the screen as it is at this moment. Every step after the first acts on a screen you have not seen, so those are your best expectation of what will be there. That is understood and it is fine. Each step is checked against the real screen immediately before it runs, and anything that no longer fits is planned again from that point with the screen in view. So a plan that is right about the first few steps and wrong later is useful; a plan padded with steps you do not believe in is not.

What a step may be:

- click a control
- type text into a control, which clicks it first
- %s
- choose a file: do is choose, the target is the control that opens the file chooser (Upload, Attach, Select files, Browse), and text is the file's full path. The chooser itself is worked for you, so never click that control as a step of its own: a click leaves a chooser open that nothing can work. Only a file the person named may be chosen. If the task needs a file and they have not said which, ask for its path before anything else.
- switch to another application: do is switch, app is its name exactly as it is listed under the other open applications, and target is {"role": "AXApplication"}. Use it when the task moves from one application to another. The screen you are shown next is that application's window.
- double-click a control (do is doubleclick): opens a file, a row or an item that a single click only selects.
- right-click a control (do is rightclick): opens that control's own menu, whose rows are listed on the next screen.
- hover over a control (do is hover): rests the pointer on something that only shows its menu or its buttons while pointed at.
- drag one control onto another (do is drag): target is what is picked up and to is where it is dropped, both named like any other control.
- set a slider or stepper (do is set): it is listed with its value and the range it runs over. Give the number wanted as text, inside that range. Never drag a slider.
- scroll (do is scroll, text is up, down, left or right, target is the list or page): only to bring into view content that is not listed yet, such as more results in a list that loads as it is scrolled. Never to reach a control that is already listed, in view or not.
- wait (do is wait): for something that takes time and needs nobody, such as an upload finishing or a button that stays greyed out while something is processed. Target is the control being waited for, and text is appears, ready or goes. Never wait for something only the person can do; ask instead.

Command-C copies what is selected, Command-V pastes it where the cursor is, and Command-A selects everything in the field with the cursor. That is how something is carried from one place to another, including between two applications. A letter key is only ever pressed with Command held.

Rules that decide whether a step is usable:

- Address a control by its role and title, spelled exactly as the screen lists them.
- If several controls share a role and title, the screen numbers them; say which with index.
- A control marked [out of view] is scrolled out of sight. If one in view does the same job, choose that. If the one you need is out of view, name it anyway: it is scrolled to before the step happens. Never scroll to reach one.
- On a web page, a list of choices is shown as an AXComboBox: type the name of the choice into it to pick it. The AXWebArea is the page itself, never a step's target except for a scroll. A part listed as embedded content that cannot be read belongs to another site, such as a card form: nothing inside it can be used from here, so if the task needs it, say stuck and say which part the person has to do themselves.
- Set final to true on a step that commits something that cannot be taken back: paying, placing an order, booking, publishing, posting, sending a message, deleting. The person is asked before that step runs, so mark it rather than avoid it. Mark only the step that commits it, such as the click on Send or the Enter that sends; typing the text is never final.
- Never type a password, a card number, a CVV, a PIN or a one-time code, and never solve a captcha. Those fields are listed as AXSecureTextField and cannot be typed into. When the task reaches one, stop there and say what the person needs to enter.
- Set fill to true on a step that only enters or picks a value the person gave: typing their text, choosing their date, picking their city or option from a list. Never on a step that opens, navigates, searches, submits or sends.
- AXMenuBarItem controls are the menu bar at the top of the screen (File, Edit, View and so on). Click one to open it; its rows are listed as AXMenuItem on the next screen, and that is when to click one.
- The first step must name a control that is listed on screen right now. Later steps may name controls you expect to appear.
- Describe each step in the words a person would use. That line is shown while it happens, and it is how someone watching knows what is being done.
- Do the task and nothing else. An extra click to "tidy up" is a change nobody asked for.
- Filling in a form is not sending it. Click Submit, Send, Save, Sign up or the like only when the task asks for that, not because the fields are filled.

You may also be shown what is known from earlier runs: which controls a click led to. Use it to plan confidently through screens you cannot see yet, spelling the controls exactly as it lists them. A control of the same kind as a listed one (another chat, another row in the same list) usually leads to the same sort of screen. It can be out of date, which is fine, because every step is checked.

You may also be shown how similar tasks were done here before: the exact controls, in order, that worked. Where this task needs the same thing, use the same controls, with this task's own values in place of theirs; an ellipsis stands for a value that request gave. They are not this task: leave out what this one does not need, and add what it does.

Give the shortest sequence that does the task. Stop the plan where you stop being able to predict the screen — a short honest plan costs one look at the screen to continue, where a long invented one gets thrown away.

Typing into a field, ticking a box, picking a value or setting a slider does not change the rest of the screen: every control listed now is still there afterwards. So when the fields and choices a task fills are all listed, plan every one of them in this one answer, and stop only at a step that opens, navigates or submits.

Set whole to true only when the last step of your plan is the last step of the task, so that once it has happened nothing remains. If you stopped early because you cannot see the screens that follow, whole is false. When the task is a question about where something is, the plan is whole when its last step is the thing asked about itself, and not whole when that step only leads towards it.

` + askingRule + `

Keep why to one short line. Decide first, then say it: reasoning written out in why is thrown away with the rest of the answer.

If steps are listed as already done and the screen shows every part of the task is now complete, give no steps, set done to true, and say in one line what shows it. A task with several parts, such as "open the panel and search for python", is done when the last part is, not the first. Only set done when the screen in front of you shows it; steps you have just listed do not count.

If part of the task remains and nothing on screen can do it, give no steps, set stuck to true, leave done false, and say what is missing in one line. That is a useful answer, and calling a half-finished task done is not. Stuck is for a screen that cannot do it. When what is missing is a detail only the person knows, that is a question: set ask, not stuck.`

func planSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"steps": map[string]any{"type": "array", "items": screenStepSchema("do", "target", "describe")},
			"why":   map[string]any{"type": "string"},
			"done":  map[string]any{"type": "boolean"},
			"ask":   askSchema(),
			"stuck": map[string]any{"type": "boolean"},
			"whole": map[string]any{"type": "boolean"},
		},
		"required":             []string{"steps", "why", "done", "whole"},
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

	Done     []string
	Failures []string

	Known []string

	Similar []string

	Open []string

	Rehearse bool
}

// PlanAnswer is the sequence the model expects to do the task, and its own
// account of why if there is none.
type PlanAnswer struct {
	Steps []uiact.Step
	Why   string
	Done  bool

	Question *Question

	Stuck bool

	Whole bool
}

func BuildPlanPrompt(req PlanRequest) string {
	var b strings.Builder

	b.WriteString("THE TASK\n")
	b.WriteString(req.Task)
	fmt.Fprintf(&b, "\n\nTODAY IS %s", today())

	if len(req.Done) > 0 {
		b.WriteString("\n\nALREADY DONE, SO PLAN ONLY WHAT IS LEFT\n")
		for i, d := range req.Done {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, d)
		}
	}

	b.WriteString("\n\nWHAT IS ON SCREEN NOW\n")
	b.WriteString(req.Tree.TextWithin(uiact.MaxTreeBytes))

	if len(req.Known) > 0 {
		b.WriteString("\nKNOWN FROM EARLIER RUNS IN THIS APPLICATION\n")
		for _, k := range req.Known {
			fmt.Fprintf(&b, "  - %s\n", k)
		}
	}

	writeOpenApps(&b, req.Open, req.Tree.App)

	if len(req.Similar) > 0 {
		b.WriteString("\nHOW SIMILAR TASKS WERE DONE HERE BEFORE\n")
		for _, s := range req.Similar {
			fmt.Fprintf(&b, "  - %s\n", s)
		}
	}

	if len(req.Failures) > 0 {
		b.WriteString("\nTHE PLANNED NEXT STEP DID NOT FIT THIS SCREEN\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}

	if req.Rehearse {
		b.WriteString("\nTHIS IS A REHEARSAL\nNothing will be carried out. Give every step you expect from here to the end of the task, including on screens you cannot see yet. Do not stop where you stop being sure, and do not ask a question: say the step as you would expect to take it.\n")
	}

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
		maxTokens = MostTokensForAPlan
	}

	system, err := c.systemFor(planSystemPrompt(), "plan", planSchema)
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
		Done  bool         `json:"done"`
		Ask   *askedJSON   `json:"ask"`
		Stuck bool         `json:"stuck"`
		Whole bool         `json:"whole"`
	}
	if err := json.Unmarshal([]byte(text), &answer); err != nil {
		return PlanAnswer{}, fmt.Errorf("synth: decode plan: %w (from %q)", err, truncate(text, 300))
	}

	if req.Most > 0 && len(answer.Steps) > req.Most {
		answer.Steps = answer.Steps[:req.Most]
		answer.Whole = false
	}
	if q := answer.Ask.question(); q != nil && len(answer.Steps) == 0 && !answer.Done {
		return PlanAnswer{Question: q, Why: answer.Why}, nil
	}
	if answer.Stuck && len(answer.Steps) == 0 && !answer.Done {
		return PlanAnswer{Stuck: true, Why: answer.Why}, nil
	}
	if len(answer.Steps) == 0 && !answer.Done {
		if q := questionIn(answer.Why); q != nil {
			return PlanAnswer{Question: q, Why: answer.Why}, nil
		}
	}
	return PlanAnswer{
		Steps: answer.Steps, Why: answer.Why,
		Done:  answer.Done && len(answer.Steps) == 0,
		Whole: answer.Whole && len(answer.Steps) > 0,
	}, nil
}
