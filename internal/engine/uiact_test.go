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
	if f.captures != 1 {
		t.Errorf("read the window %d time(s) before acting, want 1", f.captures)
	}
	if f.acts != 1 {
		t.Errorf("acted %d time(s), want 1", f.acts)
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
