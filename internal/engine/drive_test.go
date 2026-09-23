package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type scriptedSteps struct {
	answers []synth.StepAnswer
	calls   int
}

func (s *scriptedSteps) WriteStep(context.Context, synth.StepRequest) (synth.StepAnswer, error) {
	if s.calls < len(s.answers) {
		a := s.answers[s.calls]
		s.calls++
		return a, nil
	}
	return synth.StepAnswer{Done: true, Why: "nothing left to do"}, nil
}

type alwaysStep struct {
	step  uiact.Step
	calls int
}

func (a *alwaysStep) WriteStep(context.Context, synth.StepRequest) (synth.StepAnswer, error) {
	a.calls++
	return synth.StepAnswer{Step: a.step}, nil
}

func typeIntoMessage() uiact.Step {
	return uiact.Step{
		Do:       "type",
		Target:   uiact.Selector{Role: "AXTextArea", Title: "Message"},
		Text:     "hello",
		Describe: "Type hello into Message",
	}
}

func drivePlan() *Plan {
	return &Plan{Drive: &DrivePlan{App: "Editor", Task: "say hello"}}
}

func TestDriveStopsWhenTheTaskIsDone(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: f, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Done: true, Why: "already finished"}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if f.acts != 0 {
		t.Errorf("touched the screen %d time(s) for a task already done", f.acts)
	}
}

func TestDriveRunsAStepThenStops(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: f, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if f.acts != 1 {
		t.Errorf("ran %d step(s), want 1", f.acts)
	}
	if f.captures < 2 {
		t.Errorf("read the screen %d time(s); it must be read again after acting", f.captures)
	}
}

func TestDriveStopsAtTheBudget(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	w := &alwaysStep{step: typeIntoMessage()}
	e := &Engine{UIScreen: f, Steps: w}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("a task that never finishes was reported as success")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("err = %v", err)
	}
	if f.acts != MaxDriveSteps {
		t.Errorf("ran %d steps, want the budget of %d", f.acts, MaxDriveSteps)
	}
}

func TestDriveAsksAgainWhenAStepDoesNotFitTheScreen(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	missing := uiact.Step{
		Do:       "click",
		Target:   uiact.Selector{Role: "AXButton", Title: "Publish"},
		Describe: "Click Publish",
	}
	e := &Engine{UIScreen: f, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: missing}, {Step: typeIntoMessage()}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if f.acts != 1 {
		t.Errorf("ran %d step(s); the rejected one must not reach the screen", f.acts)
	}
}

type failingActScreen struct {
	fakeScreen
	actFailures int
}

func (s *failingActScreen) Act(ctx context.Context, p uiact.Program, t uiact.Tree) error {
	if s.actFailures > 0 {
		s.actFailures--
		return &uiact.InvalidError{App: t.App, Reasons: []string{"click missed target control"}}
	}
	return s.fakeScreen.Act(ctx, p, t)
}

type recordedSteps struct {
	answers  []synth.StepAnswer
	requests []synth.StepRequest
}

func (r *recordedSteps) WriteStep(_ context.Context, req synth.StepRequest) (synth.StepAnswer, error) {
	r.requests = append(r.requests, req)
	if len(r.requests) <= len(r.answers) {
		return r.answers[len(r.requests)-1], nil
	}
	return synth.StepAnswer{Done: true}, nil
}

func TestDriveVerifiesTheReceiptAgainstTheWindow(t *testing.T) {
	typed := editorTree()
	typed.Elements[0].Value = "hello"

	f := &fakeScreen{tree: editorTree(), afterwards: &typed}
	e := &Engine{UIScreen: f, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}},
	}}

	receipt, err := e.runDrive(context.Background(), drivePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if !receipt.Verified {
		t.Error("the step landed in the window and the receipt did not say so")
	}
}

// The model declaring the task finished is the claim under test, so it cannot
// be the evidence for it. A window that never moved is an unverified receipt,
// however confident the model was.
func TestDriveWillNotCallATaskVerifiedWhenTheWindowNeverMoved(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: f, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}},
	}}

	receipt, err := e.runDrive(context.Background(), drivePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if receipt.Verified {
		t.Error("nothing in the window changed and the receipt claimed it was verified")
	}
}

func TestDriveRetriesWhenStepExecutionFails(t *testing.T) {
	typed := editorTree()
	typed.Elements[0].Value = "hello"

	f := &failingActScreen{
		fakeScreen:  fakeScreen{tree: editorTree(), afterwards: &typed},
		actFailures: 1,
	}
	steps := &recordedSteps{
		answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Step: typeIntoMessage()},
		},
	}
	e := &Engine{UIScreen: f, Steps: steps}

	receipt, err := e.runDrive(context.Background(), drivePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if !receipt.Verified {
		t.Error("the recovered step landed and the receipt did not say so")
	}
	if len(steps.requests) < 2 {
		t.Fatalf("model called %d times, want >= 2", len(steps.requests))
	}
	if len(steps.requests[1].Failures) == 0 {
		t.Fatal("attempt 2 was not told why attempt 1 failed")
	}
	if !strings.Contains(steps.requests[1].Failures[0], "click missed") {
		t.Errorf("failure reason = %q, want mention of missed click", steps.requests[1].Failures[0])
	}
}

// A window withdrawn between steps is brought back, once, by the step that needs
// it — not by the read, which the coach runs twice a second while waiting.
type withdrawnWindowScreen struct {
	fakeScreen
	withheld int
}

func (s *withdrawnWindowScreen) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	if s.withheld > 0 {
		s.withheld--
		return uiact.Tree{}, uiact.ErrNoWindow
	}
	return s.fakeScreen.Capture(ctx, app)
}

func TestDriveBringsBackAWindowThatStoppedPublishing(t *testing.T) {
	typed := editorTree()
	typed.Elements[0].Value = "hello"

	f := &withdrawnWindowScreen{
		fakeScreen: fakeScreen{tree: editorTree(), afterwards: &typed},
		withheld:   1,
	}
	e := &Engine{UIScreen: f, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(f.raised) != 1 {
		t.Errorf("brought the application forward %d time(s), want 1", len(f.raised))
	}
	if f.acts != 1 {
		t.Errorf("ran %d step(s), want 1 once the window came back", f.acts)
	}
}

type flakyThenGoodSteps struct {
	calls int
}

func (f *flakyThenGoodSteps) WriteStep(context.Context, synth.StepRequest) (synth.StepAnswer, error) {
	f.calls++
	switch f.calls {
	case 1:
		return synth.StepAnswer{}, errors.New("synth: neither a step nor a reason to stop")
	case 2:
		return synth.StepAnswer{Step: typeIntoMessage()}, nil
	default:
		return synth.StepAnswer{Done: true, Why: "that is all"}, nil
	}
}

// One answer with nothing in it used to end the run and show the person a
// failure. It is worth one more ask.
func TestDriveAsksAgainWhenTheModelAnswersWithNothing(t *testing.T) {
	typed := editorTree()
	typed.Elements[0].Value = "hello"

	f := &fakeScreen{tree: editorTree(), afterwards: &typed}
	writer := &flakyThenGoodSteps{}
	e := &Engine{UIScreen: f, Steps: writer}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if f.acts != 1 {
		t.Errorf("ran %d step(s), want 1 after the empty answer was asked past", f.acts)
	}
}

type silentSteps struct {
	calls int
}

func (s *silentSteps) WriteStep(context.Context, synth.StepRequest) (synth.StepAnswer, error) {
	s.calls++
	return synth.StepAnswer{}, fmt.Errorf("synth: test/model did not answer in 45s: %w", synth.ErrNoAnswer)
}

// An empty answer is worth one more ask; a model that never answered is not.
// The re-ask waits out the whole timeout again and has nothing new to offer,
// which is how one stalled provider became six minutes of "working out step 1".
func TestDriveDoesNotAskASilentModelAgain(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	writer := &silentSteps{}
	e := &Engine{UIScreen: f, Steps: writer}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, synth.ErrNoAnswer) {
		t.Errorf("the silence did not survive to the caller: %v", err)
	}
	if writer.calls != 1 {
		t.Errorf("asked %d times, want 1", writer.calls)
	}
}
