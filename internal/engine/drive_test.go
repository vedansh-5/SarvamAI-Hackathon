package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

const placedTextArea = `{"ok":true,"value":{"app":"Editor","window":"Editor","elements":[
  {"role":"AXTextArea","title":"Message","enabled":true,"position":[100,200],"size":[80,40],"path":[0,1]}
]}}`

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
	r := &countingRunner{tree: placedTextArea}
	e := &Engine{UIRunner: r, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Done: true, Why: "already finished"}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if r.runs != 0 {
		t.Errorf("touched the screen %d time(s) for a task already done", r.runs)
	}
}

func TestDriveRunsAStepThenStops(t *testing.T) {
	r := &countingRunner{tree: placedTextArea}
	e := &Engine{UIRunner: r, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if r.runs != 1 {
		t.Errorf("ran %d step(s), want 1", r.runs)
	}
	if r.captures < 2 {
		t.Errorf("read the screen %d time(s); it must be read again after acting", r.captures)
	}
}

func TestDriveStopsAtTheBudget(t *testing.T) {
	r := &countingRunner{tree: placedTextArea}
	w := &alwaysStep{step: typeIntoMessage()}
	e := &Engine{UIRunner: r, Steps: w}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("a task that never finishes was reported as success")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("err = %v", err)
	}
	if r.runs != MaxDriveSteps {
		t.Errorf("ran %d steps, want the budget of %d", r.runs, MaxDriveSteps)
	}
}

func TestDriveAsksAgainWhenAStepDoesNotFitTheScreen(t *testing.T) {
	r := &countingRunner{tree: placedTextArea}
	missing := uiact.Step{
		Do:       "click",
		Target:   uiact.Selector{Role: "AXButton", Title: "Publish"},
		Describe: "Click Publish",
	}
	e := &Engine{UIRunner: r, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: missing}, {Step: typeIntoMessage()}},
	}}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if r.runs != 1 {
		t.Errorf("ran %d step(s); the rejected one must not reach the screen", r.runs)
	}
}
