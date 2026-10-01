package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
)

func catalogWith(t *testing.T, apps ...string) *capability.Catalog {
	t.Helper()
	root := t.TempDir()
	for _, app := range apps {
		if err := os.MkdirAll(filepath.Join(root, app+".app", "Contents"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return &capability.Catalog{Roots: []string{root}}
}

type closedScreen struct{ fakeScreen }

func (closedScreen) Capture(_ context.Context, app string) (uiact.Tree, error) {
	return uiact.Tree{}, &uiact.NotRunningError{App: app}
}

func appAction(app, action string) route.Result {
	return route.Result{
		Shape: action,
		Route: route.Route{Kind: route.KindAppAction, Slots: map[string]string{"app": app, "action": action}},
	}
}

func TestATaskIsShownInTheApplicationItNames(t *testing.T) {
	e := &Engine{
		Apps:     catalogWith(t, "Editor"),
		UIScreen: &fakeScreen{tree: editorTree()},
		Steps:    &scriptedSteps{},
	}

	plan, err := e.showInsteadOfDoing(context.Background(),
		Ask{Instruction: "say hello in editor", Mode: ModeShow}, appAction("editor", "say hello"))
	if err != nil {
		t.Fatalf("showInsteadOfDoing: %v", err)
	}
	if plan.Guide == nil || plan.Drive != nil {
		t.Fatalf("plan = %+v, want a guide and nothing that acts", plan)
	}
	if plan.Guide.App != "Editor" || plan.Guide.MustOpen {
		t.Errorf("guide = %+v, want it in Editor, which is open", plan.Guide)
	}
	if plan.Guide.Task != "say hello in editor" || plan.Guide.Shape != "say hello" {
		t.Errorf("guide = %+v, want the person's own words and the shape to remember it by", plan.Guide)
	}
}

func TestAClosedApplicationBecomesTheFirstStepOfBeingShown(t *testing.T) {
	e := &Engine{
		Apps:     catalogWith(t, "Editor"),
		UIScreen: &closedScreen{},
		Steps:    &scriptedSteps{},
	}

	plan, err := e.showInsteadOfDoing(context.Background(),
		Ask{Instruction: "say hello in editor", Mode: ModeShow}, appAction("editor", "say hello"))
	if err != nil {
		t.Fatalf("showInsteadOfDoing: %v", err)
	}
	if plan.Guide == nil || !plan.Guide.MustOpen || plan.Guide.App != "Editor" {
		t.Fatalf("guide = %+v, want opening Editor as the first step", plan.Guide)
	}
}

func TestATaskOnASiteIsShownOnThePage(t *testing.T) {
	page := &pageFake{host: "news.example.org"}
	e := &Engine{UIScreen: untouchable{t}, PageScreen: page, Steps: &scriptedSteps{}}

	res := route.Result{
		Shape: "search for flights on makemytrip",
		Route: route.Route{Kind: route.KindPageAction, Slots: map[string]string{"action": "search for flights", "site": "makemytrip"}},
	}
	plan, err := e.showInsteadOfDoing(context.Background(), Ask{Instruction: "search for flights", Mode: ModeShow}, res)
	if err != nil {
		t.Fatalf("showInsteadOfDoing: %v", err)
	}
	if plan.Guide == nil || !plan.Guide.Web || plan.Guide.Site != "https://www.makemytrip.com" {
		t.Fatalf("guide = %+v, want a page guide that starts by going to the site", plan.Guide)
	}
	if len(page.opened) != 0 {
		t.Errorf("opened %v; being shown means the person goes there", page.opened)
	}
}
