package engine

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func listWith(bigSurAt int, window string) uiact.Tree {
	return uiact.Tree{
		App:    "Settings",
		Window: window,
		Elements: []uiact.Element{
			{Role: "AXScrollArea", Enabled: true, Position: []int{0, 0}, Size: []int{400, 300}, Path: []int{0}},
			{Role: "AXButton", Title: "Sonoma", Enabled: true, Position: []int{20, 40}, Size: []int{100, 20}, Path: []int{0, 0}},
			{Role: "AXButton", Title: "Big Sur", Enabled: true, Position: []int{20, bigSurAt}, Size: []int{100, 20}, Path: []int{0, 1}},
		},
	}
}

func clickBigSur() uiact.Step {
	return uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Big Sur"}, Describe: "Click Big Sur"}
}

type scrollingScreen struct {
	fakeScreen
	script   []uiact.Tree
	scrolled int
}

func (s *scrollingScreen) Capture(context.Context, string) (uiact.Tree, error) {
	s.captures++
	if s.captures > len(s.script) {
		return s.script[len(s.script)-1], nil
	}
	return s.script[s.captures-1], nil
}

func (s *scrollingScreen) ScrollIntoView(context.Context, uiact.Tree, uiact.Selector) error {
	s.scrolled++
	s.script = []uiact.Tree{listWith(150, "Settings")}
	s.captures = 0
	return nil
}

func (s *scrollingScreen) Act(_ context.Context, p uiact.Program, t uiact.Tree) error {
	s.acts++
	s.script = []uiact.Tree{listWith(150, "Big Sur")}
	s.captures = 0
	return uiact.Validate(p, t)
}

func TestTheCoachAsksForAScrollBeforePointingAtSomethingOutOfView(t *testing.T) {
	quickGuide(t)

	hidden := listWith(600, "Settings")
	shownTree := listWith(150, "Settings")
	pressed := listWith(150, "Big Sur")
	screen := &scrollingScreen{script: []uiact.Tree{hidden, hidden, shownTree, shownTree, shownTree, pressed}}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: clickBigSur()}, {Done: true}},
	}}

	var shown []GuideStep
	plan := &Plan{Guide: &GuidePlan{App: "Settings", Task: "use Big Sur"}}
	if _, err := e.RunGuide(context.Background(), plan, func(s GuideStep) { shown = append(shown, s) }, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	if len(shown) < 2 {
		t.Fatalf("shown %d step(s), want the scroll and then the control: %+v", len(shown), shown)
	}
	if shown[0].Do != "scroll" || shown[0].Detail != "down" || shown[0].H != 300 {
		t.Errorf("first = %+v, want a scroll down over the list", shown[0])
	}
	if last := shown[len(shown)-1]; last.Do != "click" || last.Y != 150 {
		t.Errorf("last = %+v, want Big Sur pointed at where it scrolled to", last)
	}
	if screen.scrolled != 0 || screen.acts != 0 {
		t.Errorf("scrolled %d and acted %d time(s); the coach only shows", screen.scrolled, screen.acts)
	}
}

func TestADriveRunScrollsAControlIntoViewBeforeUsingIt(t *testing.T) {
	screen := &scrollingScreen{script: []uiact.Tree{listWith(600, "Settings")}}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: clickBigSur()}, {Done: true}},
	}}

	var shown []GuideStep
	e.ShowStep = func(g GuideStep) { shown = append(shown, g) }
	quickLook(t, 0)

	plan := &Plan{Drive: &DrivePlan{App: "Settings", Task: "use Big Sur"}}
	receipt, err := e.runDrive(context.Background(), plan)
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if screen.scrolled != 1 || screen.acts != 1 {
		t.Errorf("scrolled %d and acted %d time(s), want one of each", screen.scrolled, screen.acts)
	}
	if len(shown) != 1 || shown[0].Y != 150 {
		t.Errorf("shown = %+v, want the ghost sent to where it scrolled to", shown)
	}
	if !receipt.Verified {
		t.Error("not verified, though the window moved on after the click")
	}
}
