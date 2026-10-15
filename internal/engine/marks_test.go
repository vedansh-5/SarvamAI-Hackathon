package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

type browserPage struct {
	front   string
	answer  string
	scripts []string
}

func (b *browserPage) Capture(context.Context, string) (uiact.Tree, error)  { return uiact.Tree{}, nil }
func (b *browserPage) Act(context.Context, uiact.Program, uiact.Tree) error { return nil }
func (b *browserPage) Frontmost(context.Context) (string, error)            { return b.front, nil }
func (b *browserPage) BringToFront(context.Context, string) error           { return nil }
func (b *browserPage) Type(context.Context, string) error                   { return nil }
func (b *browserPage) Handles(app string) bool                              { return app == "Google Chrome" }
func (b *browserPage) FrontBrowser(context.Context) (string, bool) {
	return b.front, b.Handles(b.front)
}
func (b *browserPage) AskPage(_ context.Context, _ string, script string, out any) error {
	b.scripts = append(b.scripts, script)
	answer := b.answer
	if strings.Contains(script, "__vansStyle") {
		answer = `{"ok":true,"removed":true}`
	}
	return json.Unmarshal([]byte(answer), out)
}

func marking(t *testing.T, page *browserPage) (*Engine, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return &Engine{PageScreen: page, Prefs: st}, st
}

func TestMarkingTextOnAPageNeedsNoModelAndSaysHowMany(t *testing.T) {
	ctx := context.Background()
	page := &browserPage{front: "Google Chrome", answer: `{"ok":true,"matched":10,"alike":60,"painted":true}`}
	e, st := marking(t, page)
	_ = e.SetTheme(ctx, "purple")

	plan, err := e.Ask(ctx, Ask{Instruction: "highlight the ones that start with 2321"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Marked || !strings.Contains(plan.Say, "10 of 60 that start with 2321") {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.DecisionCalls+plan.SynthesisCalls != 0 {
		t.Errorf("a model was asked: %d calls", plan.DecisionCalls+plan.SynthesisCalls)
	}
	if len(page.scripts) != 1 || !strings.Contains(page.scripts[0], `"soft":"#e8e0ff"`) || !strings.Contains(page.scripts[0], `"text":"2321"`) {
		t.Errorf("the script did not carry the purple highlight and the text: %v", page.scripts)
	}
	if on, _ := st.Setting(ctx, marksOnIn); on != "Google Chrome" {
		t.Errorf("the marks were not noted, so they could be left behind: %q", on)
	}
}

func TestTheNextRequestTakesTheMarksOff(t *testing.T) {
	ctx := context.Background()
	page := &browserPage{front: "Google Chrome", answer: `{"ok":true,"matched":3,"alike":3,"painted":true}`}
	e, st := marking(t, page)
	if _, err := e.Ask(ctx, Ask{Instruction: "highlight the ones that start with 23"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Ask(ctx, Ask{Instruction: "highlight the ones that end with 07"}); err != nil {
		t.Fatal(err)
	}
	if len(page.scripts) != 3 || !strings.Contains(page.scripts[1], "__vansMarks") {
		t.Fatalf("the marks were not cleared before the next request: %d scripts", len(page.scripts))
	}
	e.TakeMarksOff(ctx)
	if on, _ := st.Setting(ctx, marksOnIn); on != "" {
		t.Errorf("still noted as marked: %q", on)
	}
}

func TestNothingMatchingIsSaidPlainly(t *testing.T) {
	page := &browserPage{front: "Google Chrome", answer: `{"ok":true,"matched":0,"alike":0,"painted":true}`}
	e, st := marking(t, page)
	plan, err := e.Ask(context.Background(), Ask{Instruction: "highlight the ones that start with 99"})
	if err != nil || plan.Marked || !strings.Contains(plan.Say, "Nothing on this page seems to start with 99") {
		t.Fatalf("plan = %+v, err = %v", plan, err)
	}
	if on, _ := st.Setting(context.Background(), marksOnIn); on != "" {
		t.Errorf("noted marks that were never made")
	}
}

func TestAPageThatWillNotShowTheMarksIsAFailureNotASuccess(t *testing.T) {
	page := &browserPage{front: "Google Chrome", answer: `{"ok":true,"matched":4,"alike":4,"painted":false}`}
	e, _ := marking(t, page)
	if _, err := e.Ask(context.Background(), Ask{Instruction: "highlight the ones that start with 23"}); err == nil {
		t.Fatal("reported success for marks the page would not show")
	}
}

func TestMarkingIsOnlyForABrowserInFront(t *testing.T) {
	page := &browserPage{front: "Google Chrome", answer: `{"ok":true,"matched":1,"alike":1,"painted":true}`}
	e, _ := marking(t, page)
	_, handled, _ := e.aboutMarks(context.Background(), Ask{
		Instruction: "highlight the ones that start with 23",
		Context:     &screen.Context{AppName: "Microsoft Excel"},
	})
	if handled || len(page.scripts) != 0 {
		t.Errorf("marked a browser page while Excel was in front")
	}
}
