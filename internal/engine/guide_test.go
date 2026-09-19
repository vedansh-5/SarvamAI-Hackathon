package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func quickGuide(t *testing.T) {
	t.Helper()
	poll, timeout := guidePoll, guideStepTimeout
	guidePoll, guideStepTimeout = time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { guidePoll, guideStepTimeout = poll, timeout })
}

func guidePlan() *Plan {
	return &Plan{Guide: &GuidePlan{App: "Editor", Task: "say hello"}}
}

func TestGuidePointsAtTheControlAndNeverTouchesIt(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
	}); err != nil {
		t.Fatalf("runGuide: %v", err)
	}

	if len(shown) != 1 {
		t.Fatalf("pointed %d time(s), want 1: %+v", len(shown), shown)
	}
	got := shown[0]
	if got.X != 100 || got.Y != 200 || got.W != 80 || got.H != 40 {
		t.Errorf("pointed at (%d,%d %dx%d), want the control's rectangle", got.X, got.Y, got.W, got.H)
	}
	if !strings.Contains(got.Label, "Message") {
		t.Errorf("label = %q, want what a person would be told", got.Label)
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) — showing is not doing", screen.acts)
	}
}

// The next step is worked out from the window after the person has acted, so
// the loop has to wait for them rather than run ahead.
func TestGuideWaitsForTheWindowToChange(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 3}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	if _, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}); err != nil {
		t.Fatalf("runGuide: %v", err)
	}
	if screen.captures < 4 {
		t.Errorf("read the window %d time(s); it cannot have waited", screen.captures)
	}
}

func TestGuideStopsWhenNothingHappens(t *testing.T) {
	quickGuide(t)

	screen := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: screen, Steps: &alwaysStep{step: typeIntoMessage()}}

	_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {})
	if err == nil {
		t.Fatal("waited forever on a window nobody touched")
	}
	if !strings.Contains(err.Error(), "nothing happened") {
		t.Errorf("err = %v", err)
	}
}

func TestGuideRefusesAControlItCannotPointAt(t *testing.T) {
	quickGuide(t)

	hidden := editorTree()
	hidden.Elements[0].Position = nil
	hidden.Elements[0].Size = nil

	e := &Engine{UIScreen: &fakeScreen{tree: hidden}, Steps: &alwaysStep{step: typeIntoMessage()}}

	if _, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}); err == nil {
		t.Fatal("pointed at a control with no place on screen")
	}
}

func TestPlanGuideRefusesAWindowWithNothingInIt(t *testing.T) {
	e := &Engine{
		UIScreen: &fakeScreen{tree: uiact.Tree{App: "Editor", Window: "Editor"}},
		Steps:    &alwaysStep{step: typeIntoMessage()},
	}

	if _, err := e.planGuide(context.Background(), "say hello"); err == nil {
		t.Fatal("offered to point at a window with nothing in it")
	}
}
