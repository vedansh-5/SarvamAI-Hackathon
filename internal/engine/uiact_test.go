package engine

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type stubUIWriter struct{}

func (stubUIWriter) WriteUIAction(context.Context, synth.UIActRequest) (uiact.Program, error) {
	return uiact.Program{}, nil
}

func uiPlan() *Plan {
	program := uiact.Program{App: "Editor", Steps: []uiact.Step{{
		Do:     "type",
		Target: uiact.Selector{Role: "AXTextArea", Title: "Message"},
		Text:   "hello",
	}}}

	return &Plan{UI: &UIPlan{
		App:     "Editor",
		Preview: uiact.Preview(program),
		program: program,
	}}
}

func TestCommitReadsTheWindowAgainBeforeActing(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: f, UIWriter: stubUIWriter{}}

	if _, err := e.runUI(context.Background(), uiPlan()); err != nil {
		t.Fatalf("runUI: %v", err)
	}
	// Once before acting, to check the control is still there, and once after,
	// to see what the action did.
	if f.captures != 2 {
		t.Errorf("read the window %d time(s), want 2", f.captures)
	}
	if f.acts != 1 {
		t.Errorf("acted %d time(s), want 1", f.acts)
	}
}

// A receipt that says a thing was done has to rest on something. The window is
// read back and the typed text either turned up in it or it did not.
func TestCommitConfirmsTheTextArrived(t *testing.T) {
	landed := editorTree()
	landed.Elements[0].Value = "hello"

	f := &fakeScreen{tree: editorTree(), afterwards: &landed}
	e := &Engine{UIScreen: f, UIWriter: stubUIWriter{}}

	receipt, err := e.runUI(context.Background(), uiPlan())
	if err != nil {
		t.Fatalf("runUI: %v", err)
	}
	if !receipt.Verified {
		t.Error("the text arrived in the window and the receipt did not say so")
	}
}

func TestCommitDoesNotClaimAnActionThatChangedNothing(t *testing.T) {
	f := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: f, UIWriter: stubUIWriter{}}

	receipt, err := e.runUI(context.Background(), uiPlan())
	if err != nil {
		t.Fatalf("runUI: %v", err)
	}
	if receipt.Verified {
		t.Error("nothing in the window changed and the receipt claimed it was done")
	}
}

func TestCommitRefusesWhenTheControlIsGone(t *testing.T) {
	gone := editorTree()
	gone.Elements = nil

	f := &fakeScreen{tree: gone}
	e := &Engine{UIScreen: f, UIWriter: stubUIWriter{}}

	if _, err := e.runUI(context.Background(), uiPlan()); err == nil {
		t.Fatal("acted on a window that no longer holds the control")
	}
}
