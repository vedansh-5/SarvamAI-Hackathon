package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func quickWait(t *testing.T, ceiling time.Duration) {
	t.Helper()
	c, p, look := waitCeiling, waitPoll, lookBeforeActing
	waitCeiling, waitPoll, lookBeforeActing = ceiling, time.Millisecond, 0
	t.Cleanup(func() { waitCeiling, waitPoll, lookBeforeActing = c, p, look })
}

func waitForIt(text string) uiact.Step {
	return uiact.Step{Do: "wait", Text: text, Describe: "Wait for the upload",
		Target: uiact.Selector{Role: "AXMenuItem", Title: "VNS Varanasi"}}
}

func TestAWaitHoldsTheRunUntilTheThingAppears(t *testing.T) {
	quickWait(t, time.Second)

	screen := &lateScreen{arrivesAt: 6}
	steps := &recordingSteps{answers: []synth.StepAnswer{{Step: waitForIt("appears")}, {Done: true, Why: "it is up"}}}
	e := &Engine{UIScreen: screen, Steps: steps}

	receipt, err := e.runDrive(context.Background(), drivePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if screen.acts != 0 {
		t.Errorf("touched the screen %d time(s) while waiting", screen.acts)
	}
	if len(steps.requests) != 2 {
		t.Fatalf("asked the model %d time(s), want 2: waiting costs no calls", len(steps.requests))
	}
	if told := strings.Join(steps.requests[1].Done, "\n"); !strings.Contains(told, "it appeared") {
		t.Errorf("the model was not told the wait ended: %q", told)
	}
	if receipt.Work == nil || receipt.Work.Steps != 1 {
		t.Errorf("work = %+v, want the wait counted as the one step", receipt.Work)
	}
}

func TestAWaitThatNeverEndsStopsAndSaysWhatItWaitedFor(t *testing.T) {
	quickWait(t, 30*time.Millisecond)

	e := &Engine{UIScreen: &fakeScreen{tree: editorTree()}, Steps: &scriptedSteps{answers: []synth.StepAnswer{{Step: waitForIt("appears")}}}}

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil || !strings.Contains(err.Error(), "never appeared") || !strings.Contains(err.Error(), "VNS Varanasi") {
		t.Errorf("err = %v, want it to say what never appeared", err)
	}
}

func TestWaitingForSomethingToGoOrBeReady(t *testing.T) {
	tree := editorTree()
	tree.Elements = append(tree.Elements, uiact.Element{Role: "AXButton", Title: "Publish", Enabled: false, Path: []int{0, 2}})
	publish := uiact.Selector{Role: "AXButton", Title: "Publish"}

	if ok, _ := met(tree, uiact.Step{Do: "wait", Target: publish, Text: "ready"}); ok {
		t.Error("a greyed-out button was taken as ready")
	}
	if ok, _ := met(tree, uiact.Step{Do: "wait", Target: publish, Text: "appears"}); !ok {
		t.Error("a button on screen was not taken as having appeared")
	}
	if ok, _ := met(tree, uiact.Step{Do: "wait", Target: publish, Text: "goes"}); ok {
		t.Error("a button still on screen was taken as gone")
	}
	tree.Elements[len(tree.Elements)-1].Enabled = true
	if ok, what := met(tree, uiact.Step{Do: "wait", Target: publish, Text: "ready"}); !ok || what != "it is ready" {
		t.Errorf("ready = %v, %q once the button is usable", ok, what)
	}
	if ok, _ := met(editorTree(), uiact.Step{Do: "wait", Target: publish, Text: "goes"}); !ok {
		t.Error("a button no longer on screen was not taken as gone")
	}
}
