package engine

import (
	"context"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type sequencedPlanner struct {
	plans    [][]uiact.Step
	requests []synth.PlanRequest
}

func (p *sequencedPlanner) WritePlan(_ context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	p.requests = append(p.requests, req)
	if len(p.requests) > len(p.plans) {
		return synth.PlanAnswer{Why: "no more plans"}, nil
	}
	return synth.PlanAnswer{Steps: p.plans[len(p.requests)-1]}, nil
}

func nowhere() uiact.Step {
	return uiact.Step{
		Do:       "click",
		Target:   uiact.Selector{Role: "AXButton", Title: "Nowhere"},
		Describe: "Click Nowhere",
	}
}

func coachPlan() *Plan {
	return &Plan{Guide: &GuidePlan{App: "Editor", Task: "say hello", Shape: "say hello", Literals: map[string]string{}}}
}

func TestACoachedTaskIsShownFromMemoryTheSecondTime(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 2}
	traces := &traceRecorder{stored: uiact.Program{App: "Editor", Steps: []uiact.Step{typeIntoMessage()}}}
	steps := &countingSteps{}
	planner := &fakePlanner{steps: []uiact.Step{typeIntoMessage()}}
	e := &Engine{UIScreen: screen, Steps: steps, Plans: planner, Traces: traces}

	work, err := e.RunGuideCounted(context.Background(), coachPlan(), func(GuideStep) {}, nil)
	if err != nil {
		t.Fatalf("RunGuideCounted: %v", err)
	}
	if steps.calls != 0 || planner.calls != 0 {
		t.Errorf("asked the model %d time(s) and planned %d time(s), want neither", steps.calls, planner.calls)
	}
	if work.Calls != 0 || work.Replayed != 1 || work.Steps != 1 {
		t.Errorf("work = %+v, want one step from last time and no calls", work)
	}
	if traces.worked != 1 {
		t.Errorf("recorded the kept steps working %d time(s), want 1", traces.worked)
	}
}

func TestAFinishedCoachedRunIsKeptApartFromDriveRuns(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 2}
	traces := &traceRecorder{}
	e := &Engine{UIScreen: screen, Traces: traces, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	if _, err := e.RunGuideCounted(context.Background(), coachPlan(), func(GuideStep) {}, nil); err != nil {
		t.Fatalf("RunGuideCounted: %v", err)
	}
	if len(traces.saved) != 1 {
		t.Fatalf("kept %d run(s), want 1", len(traces.saved))
	}
	if got := traces.saved[0].shape; got != "coach: say hello" {
		t.Errorf("kept under %q, want the coach's own key", got)
	}
}

func TestACoachedQuestionWithNoShapeIsNotKept(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 2}
	traces := &traceRecorder{}
	e := &Engine{UIScreen: screen, Traces: traces, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	if _, err := e.RunGuideCounted(context.Background(), guidePlan(), func(GuideStep) {}, nil); err != nil {
		t.Fatalf("RunGuideCounted: %v", err)
	}
	if traces.lookups != 0 || len(traces.saved) != 0 {
		t.Errorf("looked up %d and kept %d with no shape to key on", traces.lookups, len(traces.saved))
	}
}

func TestAPlanThatGoesWrongIsPlannedAgainNotWalkedStepByStep(t *testing.T) {
	planner := &sequencedPlanner{plans: [][]uiact.Step{
		{typeIntoMessage(), nowhere()},
		{pressEnter()},
	}}
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

	if len(planner.requests) != 2 {
		t.Fatalf("planned %d time(s), want 2: once up front, once when step 2 did not fit", len(planner.requests))
	}
	again := planner.requests[1]
	if len(again.Done) != 1 || len(again.Failures) == 0 {
		t.Errorf("second plan was told done=%v failures=%v, want the first step and why step 2 failed", again.Done, again.Failures)
	}
	if steps.calls != 1 {
		t.Errorf("asked for single steps %d time(s), want only the check that the new plan was all of it", steps.calls)
	}
	if receipt.Work == nil || receipt.Work.Steps != 2 || receipt.Work.Calls != 3 {
		t.Errorf("work = %+v, want 2 steps for 3 calls: two plans and the check", receipt.Work)
	}
}

func TestReplanningIsBounded(t *testing.T) {
	planner := &sequencedPlanner{plans: [][]uiact.Step{
		{nowhere()}, {nowhere()}, {nowhere()}, {nowhere()},
	}}
	steps := &countingSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}}}
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps, Plans: planner}

	plan, _ := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if n := len(planner.requests); n > 1+mostReplans {
		t.Errorf("planned %d time(s), want at most %d", n, 1+mostReplans)
	}
}

func quickSettle(t *testing.T, poll, quiet time.Duration) {
	t.Helper()
	settle, p, q := settleBeforeJudging, settlePoll, quietFor
	settleBeforeJudging, settlePoll, quietFor = 900*time.Millisecond, poll, quiet
	t.Cleanup(func() { settleBeforeJudging, settlePoll, quietFor = settle, p, q })
}

func TestAQuietWindowIsSettledWithoutWaitingOutTheWholeAllowance(t *testing.T) {
	quickSettle(t, 5*time.Millisecond, 30*time.Millisecond)

	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}}
	start := time.Now()
	e.settledView(context.Background(), "Editor", editorTree(), nil)
	if took := time.Since(start); took > 400*time.Millisecond {
		t.Errorf("settled in %v; a window that never moved should not wait out the full allowance", took)
	}
}

func TestAWindowThatKeepsMovingIsGivenTimeToStop(t *testing.T) {
	quickSettle(t, 5*time.Millisecond, 40*time.Millisecond)

	changed := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for i := 0; i < 15; i++ {
			select {
			case changed <- struct{}{}:
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}}
	start := time.Now()
	e.settledView(ctx, "Editor", editorTree(), changed)
	took := time.Since(start)
	if took < 150*time.Millisecond {
		t.Errorf("settled after %v while the window was still moving", took)
	}
	if took > 700*time.Millisecond {
		t.Errorf("settled after %v; it went quiet long before that", took)
	}
}
