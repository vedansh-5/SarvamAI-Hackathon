package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func withSend(tree uiact.Tree) uiact.Tree {
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: "AXButton", Title: "Send", Enabled: true,
		Position: []int{300, 200}, Size: []int{60, 30}, Path: []int{0, 9},
	})
	return tree
}

type sendScreen struct{ handScreen }

func (s *sendScreen) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	tree, err := s.handScreen.Capture(ctx, app)
	return withSend(tree), err
}

func clickSend() uiact.Step {
	return uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Send"}, Describe: "Click Send"}
}

func sendingEngine(screen uiact.Screen, resume chan struct{}, paused *[]GuideStep, steps ...synth.StepAnswer) *Engine {
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{answers: append(steps, synth.StepAnswer{Done: true})}}
	e.ShowStep = func(GuideStep) {}
	e.Resume = resume
	e.Paused = func(g GuideStep) { *paused = append(*paused, g) }
	return e
}

func TestAStepThatSendsWaitsToBeToldToGoAhead(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	screen := &sendScreen{}
	resume := make(chan struct{}, 1)
	e := sendingEngine(screen, resume, &paused, synth.StepAnswer{Step: typeIntoMessage()}, synth.StepAnswer{Step: clickSend()})

	go func() {
		time.Sleep(40 * time.Millisecond)
		resume <- struct{}{}
	}()
	if _, err := e.runDrive(context.Background(), drivePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if len(paused) != 1 {
		t.Fatalf("asked %d time(s), want once, before Send and not before typing", len(paused))
	}
	if paused[0].Number != 2 || !strings.Contains(paused[0].Ask, "send") || paused[0].Label != "Click Send" {
		t.Errorf("asked with %+v, want step 2 and why it stopped", paused[0])
	}
	if screen.acts != 2 {
		t.Errorf("acted %d time(s), want both steps done once told to go ahead", screen.acts)
	}
}

func TestSayingNoLeavesTheMessageUnsent(t *testing.T) {
	quickLook(t, 0)
	var paused []GuideStep
	screen := &sendScreen{}
	e := sendingEngine(screen, make(chan struct{}, 1), &paused, synth.StepAnswer{Step: typeIntoMessage()}, synth.StepAnswer{Step: clickSend()})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	if _, err := e.runDrive(ctx, drivePlan()); err == nil {
		t.Fatal("the run carried on after being stopped")
	}
	if screen.acts != 1 {
		t.Errorf("acted %d time(s), want only the typing", screen.acts)
	}
}

func TestAStepTheModelMarkedFinalWaitsWhateverTheControlSays(t *testing.T) {
	quickLook(t, 0)
	quickPause(t, 30*time.Millisecond)
	var paused []GuideStep
	screen := &handScreen{}
	marked := typeIntoMessage()
	marked.Final = true
	e := sendingEngine(screen, make(chan struct{}, 1), &paused, synth.StepAnswer{Step: marked})

	if _, err := e.runDrive(context.Background(), drivePlan()); err == nil {
		t.Fatal("a final step ran with nobody saying go ahead")
	}
	if len(paused) != 1 || screen.acts != 0 {
		t.Errorf("asked %d time(s) and acted %d; want asked once and nothing done", len(paused), screen.acts)
	}
}
