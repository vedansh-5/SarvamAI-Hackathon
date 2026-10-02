package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type wholePlanner struct {
	steps    []uiact.Step
	whole    bool
	requests []synth.PlanRequest
}

func (p *wholePlanner) WritePlan(_ context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	p.requests = append(p.requests, req)
	if len(req.Done) > 0 {
		return synth.PlanAnswer{Done: true, Why: "the message is there"}, nil
	}
	return synth.PlanAnswer{Steps: p.steps, Whole: p.whole}, nil
}

func sendIt() uiact.Step {
	step := pressEnter()
	step.Final = true
	step.Describe = "Press Enter to send"
	return step
}

func runWholePlan(t *testing.T, p *wholePlanner) *countingSteps {
	t.Helper()
	steps := &countingSteps{}
	e := &Engine{UIScreen: &movingScreen{}, Steps: steps, Plans: p}

	plan, err := e.driveOn(context.Background(), "Editor", "say hello", "say hello", nil)
	if err != nil {
		t.Fatalf("driveOn: %v", err)
	}
	receipt, err := e.runDrive(context.Background(), plan)
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if receipt.Work == nil || receipt.Work.Steps != len(p.steps) {
		t.Fatalf("work = %+v, want every planned step carried out", receipt.Work)
	}
	return steps
}

func TestAWholePlanEndingInACommitIsNotAskedAbout(t *testing.T) {
	p := &wholePlanner{steps: []uiact.Step{typeIntoMessage(), sendIt()}, whole: true}
	steps := runWholePlan(t, p)

	if len(p.requests) != 1 {
		t.Errorf("planned %d time(s), want 1: the closing check is spent on nothing here", len(p.requests))
	}
	if steps.calls != 0 {
		t.Errorf("asked for single steps %d time(s), want none", steps.calls)
	}
}

func TestAPlanThatIsNotWholeIsStillCheckedAtTheEnd(t *testing.T) {
	p := &wholePlanner{steps: []uiact.Step{typeIntoMessage(), sendIt()}, whole: false}
	runWholePlan(t, p)

	if len(p.requests) != 2 {
		t.Errorf("planned %d time(s), want 2: nothing said this plan was all of it", len(p.requests))
	}
}

func TestAWholePlanThatCommitsNothingIsStillCheckedAtTheEnd(t *testing.T) {
	p := &wholePlanner{steps: []uiact.Step{typeIntoMessage()}, whole: true}
	runWholePlan(t, p)

	if len(p.requests) != 2 {
		t.Errorf("planned %d time(s), want 2: typing proves nothing about the task being finished", len(p.requests))
	}
}

func TestAWhereQuestionIsFinishedByPointing(t *testing.T) {
	quickGuide(t)

	run := func(question string) *wholePlanner {
		moved := editorTree()
		moved.Elements[0].Focused = true
		screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 2}
		p := &wholePlanner{steps: []uiact.Step{clickMessage()}, whole: true}
		e := &Engine{UIScreen: screen, Steps: &countingSteps{}, Plans: p}

		plan := &Plan{Guide: &GuidePlan{App: "Editor", Task: question}}
		if _, err := e.RunGuideCounted(context.Background(), plan, func(GuideStep) {}, nil); err != nil {
			t.Fatalf("RunGuideCounted(%q): %v", question, err)
		}
		return p
	}

	if p := run("where is the message box"); len(p.requests) != 1 {
		t.Errorf("planned %d time(s) for a where question, want 1: pointing at it is the answer", len(p.requests))
	}
	if p := run("how do I write a message"); len(p.requests) != 2 {
		t.Errorf("planned %d time(s) for a how question, want 2: only the screen says whether it is done", len(p.requests))
	}
}

func TestWhereQuestionsAreToldFromHowQuestions(t *testing.T) {
	for _, q := range []string{"where is the search box", "Where's dark mode", "show me where the settings are", "where do I find extensions", "point out the send button"} {
		if !asksWhere(q) {
			t.Errorf("%q asks where something is", q)
		}
	}
	for _, q := range []string{"how do I open settings", "send hi to Joe", "open the place where settings are", "change the theme"} {
		if asksWhere(q) {
			t.Errorf("%q does not ask where something is", q)
		}
	}
}

type keptStore struct {
	traceRecorder
	runs []store.KeptRun
}

func (k *keptStore) DriveTracesIn(context.Context, string) ([]store.KeptRun, error) {
	return k.runs, nil
}

func TestThePlannerIsShownHowSimilarTasksWereDone(t *testing.T) {
	search := uiact.Step{Do: "type", Target: uiact.Selector{Role: "AXTextField", Title: "Search"}, Text: "suhani"}
	open := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Suhani"}}
	write := uiact.Step{Do: "type", Target: uiact.Selector{Role: "AXTextArea", Title: "Type a message"}, Text: "{s0}"}
	kept := &keptStore{runs: []store.KeptRun{
		{Shape: "send {s0} to suhani on whatsapp", Program: uiact.Program{Steps: []uiact.Step{search, open, write, pressEnter()}}},
		{Shape: "coach: how do i change the wallpaper", Program: uiact.Program{Steps: []uiact.Step{open}}},
	}}
	e := &Engine{Traces: kept}

	near := e.similarRuns(context.Background(), "WhatsApp", "send good night to Joe on whatsapp")
	if len(near) != 1 {
		t.Fatalf("found %d similar run(s), want the one about sending: %q", len(near), near)
	}
	for _, want := range []string{`type "suhani" into AXTextField "Search"`, `AXTextArea "Type a message"`, "press Enter"} {
		if !strings.Contains(near[0], want) {
			t.Errorf("want %s in:\n%s", want, near[0])
		}
	}
	if strings.Contains(near[0], "{s0}") {
		t.Errorf("a placeholder was shown to the model as if it were a value:\n%s", near[0])
	}

	if far := e.similarRuns(context.Background(), "WhatsApp", "start a video call"); len(far) != 0 {
		t.Errorf("an unrelated task was shown %d run(s): %q", len(far), far)
	}
}

func TestSimilarRunsReachThePlanPrompt(t *testing.T) {
	prompt := synth.BuildPlanPrompt(synth.PlanRequest{
		Task: "send hi to Joe", Tree: editorTree(),
		Similar: []string{`"send … to suhani" was done with: 1. click AXButton "Suhani"`},
	})
	if !strings.Contains(prompt, "HOW SIMILAR TASKS WERE DONE HERE BEFORE") || !strings.Contains(prompt, `AXButton "Suhani"`) {
		t.Errorf("the similar run is missing from the prompt:\n%s", prompt)
	}
}
