package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type breaksOnce struct {
	movingScreen
	failAt int
}

func (b *breaksOnce) Act(ctx context.Context, p uiact.Program, t uiact.Tree) error {
	b.acts++
	if b.acts == b.failAt {
		return errors.New("the window went away")
	}
	return uiact.Validate(p, t)
}

func TestARunThatBreaksPartwaySaysHowFarItGotAndCanCarryOn(t *testing.T) {
	quickLook(t, 0)
	screen := &breaksOnce{failAt: 2}
	steps := &recordingSteps{answers: []synth.StepAnswer{
		{Step: typeIntoMessage()},
		{Step: pressEnter()},
		{Step: pressEnter()},
		{Done: true, Why: "sent"},
	}}
	traces := &traceRecorder{}
	e := &Engine{UIScreen: screen, Steps: steps, Traces: traces}

	plan := tracePlan()
	_, err := e.runDrive(context.Background(), plan)

	var broke *BrokeOffError
	if !errors.As(err, &broke) {
		t.Fatalf("err = %v, want to be told it stopped partway", err)
	}
	if len(broke.Did) != 1 || !strings.Contains(broke.Did[0], "Type hello into Message") {
		t.Fatalf("did = %v, want the one step that was done", broke.Did)
	}
	if !plan.Drive.Resumed || len(plan.Drive.Done) != 1 {
		t.Fatalf("plan = %+v, want it to remember where it got to", plan.Drive)
	}

	asksBefore := len(steps.requests)
	receipt, err := e.runDrive(context.Background(), plan)
	if err != nil {
		t.Fatalf("carrying on: %v", err)
	}
	next := steps.requests[asksBefore]
	if len(next.Done) != 1 || !strings.Contains(next.Done[0], "Type hello into Message") {
		t.Errorf("the first ask after carrying on was told %v, want the step already done", next.Done)
	}
	if receipt.Work == nil || receipt.Work.Steps != 1 {
		t.Errorf("work = %+v, want only the step that was left", receipt.Work)
	}
	if screen.acts != 3 {
		t.Errorf("acted %d time(s) in all; the first step must not be done twice", screen.acts)
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d run(s); the second half of a task is not the task", len(traces.saved))
	}
}

func TestARunThatBreaksBeforeDoingAnythingIsJustAFailure(t *testing.T) {
	quickLook(t, 0)
	e := &Engine{UIScreen: &breaksOnce{failAt: 1}, Steps: &recordingSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}}}}

	plan := drivePlan()
	_, err := e.runDrive(context.Background(), plan)
	var broke *BrokeOffError
	if err == nil || errors.As(err, &broke) {
		t.Fatalf("err = %v, want a plain failure; there is nothing to carry on from", err)
	}
}

func TestBeingStoppedIsNotBreaking(t *testing.T) {
	quickLook(t, 0)
	screen := &breaksOnce{failAt: 2}
	ctx, cancel := context.WithCancel(context.Background())
	steps := &recordingSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Step: pressEnter()}}}
	e := &Engine{UIScreen: screen, Steps: steps}
	e.ShowStep = func(g GuideStep) {
		if g.Number == 2 {
			cancel()
		}
	}

	_, err := e.runDrive(ctx, drivePlan())
	var broke *BrokeOffError
	if errors.As(err, &broke) {
		t.Fatalf("a run the person stopped was offered to carry on: %v", err)
	}
}
