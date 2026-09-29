package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type fakePlanner struct {
	steps []uiact.Step
	why   string
	err   error
	calls int
}

func (f *fakePlanner) WritePlan(context.Context, synth.PlanRequest) (synth.PlanAnswer, error) {
	f.calls++
	if f.err != nil {
		return synth.PlanAnswer{}, f.err
	}
	return synth.PlanAnswer{Steps: f.steps, Why: f.why}, nil
}

// The claim of the whole change. Two steps used to cost three calls — one each,
// and one more to hear the task was done. Planned up front they cost two: the
// plan, and the one question of whether that was all of it.
func TestAPlannedRunCostsTheSameWhateverItsLength(t *testing.T) {
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage(), pressEnter()}}
	steps := &countingSteps{}
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps, Plans: planner}

	plan, err := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	receipt, err := e.runDrive(context.Background(), plan)
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if planner.calls != 1 {
		t.Errorf("planned %d time(s), want 1", planner.calls)
	}
	if steps.calls != 1 {
		t.Errorf("asked about steps %d time(s), want 1 — only whether the plan was all of it", steps.calls)
	}
	if receipt.Work == nil || receipt.Work.Calls != 2 {
		t.Errorf("work = %+v, want 2 calls in all: the plan and the check", receipt.Work)
	}
	if receipt.Work != nil && receipt.Work.Steps != 2 {
		t.Errorf("ran %d step(s), want the 2 planned", receipt.Work.Steps)
	}
}

// A guessed plan running out is not the task finishing. The model saw one window
// when it wrote the plan and a different one now, so it is asked — and if it has
// more to do, the run carries on.
func TestRunningOutOfAPlanAsksWhetherThereIsMore(t *testing.T) {
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	steps := &countingSteps{answers: []synth.StepAnswer{
		{Step: pressEnter()},
		{Done: true, Why: "sent"},
	}}
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps, Plans: planner}

	plan, _ := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	receipt, err := e.runDrive(context.Background(), plan)
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if receipt.Work == nil || receipt.Work.Steps != 2 {
		t.Errorf("work = %+v, want the planned step and the one the model added", receipt.Work)
	}
}

// The confirmation shows the plan, because it can. "Up to 12 steps" is what you
// say before anything is planned, and saying it afterwards hides what somebody is
// agreeing to.
func TestAPlannedRunIsConfirmedStepByStep(t *testing.T) {
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	e := &Engine{UIScreen: &movingScreen{}, Steps: &countingSteps{}, Plans: planner}

	plan, err := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	if plan.Drive.Kept {
		t.Error("a plan written this run is marked as kept from last time")
	}
	if !strings.Contains(plan.Drive.Preview, "Type hello into Message") {
		t.Errorf("preview = %q, want it to name the planned step", plan.Drive.Preview)
	}
	if strings.Contains(plan.Drive.Preview, "last time") {
		t.Error("preview says these steps worked last time; they were just planned")
	}
}

// A plan is an optimisation. A planner that fails must leave the run exactly as it
// was before planning existed.
func TestAFailedPlanFallsBackToStepByStep(t *testing.T) {
	planner := &fakePlanner{err: errors.New("model unavailable")}
	e := &Engine{UIScreen: &movingScreen{}, Steps: &countingSteps{}, Plans: planner}

	plan, err := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	if err != nil {
		t.Fatalf("driveOn failed because the plan did: %v", err)
	}
	if len(plan.Drive.Replay) != 0 {
		t.Errorf("replay = %d step(s) from a plan that failed", len(plan.Drive.Replay))
	}
}

// Remembered steps are cheaper than a plan, so a plan is not asked for when there
// are any.
func TestNoPlanIsWrittenWhenStepsWereKept(t *testing.T) {
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	traces := &traceRecorder{stored: uiact.Program{App: "Editor", Steps: []uiact.Step{typeIntoMessage()}}}
	e := &Engine{UIScreen: &movingScreen{}, Steps: &countingSteps{}, Plans: planner, Traces: traces}

	plan, err := e.driveOn(context.Background(), "Editor", "say hello", "say hello", map[string]string{})
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	if planner.calls != 0 {
		t.Errorf("planned %d time(s) though the steps were already kept", planner.calls)
	}
	if !plan.Drive.Kept {
		t.Error("kept steps not marked as kept")
	}
}

// Where the wait was felt. Coached steps used to be worked out one at a time,
// after the person had done the one before, so every step began with a pause for
// the model. Planned up front, a coached step the plan already has costs no call
// at all — the next one is ready the moment the person is.
func TestACoachedRunDoesNotAskForAStepItPlanned(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 2}

	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	steps := &countingSteps{answers: []synth.StepAnswer{{Done: true, Why: "that is all"}}}
	e := &Engine{UIScreen: screen, Steps: steps, Plans: planner}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
	}, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	if planner.calls != 1 {
		t.Errorf("planned %d time(s), want 1", planner.calls)
	}
	if len(shown) == 0 || !strings.Contains(shown[0].Label, "Message") {
		t.Fatalf("shown = %+v, want the planned step pointed at", shown)
	}
	if steps.calls != 1 {
		t.Errorf("asked for steps %d time(s), want only the one check that the plan was all of it", steps.calls)
	}
}
