package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type countingRunner struct {
	captures int
	runs     int
	tree     string
}

func (c *countingRunner) Run(_ context.Context, script string) ([]byte, error) {
	if strings.Contains(script, "walk(win") {
		c.captures++
		return []byte(c.tree), nil
	}
	c.runs++
	return []byte(`{"ok":true,"verified":true}`), nil
}

type stubUIWriter struct{}

func (stubUIWriter) WriteUIAction(context.Context, synth.UIActRequest) (uiact.Program, error) {
	return uiact.Program{}, nil
}

const oneTextArea = `{"ok":true,"value":{"app":"Editor","window":"Editor","elements":[
  {"role":"AXTextArea","title":"Message","enabled":true,"path":[0,1]}
]}}`

func uiPlanFor(taken time.Time) *Plan {
	tree := uiact.Tree{
		App:    "Editor",
		Window: "Editor",
		Elements: []uiact.Element{{
			Role: "AXTextArea", Title: "Message", Enabled: true, Path: []int{0, 1},
		}},
	}
	program := uiact.Program{App: "Editor", Steps: []uiact.Step{{
		Do:     "type",
		Target: uiact.Selector{Role: "AXTextArea", Title: "Message"},
		Text:   "hello",
	}}}

	return &Plan{UI: &UIPlan{
		App:     "Editor",
		Preview: uiact.Preview(program),
		program: program,
		tree:    tree,
		taken:   taken,
	}}
}

// Preparing already read the window. Reading it again moments later is another
// pass of Apple events over a tree that has not moved.
func TestCommitReusesAFreshCapture(t *testing.T) {
	r := &countingRunner{tree: oneTextArea}
	e := &Engine{UIRunner: r, UIWriter: stubUIWriter{}}

	if _, err := e.runUI(context.Background(), uiPlanFor(time.Now())); err != nil {
		t.Fatalf("runUI: %v", err)
	}
	if r.captures != 0 {
		t.Errorf("read the window again %d time(s) for a plan just made", r.captures)
	}
	if r.runs != 1 {
		t.Errorf("ran the program %d time(s), want 1", r.runs)
	}
}

// Left sitting, the window is no longer the one that was confirmed, so it is
// read again rather than trusted.
func TestCommitReReadsAStaleCapture(t *testing.T) {
	r := &countingRunner{tree: oneTextArea}
	e := &Engine{UIRunner: r, UIWriter: stubUIWriter{}}

	stale := time.Now().Add(-UIFreshness - time.Second)
	if _, err := e.runUI(context.Background(), uiPlanFor(stale)); err != nil {
		t.Fatalf("runUI: %v", err)
	}
	if r.captures != 1 {
		t.Errorf("read the window %d time(s) for a stale plan, want 1", r.captures)
	}
}
