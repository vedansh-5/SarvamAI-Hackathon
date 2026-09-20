package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/runner"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// fixedDecider answers with one decision, whatever it is asked.
type fixedDecider struct {
	d     route.Decision
	calls int
}

func (f *fixedDecider) Decide(context.Context, route.Request) (route.Decision, error) {
	f.calls++
	return f.d, nil
}

func withRouter(t *testing.T, h *harness, d route.Decision) *fixedDecider {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	dec := &fixedDecider{d: d}
	h.engine.Router = &route.Router{Decider: dec, Cache: st}
	return dec
}

func extractDecision() route.Decision {
	return route.Decision{
		Kind: route.KindWebExtract,
		Slots: map[string]string{
			"url":    "{u0}",
			"intent": "product names and prices",
		},
	}
}

// The point of the router: the user says one sentence and the runtime works
// out both the page and what to take off it.
func TestAskRoutesAnInstructionAndPreparesIt(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	withRouter(t, h, extractDecision())

	plan, err := h.engine.Ask(context.Background(), Ask{
		Instruction: "product names and prices from " + pageURL,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Rows) == 0 {
		t.Fatal("no rows")
	}
	if plan.Route == nil || plan.Route.Kind != route.KindWebExtract {
		t.Fatalf("route not recorded on the plan: %#v", plan.Route)
	}
	if plan.DecisionCalls != 1 {
		t.Errorf("decision calls = %d, want 1", plan.DecisionCalls)
	}
}

// An answer the user only wants to read changes nothing, so there is nothing
// to confirm. The gate follows the destination rather than being a fixed step.
func TestAnAnswerForThePanelNeedsNoConfirmation(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	withRouter(t, h, extractDecision())

	plan, err := h.engine.Ask(context.Background(), Ask{
		Instruction: "product names and prices from " + pageURL,
	})
	if err != nil {
		t.Fatal(err)
	}

	if plan.NeedsCommit() {
		t.Error("a panel answer asked for confirmation it does not need")
	}
	if _, err := h.engine.Commit(context.Background(), plan); err == nil {
		t.Error("committing a panel answer was allowed; there is nothing to write")
	}
	if len(h.sink.writes) != 0 {
		t.Errorf("the sink was written to %d times for a panel answer", len(h.sink.writes))
	}
}

func TestAskingForANamedDestinationStillGates(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	withRouter(t, h, extractDecision())

	plan, err := h.engine.Ask(context.Background(), Ask{
		Instruction: "product names and prices from " + pageURL,
		Destination: "notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsCommit() {
		t.Fatal("a write to Notes did not ask for confirmation")
	}
	if len(h.sink.writes) != 0 {
		t.Fatal("Prepare wrote before the gate")
	}
	if _, err := h.engine.Commit(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if len(h.sink.writes) != 1 {
		t.Errorf("sink writes = %d, want 1", len(h.sink.writes))
	}
}

// A typo in a destination must not burn the plan: resolution happens before
// the plan is marked spent, so the user can correct it and commit again.
func TestAnUnknownDestinationIsRejectedWithoutSpendingThePlan(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	withRouter(t, h, extractDecision())

	plan, err := h.engine.Ask(context.Background(), Ask{
		Instruction: "product names and prices from " + pageURL,
		Destination: "obsidian",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = h.engine.Commit(context.Background(), plan)
	if err == nil {
		t.Fatal("an unknown destination was accepted")
	}
	if !strings.Contains(err.Error(), "notes") {
		t.Errorf("the error should list what is available, got: %v", err)
	}
	if plan.Committed() {
		t.Error("the plan was spent by a destination that never existed")
	}

	// And it is still usable once corrected.
	plan.Destination = "notes"
	if _, err := h.engine.Commit(context.Background(), plan); err != nil {
		t.Fatalf("the corrected plan would not commit: %v", err)
	}
}

// The router can choose kinds the engine has no adapter for. Saying so is
// better than attempting it, and the message has to carry what VANS *can* do
// or the user is left with a dead end.
func TestAKindWithNoAdapterIsDeclinedWithAlternatives(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	withRouter(t, h, route.Decision{
		Kind:  route.KindAppAction,
		Slots: map[string]string{"app": "Telegram", "action": "reply"},
	})

	_, err := h.engine.Ask(context.Background(), Ask{Instruction: "reply in Telegram"})

	var unroutable *UnroutableError
	if !errors.As(err, &unroutable) {
		t.Fatalf("want UnroutableError, got %v", err)
	}
	if len(unroutable.Can) == 0 {
		t.Error("the refusal offered no alternative")
	}
}

func TestDestinationsAlwaysOfferThePanelFirst(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	got := h.engine.Destinations()

	if len(got) == 0 || got[0].Name != PanelDestination {
		t.Fatalf("panel is not the first destination: %#v", got)
	}
	if got[0].TouchesWorld {
		t.Error("the panel is marked as touching the world; it writes nothing")
	}
}

// Tab-separated, because the destination is a pasteboard and every spreadsheet
// on the machine reads tabs as columns. A cell containing a tab or a newline
// would silently become extra columns and rows.
func TestClipboardTextCannotForgeColumns(t *testing.T) {
	doc := Document{
		Fields: []string{"name", "note"},
		Rows: []runner.Row{
			{"name": "Widget", "note": "has\ta tab\nand a newline"},
		},
	}
	lines := strings.Split(strings.TrimRight(doc.Text(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("a newline in a cell created %d lines, want 2:\n%q", len(lines), doc.Text())
	}
	if n := strings.Count(lines[1], "\t"); n != 1 {
		t.Errorf("a tab in a cell created %d separators, want 1: %q", n, lines[1])
	}
}

type fixedUIWriter struct {
	p uiact.Program
}

func (f fixedUIWriter) WriteUIAction(context.Context, synth.UIActRequest) (uiact.Program, error) {
	return f.p, nil
}

func TestAskInfersFrontmostAppWhenAppActionOmitsSlot(t *testing.T) {
	h := newHarness(t, always(goodProgram()))
	h.engine.UIScreen = &fakeScreen{tree: editorTree(), front: "Editor"}
	h.engine.UIWriter = fixedUIWriter{p: uiact.Program{
		App: "Editor",
		Steps: []uiact.Step{{
			Do:     "type",
			Target: uiact.Selector{Role: "AXTextArea", Title: "Message"},
			Text:   "hello",
		}},
	}}
	withRouter(t, h, route.Decision{
		Kind:  route.KindAppAction,
		Slots: map[string]string{"action": "type hello"},
	})

	plan, err := h.engine.Ask(context.Background(), Ask{
		Instruction: "type hello",
		Context:     &screen.Context{AppName: "Editor"},
	})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if plan.Route == nil || plan.Route.Slot("app") != "Editor" {
		t.Fatalf("route app slot = %q, want Editor", plan.Route.Slot("app"))
	}
	if plan.UI == nil || plan.UI.App != "Editor" {
		t.Fatalf("plan.UI.App = %v, want Editor", plan.UI)
	}
}
