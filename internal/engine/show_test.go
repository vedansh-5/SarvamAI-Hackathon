package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func quickLook(t *testing.T, d time.Duration) {
	t.Helper()
	was := lookBeforeActing
	lookBeforeActing = d
	t.Cleanup(func() { lookBeforeActing = was })
}

// orderScreen records whether each act came after the step was shown.
type orderScreen struct {
	movingScreen
	events *[]string
}

func (o *orderScreen) Act(ctx context.Context, p uiact.Program, t uiact.Tree) error {
	*o.events = append(*o.events, "act")
	return o.movingScreen.Act(ctx, p, t)
}

// Showing the click before making it: every step is drawn first, then done.
func TestADriveStepIsShownBeforeItIsPerformed(t *testing.T) {
	quickLook(t, time.Millisecond)

	var events []string
	e := &Engine{
		UIScreen: &orderScreen{events: &events},
		Steps: &countingSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "done"},
		}},
		ShowStep: func(g GuideStep) { events = append(events, "show") },
	}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(events) < 2 || events[0] != "show" || events[1] != "act" {
		t.Errorf("events = %v, want the step shown and then performed", events)
	}
}

// The pause is a veto. Stopping during it means the step is not done at all —
// that is the whole reason the pause is there.
func TestStoppingWhileAStepIsShownMeansItIsNotPerformed(t *testing.T) {
	quickLook(t, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	var events []string
	e := &Engine{
		UIScreen: &orderScreen{events: &events},
		Steps:    &countingSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}}},
		ShowStep: func(GuideStep) {
			events = append(events, "show")
			cancel()
		},
	}

	_, err := e.runDrive(ctx, tracePlan())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want the run stopped", err)
	}
	for _, ev := range events {
		if ev == "act" {
			t.Fatal("the step was performed although the run was stopped while it was shown")
		}
	}
}

// The ghost has to know where to go, so the rectangle comes from the window.
func TestAShownStepCarriesItsRectangle(t *testing.T) {
	quickLook(t, time.Millisecond)

	var shown []GuideStep
	e := &Engine{
		UIScreen: &movingScreen{},
		Steps: &countingSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "done"},
		}},
		ShowStep: func(g GuideStep) { shown = append(shown, g) },
	}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(shown) != 1 {
		t.Fatalf("shown %d step(s), want 1", len(shown))
	}
	if g := shown[0]; g.W == 0 || g.H == 0 || g.Do != "type" || g.Detail != "hello" {
		t.Errorf("shown = %+v, want the field's rectangle, the verb and the text", g)
	}
}

// A single window action shows each of its steps too.
func TestAWindowActionShowsItsSteps(t *testing.T) {
	quickLook(t, time.Millisecond)

	var shown []GuideStep
	moved := editorTree()
	moved.Elements[0].Value = "hello"
	e := &Engine{
		UIScreen: &fakeScreen{tree: editorTree(), afterwards: &moved},
		UIWriter: stubUIWriter{},
		ShowStep: func(g GuideStep) { shown = append(shown, g) },
	}

	if _, err := e.runUI(context.Background(), uiPlan()); err != nil {
		t.Fatalf("runUI: %v", err)
	}
	if len(shown) != 1 {
		t.Errorf("shown %d step(s), want 1", len(shown))
	}
}

// Nobody to show it to, no pause. A build without a shell runs as it always did.
func TestWithNobodyWatchingThereIsNoPause(t *testing.T) {
	quickLook(t, time.Hour)

	e := &Engine{
		UIScreen: &movingScreen{},
		Steps: &countingSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "done"},
		}},
	}

	done := make(chan error, 1)
	go func() { _, err := e.runDrive(context.Background(), tracePlan()); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runDrive: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waited to show a step to nobody")
	}
}
