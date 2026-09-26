package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// countingSteps answers with a scripted list and records how often it was asked.
type countingSteps struct {
	answers []synth.StepAnswer
	calls   int
}

func (c *countingSteps) WriteStep(context.Context, synth.StepRequest) (synth.StepAnswer, error) {
	c.calls++
	if c.calls <= len(c.answers) {
		return c.answers[c.calls-1], nil
	}
	return synth.StepAnswer{Done: true, Why: "nothing left to do"}, nil
}

// Validate resolves the target of every step, a press included, so this names the
// control the key goes to rather than leaving it empty.
func pressEnter() uiact.Step {
	return uiact.Step{
		Do:       "press",
		Key:      "Enter",
		Target:   uiact.Selector{Role: "AXTextArea", Title: "Message"},
		Describe: "Press Enter",
	}
}

func replayPlanWith(steps ...uiact.Step) *Plan {
	plan := tracePlan()
	plan.Drive.Replay = steps
	return plan
}

// The whole claim of the cache. A run whose steps are all known asks nothing at
// all — not even to hear that the task is finished, since a trace is only kept for
// a run the model called finished, so its length is that answer.
func TestAFullyKnownRunAsksTheModelNothing(t *testing.T) {
	steps := &countingSteps{}
	traces := &traceRecorder{}
	e := &Engine{UIScreen: &movingScreen{}, Traces: traces, Steps: steps}

	receipt, err := e.runDrive(context.Background(),
		replayPlanWith(typeIntoMessage(), pressEnter()))
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if steps.calls != 0 {
		t.Errorf("asked the model %d time(s) for a run whose steps were all known", steps.calls)
	}
	if !receipt.Verified {
		t.Error("receipt not verified; a replayed step is verified by the same evidence as a live one")
	}
	if traces.worked != 1 {
		t.Errorf("recorded the steps as having worked %d time(s), want 1", traces.worked)
	}
	if traces.diverged != 0 {
		t.Errorf("recorded %d divergence(s) on a clean replay", traces.diverged)
	}
}

// Divergence resumes rather than restarting: the steps that already ran are kept,
// and the model is asked only about the rest.
func TestDivergenceAsksOnlyAboutTheRestOfTheRun(t *testing.T) {
	steps := &countingSteps{answers: []synth.StepAnswer{
		{Step: pressEnter()},
		{Done: true, Why: "done"},
	}}
	traces := &traceRecorder{}
	e := &Engine{UIScreen: &movingScreen{}, Traces: traces, Steps: steps}

	// Step 1 fits the window; step 2 names a control that is not there.
	missing := uiact.Step{
		Do:     "click",
		Target: uiact.Selector{Role: "AXButton", Title: "Nothing Like This"},
	}

	if _, err := e.runDrive(context.Background(),
		replayPlanWith(typeIntoMessage(), missing)); err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if steps.calls == 0 {
		t.Fatal("never asked the model, though a step stopped fitting")
	}
	if steps.calls > 2 {
		t.Errorf("asked the model %d time(s); only the steps from the divergence onwards "+
			"should cost anything", steps.calls)
	}
	if traces.diverged != 1 {
		t.Errorf("recorded %d divergence(s), want 1", traces.diverged)
	}
	if traces.worked != 0 {
		t.Errorf("recorded the steps as working %d time(s) after they went off", traces.worked)
	}
}

// A repaired run is the newest thing known to work, so it replaces what was
// stored — including re-creating an entry that the divergence just dropped.
func TestARepairedRunReplacesTheStoredSteps(t *testing.T) {
	steps := &countingSteps{answers: []synth.StepAnswer{
		{Step: pressEnter()},
		{Done: true, Why: "done"},
	}}
	traces := &traceRecorder{dropAfter: true}
	e := &Engine{UIScreen: &movingScreen{}, Traces: traces, Steps: steps, Model: "m"}

	missing := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Gone"}}

	if _, err := e.runDrive(context.Background(), replayPlanWith(missing)); err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if len(traces.saved) != 1 {
		t.Fatalf("saved %d trace(s) after repairing the run, want 1", len(traces.saved))
	}
	if n := len(traces.saved[0].program.Steps); n != 1 {
		t.Errorf("saved %d step(s), want the 1 that actually worked", n)
	}
}

// A replayed step that ran but moved nothing is not a success. Nothing changed, so
// nothing is spoiled by asking the model about that same step instead.
func TestAReplayedStepThatChangedNothingDiverges(t *testing.T) {
	steps := &countingSteps{answers: []synth.StepAnswer{
		{Done: true, Why: "it was already done"},
	}}
	traces := &traceRecorder{}
	// The plain fake never changes the window, so nothing a step does shows up.
	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}, Traces: traces, Steps: steps}

	if _, err := e.runDrive(context.Background(), replayPlanWith(typeIntoMessage())); err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if traces.diverged != 1 {
		t.Errorf("recorded %d divergence(s) for a step that changed nothing, want 1", traces.diverged)
	}
	if steps.calls == 0 {
		t.Error("never asked the model after the replayed step did nothing")
	}
}

// The confirmation for a known run names the steps, because it can. "Up to 12
// steps" is honest about a run nobody has planned; saying it about one whose steps
// are in hand would withhold what somebody is being asked to agree to.
func TestAKnownRunIsPreviewedStepByStep(t *testing.T) {
	traces := &traceRecorder{
		stored: uiact.Program{App: "Editor", Steps: []uiact.Step{typeIntoMessage()}},
	}
	e := &Engine{
		UIScreen: &fakeScreen{tree: editorTree()},
		Traces:   traces,
		Steps:    &countingSteps{},
	}

	plan, err := e.driveOn(context.Background(), "Editor", "type hello into the message",
		"type hello into the message", map[string]string{})
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}

	if len(plan.Drive.Replay) != 1 {
		t.Fatalf("plan carries %d replay step(s), want 1", len(plan.Drive.Replay))
	}
	if want := "Type hello into Message"; !strings.Contains(plan.Drive.Preview, want) {
		t.Errorf("preview = %q, want it to name the step %q", plan.Drive.Preview, want)
	}
	if strings.Contains(plan.Drive.Preview, "Up to") {
		t.Error("preview still says 'up to N steps' though the steps are known")
	}
}

// A stored sequence is filled with this request's values, not last request's.
func TestAStoredRunIsFilledWithThisRequestsValues(t *testing.T) {
	templated := typeIntoMessage()
	templated.Text = "{s0}"
	templated.Describe = ""

	traces := &traceRecorder{stored: uiact.Program{App: "Editor", Steps: []uiact.Step{templated}}}
	e := &Engine{
		UIScreen: &fakeScreen{tree: editorTree()},
		Traces:   traces,
		Steps:    &countingSteps{},
	}

	plan, err := e.driveOn(context.Background(), "Editor", `type "goodbye" into the message`,
		"type {s0} into the message", map[string]string{"s0": "goodbye"})
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}

	if got := plan.Drive.Replay[0].Text; got != "goodbye" {
		t.Errorf("replay types %q, want this request's value", got)
	}
}

// A build with no trace store drives exactly as it did before.
func TestDrivingWithoutATraceStoreLooksUpNothing(t *testing.T) {
	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}, Steps: &countingSteps{}}

	plan, err := e.driveOn(context.Background(), "Editor", "do a thing", "do a thing", nil)
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	if len(plan.Drive.Replay) != 0 {
		t.Errorf("plan carries %d replay step(s) with no store", len(plan.Drive.Replay))
	}
}
