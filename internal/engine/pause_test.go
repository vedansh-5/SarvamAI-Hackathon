package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type handScreen struct {
	movingScreen
	mu     sync.Mutex
	x, y   int
	reads  int
	moveAt int
}

func (h *handScreen) PointerAt() (int, int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reads++
	if h.moveAt > 0 && h.reads == h.moveAt {
		h.x += 200
	}
	return h.x, h.y, true
}

func pausingEngine(screen *handScreen, resume chan struct{}, paused *[]GuideStep) *Engine {
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}
	e.ShowStep = func(GuideStep) {}
	e.Resume = resume
	e.Paused = func(g GuideStep) { *paused = append(*paused, g) }
	return e
}

func quickPause(t *testing.T, d time.Duration) {
	t.Helper()
	was := pausedFor
	pausedFor = d
	t.Cleanup(func() { pausedFor = was })
}

func TestAStillHandDoesNotPauseTheRun(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	screen := &handScreen{}
	e := pausingEngine(screen, make(chan struct{}, 1), &paused)

	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(paused) != 0 {
		t.Errorf("paused %d time(s) with nobody touching the mouse", len(paused))
	}
	if screen.acts != 1 {
		t.Errorf("acted %d time(s), want 1", screen.acts)
	}
}

func TestMovingTheMousePausesUntilToldToCarryOn(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	screen := &handScreen{moveAt: 2}
	resume := make(chan struct{}, 1)
	e := pausingEngine(screen, resume, &paused)

	go func() {
		time.Sleep(40 * time.Millisecond)
		resume <- struct{}{}
	}()

	start := time.Now()
	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(paused) != 1 {
		t.Fatalf("paused %d time(s), want 1", len(paused))
	}
	if paused[0].Number != 1 || paused[0].W == 0 {
		t.Errorf("paused on %+v, want step 1 with where it is", paused[0])
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Error("carried on before being told to")
	}
	if screen.acts != 1 {
		t.Errorf("acted %d time(s), want the step done once, after the pause", screen.acts)
	}
}

func TestAPauseNobodyComesBackToStopsTheRun(t *testing.T) {
	quickLook(t, 0)
	quickPause(t, 30*time.Millisecond)
	var paused []GuideStep
	screen := &handScreen{moveAt: 2}
	e := pausingEngine(screen, make(chan struct{}, 1), &paused)

	_, err := e.runDrive(context.Background(), drivePlan())
	if err == nil {
		t.Fatal("a pause nobody answered let the run go on")
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) during a pause", screen.acts)
	}
}

func TestStoppingDuringAPauseLeavesTheStepUndone(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	screen := &handScreen{moveAt: 2}
	e := pausingEngine(screen, make(chan struct{}, 1), &paused)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := e.runDrive(ctx, drivePlan())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want the run cancelled", err)
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) after being stopped", screen.acts)
	}
}

func TestAStaleCarryOnDoesNotEndAPause(t *testing.T) {
	quickLook(t, 0)
	quickPause(t, 40*time.Millisecond)
	var paused []GuideStep
	screen := &handScreen{moveAt: 2}
	resume := make(chan struct{}, 1)
	resume <- struct{}{}
	e := pausingEngine(screen, resume, &paused)

	if _, err := e.runDrive(context.Background(), drivePlan()); err == nil {
		t.Fatal("a ⌥Space pressed before the pause ended it")
	}
}

func TestNoShellMeansNoWatchingTheHand(t *testing.T) {
	quickLook(t, 0)
	screen := &handScreen{moveAt: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}
	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if screen.reads != 0 {
		t.Errorf("read the pointer %d time(s) with nobody to tell about a pause", screen.reads)
	}
}

var _ uiact.PointerReader = (*handScreen)(nil)
