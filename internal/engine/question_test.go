package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type recordingSteps struct {
	answers  []synth.StepAnswer
	requests []synth.StepRequest
}

func (r *recordingSteps) WriteStep(_ context.Context, req synth.StepRequest) (synth.StepAnswer, error) {
	r.requests = append(r.requests, req)
	if len(r.requests) <= len(r.answers) {
		return r.answers[len(r.requests)-1], nil
	}
	return synth.StepAnswer{Done: true, Why: "nothing left"}, nil
}

func whichFlight() *synth.Question {
	return &synth.Question{Ask: "Which flight?", Options: []string{"IndiGo 7:10", "Air India 9:45"}}
}

func TestAChoiceThatIsThePersonsIsAskedAndTheAnswerCarriedForward(t *testing.T) {
	steps := &recordingSteps{answers: []synth.StepAnswer{
		{Question: whichFlight()},
		{Step: typeIntoMessage()},
		{Done: true, Why: "picked"},
	}}
	var asked []string
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps}
	e.Question = func(_ context.Context, q string, options []string) (string, error) {
		asked = append(asked, q+" "+strings.Join(options, "|"))
		return "IndiGo 7:10", nil
	}

	receipt, err := e.runDrive(context.Background(), drivePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "Which flight?") || !strings.Contains(asked[0], "Air India 9:45") {
		t.Fatalf("asked %v, want the one question with its choices", asked)
	}
	if len(steps.requests) < 2 || len(steps.requests[1].Done) != 1 || !strings.Contains(steps.requests[1].Done[0], "IndiGo 7:10") {
		t.Errorf("the next ask was told %v, want the answer in what has been done", steps.requests[1].Done)
	}
	if receipt.Work == nil || receipt.Work.Steps != 1 {
		t.Errorf("work = %+v; a question is not a step", receipt.Work)
	}
}

func TestARunThatNeededAnAnswerIsNotKeptForReplay(t *testing.T) {
	traces := &traceRecorder{}
	e := &Engine{UIScreen: &movingScreen{}, Traces: traces, Steps: &recordingSteps{answers: []synth.StepAnswer{
		{Question: whichFlight()},
		{Step: typeIntoMessage()},
		{Done: true},
	}}}
	e.Question = func(context.Context, string, []string) (string, error) { return "IndiGo 7:10", nil }

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d run(s) whose steps depended on an answer; next time may want another", len(traces.saved))
	}
}

func TestWithNobodyToAskTheRunStopsWithTheQuestion(t *testing.T) {
	e := &Engine{UIScreen: &movingScreen{}, Steps: &recordingSteps{answers: []synth.StepAnswer{{Question: whichFlight()}}}}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil || !strings.Contains(err.Error(), "Which flight?") {
		t.Fatalf("err = %v, want the run stopped and the question said", err)
	}
}

func TestAModelThatOnlyAsksIsStopped(t *testing.T) {
	always := &recordingSteps{}
	for i := 0; i < 20; i++ {
		always.answers = append(always.answers, synth.StepAnswer{Question: whichFlight()})
	}
	n := 0
	e := &Engine{UIScreen: &movingScreen{}, Steps: always}
	e.Question = func(context.Context, string, []string) (string, error) { n++; return "either", nil }

	if _, err := e.runDrive(context.Background(), drivePlan()); err == nil {
		t.Fatal("a run made of nothing but questions was allowed to go on")
	}
	if n != mostQuestions {
		t.Errorf("asked %d time(s), want it stopped at %d", n, mostQuestions)
	}
}

func TestThePlannerCanAskToo(t *testing.T) {
	planner := &questionPlanner{}
	asked := 0
	e := &Engine{UIScreen: &movingScreen{}, Steps: &countingSteps{}, Plans: planner}
	e.Question = func(context.Context, string, []string) (string, error) { asked++; return "IndiGo 7:10", nil }

	plan, err := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if asked != 1 {
		t.Errorf("asked %d time(s), want 1", asked)
	}
}

type questionPlanner struct{ calls int }

func (p *questionPlanner) WritePlan(_ context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	p.calls++
	switch p.calls {
	case 1:
		return synth.PlanAnswer{Steps: []uiact.Step{typeIntoMessage()}}, nil
	case 2:
		return synth.PlanAnswer{Question: whichFlight()}, nil
	default:
		return synth.PlanAnswer{Done: true, Why: "done"}, nil
	}
}
