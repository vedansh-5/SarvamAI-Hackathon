package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func TestAStepThatKeepsChangingNothingStopsTheRun(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	w := &alwaysStep{step: clickMessage()}
	e := &Engine{UIScreen: f, Steps: w}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("a run going nowhere was reported as success")
	}
	if !strings.Contains(err.Error(), "without getting anywhere") {
		t.Errorf("err = %v, want it to say the run was going in circles", err)
	}
	if f.acts != mostStillTries {
		t.Errorf("ran the same dead step %d time(s), want %d", f.acts, mostStillTries)
	}
}

type backAndForth struct {
	calls int
	seen  []synth.StepRequest
}

func (b *backAndForth) WriteStep(_ context.Context, req synth.StepRequest) (synth.StepAnswer, error) {
	b.calls++
	b.seen = append(b.seen, req)
	if len(req.Done)%2 == 0 {
		return synth.StepAnswer{Step: clickMessage()}, nil
	}
	return synth.StepAnswer{Step: pressEnter()}, nil
}

func TestGoingBackAndForthStopsTheRunAndSaysWhy(t *testing.T) {
	f := &movingScreen{}
	w := &backAndForth{}
	e := &Engine{UIScreen: f, Steps: w}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("a run going back and forth was reported as success")
	}
	if !strings.Contains(err.Error(), "back and forth") {
		t.Errorf("err = %v, want it to name the two steps it was stuck between", err)
	}
	if f.acts != 3 {
		t.Errorf("ran %d step(s), want the 3 before the pattern repeats", f.acts)
	}

	told := false
	for _, req := range w.seen {
		for _, why := range req.Failures {
			if strings.Contains(why, "back and forth") {
				told = true
			}
		}
	}
	if !told {
		t.Error("the model was never told it was going back and forth, so it could not choose differently")
	}
}

func TestTheSameClickSeveralTimesIsAllowedWhenItMoves(t *testing.T) {
	var h stepsSoFar
	next := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Next Month"}}
	for i := 0; i < 5; i++ {
		if why := h.goingInCircles(next, "Click Next Month"); why != "" {
			t.Fatalf("refused click %d of a control that works: %s", i+1, why)
		}
		h.did(next, "Click Next Month")
	}
}

func TestTypingTheSameThingAThirdTimeIsRefused(t *testing.T) {
	var h stepsSoFar
	typing := typeIntoMessage()
	other := clickMessage()
	h.did(typing, "Type hello")
	h.did(other, "Click Message")
	if why := h.goingInCircles(typing, "Type hello"); why != "" {
		t.Fatalf("refused a second go at typing: %s", why)
	}
	h.did(typing, "Type hello")
	h.did(pressEnter(), "Press Enter")
	if why := h.goingInCircles(typing, "Type hello"); why == "" {
		t.Error("typing the same text into the same field a third time was allowed")
	}
}

type lateScreen struct {
	fakeScreen
	arrivesAt int
}

func (l *lateScreen) Capture(context.Context, string) (uiact.Tree, error) {
	l.captures++
	tree := editorTree()
	if l.captures >= l.arrivesAt {
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: "AXMenuItem", Title: "VNS Varanasi", Enabled: true,
			Position: []int{100, 300}, Size: []int{200, 30}, Path: []int{0, 2},
		})
	}
	return tree, nil
}

func TestAControlThatHasNotArrivedYetIsWaitedForNotAskedAbout(t *testing.T) {
	late, poll := lateFor, arrivalPoll
	lateFor, arrivalPoll = 500*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { lateFor, arrivalPoll = late, poll })

	suggestion := uiact.Step{
		Do:       "click",
		Target:   uiact.Selector{Role: "AXMenuItem", Title: "VNS Varanasi"},
		Describe: "Click the Varanasi suggestion",
	}
	f := &lateScreen{arrivesAt: 3}
	w := &scriptedSteps{answers: []synth.StepAnswer{{Step: suggestion}}}
	e := &Engine{UIScreen: f, Steps: w}

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if f.acts != 1 {
		t.Errorf("ran %d step(s), want the one click once the suggestion appeared", f.acts)
	}
	if w.calls != 1 {
		t.Errorf("asked the model %d time(s) about a step that only needed the page to catch up, want 1", w.calls)
	}
}

func TestTheSameStepRightAfterItWorkedIsRefused(t *testing.T) {
	f := &movingScreen{}
	w := &alwaysStep{step: clickMessage()}
	e := &Engine{UIScreen: f, Steps: w}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("a run that kept giving the same step was reported as success")
	}
	if !strings.Contains(err.Error(), "same steps") {
		t.Errorf("err = %v, want it to say it kept coming back to the same steps", err)
	}
	if f.acts != 1 {
		t.Errorf("clicked the same control %d time(s) though every click changed the window, want 1", f.acts)
	}
}

func TestAClickIsRefusedAfterItsThirdTime(t *testing.T) {
	var h stepsSoFar
	row := clickMessage()
	for i := 0; i < mostSameClicks; i++ {
		if why := h.goingInCircles(row, "Click Message"); why != "" {
			t.Fatalf("refused click %d: %s", i+1, why)
		}
		h.did(row, "Click Message")
		other := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: fmt.Sprint("Other ", i)}}
		h.did(other, "Click another")
	}
	if why := h.goingInCircles(row, "Click Message"); why == "" {
		t.Errorf("a click on the same control was allowed a %d time", mostSameClicks+1)
	}
}

func TestARunThatRepeatedAStepIsCalledCircling(t *testing.T) {
	row := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXRow", Title: "Variable"}, Describe: "Click the Variable row"}
	env := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Environments"}, Describe: "Click Environments"}
	next := uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Next Month"}}

	if why := wentInCircles([]uiact.Step{env, row, row, row}); why == "" {
		t.Error("the Postman run, the same row clicked back to back, was not called circling")
	}
	if why := wentInCircles([]uiact.Step{row, env, row, env, row}); why == "" {
		t.Error("a step given three times was not called circling")
	}
	if why := wentInCircles([]uiact.Step{next, next, next, next, env}); why != "" {
		t.Errorf("paging through months was called circling: %s", why)
	}
	if why := wentInCircles([]uiact.Step{env, row, typeIntoMessage(), pressEnter()}); why != "" {
		t.Errorf("an ordinary run was called circling: %s", why)
	}
}
