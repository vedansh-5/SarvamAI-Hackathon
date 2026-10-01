package engine

import (
	"context"
	"errors"
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

type hiddenThenShown struct {
	movingScreen
	raised int
}

func (h *hiddenThenShown) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	if h.raised == 0 {
		return uiact.Tree{}, uiact.ErrNoWindow
	}
	return h.movingScreen.Capture(ctx, app)
}

func (h *hiddenThenShown) BringToFront(context.Context, string) error {
	h.raised++
	return nil
}

func TestAnInstalledAppBehindOthersIsBroughtForwardNotSwappedForItsSite(t *testing.T) {
	screen := &hiddenThenShown{}
	page := &pageFake{host: "news.example.org"}
	e := &Engine{
		Apps:       catalogWith(t, "WhatsApp"),
		UIScreen:   screen,
		PageScreen: page,
		Steps:      &scriptedSteps{},
	}

	r := route.Route{Kind: route.KindAppAction, Slots: map[string]string{"app": "whatsapp", "action": "send hi to Joe"}}
	plan, _, err := e.actInApplication(context.Background(), r, "send hi to joe on whatsapp")
	if err != nil {
		t.Fatalf("actInApplication: %v", err)
	}
	if plan.Drive == nil || plan.Drive.Web || plan.Drive.Open != "" {
		t.Fatalf("plan = %+v, want the run in the application itself", plan.Drive)
	}
	if screen.raised != 1 {
		t.Errorf("raised %d time(s), want the window brought forward once", screen.raised)
	}
}

func TestAnInstalledAppThatIsClosedIsNotSwappedForItsSite(t *testing.T) {
	page := &pageFake{host: "news.example.org"}
	e := &Engine{
		Apps:       catalogWith(t, "WhatsApp"),
		UIScreen:   &closedScreen{},
		PageScreen: page,
		UIWriter:   &oneStepWriter{},
		Steps:      &scriptedSteps{},
	}

	r := route.Route{Kind: route.KindAppAction, Slots: map[string]string{"app": "whatsapp", "action": "send hi to Joe"}}
	_, _, err := e.actInApplication(context.Background(), r, "send hi to joe on whatsapp")
	var closed *uiact.NotRunningError
	if !errors.As(err, &closed) || closed.App != "WhatsApp" {
		t.Fatalf("err = %v, want to be told WhatsApp is not open", err)
	}
	if len(page.opened) != 0 {
		t.Errorf("opened %v", page.opened)
	}
}
