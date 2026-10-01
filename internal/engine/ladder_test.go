package engine

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type oneStepWriter struct{ asked int }

func (w *oneStepWriter) WriteUIAction(context.Context, synth.UIActRequest) (uiact.Program, error) {
	w.asked++
	return uiact.Program{App: "Editor", Steps: []uiact.Step{typeIntoMessage()}}, nil
}

func TestATaskInAWindowIsWorkedThroughStepByStepNotWrittenInOneGo(t *testing.T) {
	oneShot := &oneStepWriter{}
	e := &Engine{
		Apps:     &capability.Catalog{Roots: []string{t.TempDir()}},
		UIScreen: &movingScreen{},
		UIWriter: oneShot,
		Steps:    &scriptedSteps{},
	}

	r := route.Route{Kind: route.KindAppAction, Slots: map[string]string{"app": "Editor", "action": "say hello and send it"}}
	plan, _, err := e.actInApplication(context.Background(), r, "say hello and send it in editor")
	if err != nil {
		t.Fatalf("actInApplication: %v", err)
	}
	if plan.Drive == nil || plan.UI != nil {
		t.Fatalf("plan = %+v, want the step-by-step run, which can see past the first screen", plan)
	}
	if oneShot.asked != 0 {
		t.Errorf("asked for a one-go program %d time(s) though the task could be worked through", oneShot.asked)
	}
}
