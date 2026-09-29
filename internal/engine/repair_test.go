package engine

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// A window whose one control is a row, not a button, and which changes after each
// read so a step that runs is seen to land.
type sidebarScreen struct {
	fakeScreen
	reads int
}

func (s *sidebarScreen) Capture(context.Context, string) (uiact.Tree, error) {
	s.reads++
	s.fakeScreen.captures++
	return uiact.Tree{
		App:    "Editor",
		Window: "Editor",
		Elements: []uiact.Element{
			{Role: "AXRow", Title: "Wallpaper", Enabled: true,
				Position: []int{20, 100}, Size: []int{180, 28}, Path: []int{0, 1}},
			{Role: "AXStaticText", Title: nthChange(s.reads), Enabled: true,
				Position: []int{0, 0}, Size: []int{10, 10}, Path: []int{0, 2}},
		},
	}, nil
}

func nthChange(n int) string {
	return "change " + string(rune('a'+n%26))
}

// The measured bug: the model asks for a button, the window has a row of that
// name, and asking again produced the same wrong answer twice before the run gave
// up. The answer was in the window the whole time.
func TestAWrongRoleIsCorrectedWithoutAskingAgain(t *testing.T) {
	steps := &countingSteps{answers: []synth.StepAnswer{
		{Step: uiact.Step{
			Do:     "click",
			Target: uiact.Selector{Role: "AXButton", Title: "Wallpaper"},
		}},
		{Done: true, Why: "opened"},
	}}
	e := &Engine{UIScreen: &sidebarScreen{}, Steps: steps}

	receipt, err := e.runDrive(context.Background(), tracePlan())
	if err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	// Two calls: the step, and hearing the task was done. Not three — the
	// refusal cost nothing.
	if steps.calls != 2 {
		t.Errorf("asked the model %d time(s), want 2 — the correction should cost none", steps.calls)
	}
	if receipt.Work == nil || receipt.Work.Steps != 1 {
		t.Fatalf("work = %+v, want the corrected step to have run", receipt.Work)
	}
	if !receipt.Verified {
		t.Error("the corrected step did not land")
	}
}

// Correcting a step must not become a way to keep trying. If the corrected step is
// refused too, the window was never the problem and the model gets asked.
func TestAStepThatIsWrongInSomeOtherWayStillGoesBackToTheModel(t *testing.T) {
	steps := &countingSteps{answers: []synth.StepAnswer{
		// Typing into a row: the name is right, the role is wrong, and the
		// correction is still not something that can hold typed text.
		{Step: uiact.Step{
			Do:     "type",
			Target: uiact.Selector{Role: "AXTextField", Title: "Wallpaper"},
			Text:   "hello",
		}},
		{Done: true, Why: "gave up"},
	}}
	e := &Engine{UIScreen: &sidebarScreen{}, Steps: steps}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if steps.calls < 2 {
		t.Errorf("asked the model %d time(s); a correction that does not fit must not be run", steps.calls)
	}
}
