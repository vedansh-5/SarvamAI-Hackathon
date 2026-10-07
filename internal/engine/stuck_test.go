package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type stuckPlanner struct{ calls int }

func (p *stuckPlanner) WritePlan(_ context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	p.calls++
	if len(req.Done) == 0 {
		return synth.PlanAnswer{Steps: []uiact.Step{clickMessage()}}, nil
	}
	return synth.PlanAnswer{Stuck: true, Why: "there is no search box on screen to type python into"}, nil
}

func TestAHalfDoneTaskIsReportedAsStuckNotAsDone(t *testing.T) {
	traces := &traceRecorder{}
	e := &Engine{UIScreen: &movingScreen{}, Plans: &stuckPlanner{}, Steps: &countingSteps{}, Traces: traces}

	plan, err := e.driveOn(context.Background(), "Editor", "open the panel and search for python", "open the panel and search for python", nil)
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	_, err = e.runDrive(context.Background(), plan)

	if err == nil {
		t.Fatal("a task with a part left undone was reported as finished")
	}
	if !strings.Contains(err.Error(), "no search box") {
		t.Errorf("err = %v, want what is missing said", err)
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d run(s); half a task replayed is half a task every time", len(traces.saved))
	}
}

func TestTheStepWriterCanSayItIsStuckToo(t *testing.T) {
	e := &Engine{UIScreen: &movingScreen{}, Steps: &scriptedSteps{answers: []synth.StepAnswer{
		{Step: clickMessage()},
		{Stuck: true, Why: "the field it needs is not here"},
	}}}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil || !strings.Contains(err.Error(), "the field it needs is not here") {
		t.Fatalf("err = %v, want the run stopped with the reason", err)
	}
}

func TestBeingStuckBeforeDoingAnythingIsANothingToDoNotACrash(t *testing.T) {
	e := &Engine{UIScreen: &movingScreen{}, Steps: &scriptedSteps{answers: []synth.StepAnswer{
		{Stuck: true, Why: "this window has nothing to do with it"},
	}}}

	receipt, err := e.runDrive(context.Background(), drivePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if receipt.Verified || receipt.Work == nil || receipt.Work.Steps != 0 {
		t.Errorf("receipt = %+v, want nothing done and nothing claimed", receipt)
	}
}

type stuckAtOnce struct{}

func (stuckAtOnce) WritePlan(context.Context, synth.PlanRequest) (synth.PlanAnswer, error) {
	return synth.PlanAnswer{Stuck: true, Why: "nothing here answers to qzx vfrt"}, nil
}

func TestARequestThePlannerCannotStartIsNotOfferedForConfirmation(t *testing.T) {
	e := &Engine{UIScreen: &movingScreen{}, Plans: stuckAtOnce{}, Steps: &countingSteps{}}

	plan, err := e.driveOn(context.Background(), "Editor", "qzx vfrt plonk wibble", "qzx vfrt plonk wibble", nil)

	var cannot *synth.UnwritableError
	if !errors.As(err, &cannot) {
		t.Fatalf("plan = %+v, err = %v, want it turned down before anything is offered", plan, err)
	}
	if !strings.Contains(cannot.Why, "qzx vfrt") {
		t.Errorf("why = %q, want the planner's own reason", cannot.Why)
	}
}
