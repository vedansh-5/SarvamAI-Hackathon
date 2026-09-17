package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/runner"
	"github.com/vedansh/vans/internal/uiact"
)

func answerDoc() Document {
	return Document{
		Title:  "Answer",
		Fields: []string{"text"},
		Rows:   []runner.Row{{"text": "on my way"}},
	}
}

func TestAppSinkTypesIntoTheControlItFinds(t *testing.T) {
	screen := &fakeScreen{tree: editorTree()}
	sink := AppSink{Screen: screen}

	if _, err := sink.Write(context.Background(), answerDoc()); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if screen.acts != 1 {
		t.Errorf("acts = %d, want the control to be addressed directly", screen.acts)
	}
	if len(screen.typed) != 0 {
		t.Errorf("fell back to the cursor with a control in hand: %v", screen.typed)
	}
}

// Chromium publishes a window with nothing in it (D-056), so there is no
// control to address. Typing is posted as key events now, which needs no tree
// at all — the destination says "where the cursor is", and that is reachable
// whether or not the application describes itself.
func TestAppSinkTypesWhereTheCursorIsWhenNothingIsPublished(t *testing.T) {
	screen := &fakeScreen{
		tree:  uiact.Tree{App: "Google Chrome", Window: "Google Chrome"},
		front: "Google Chrome",
	}
	sink := AppSink{Screen: screen}

	receipt, err := sink.Write(context.Background(), answerDoc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if len(screen.typed) != 1 || !strings.Contains(screen.typed[0], "on my way") {
		t.Errorf("typed = %v, want the answer posted where the cursor is", screen.typed)
	}
	if screen.acts != 0 {
		t.Errorf("addressed a control that was never published")
	}
	if receipt.Title != "Google Chrome" {
		t.Errorf("receipt names %q", receipt.Title)
	}
}

func TestAppSinkRefusesToTypeIntoItself(t *testing.T) {
	screen := &fakeScreen{tree: editorTree(), front: "VANS"}
	sink := AppSink{Screen: screen}

	_, err := sink.Write(context.Background(), answerDoc())
	if err == nil {
		t.Fatal("typed into VANS itself")
	}
	if !strings.Contains(err.Error(), "nothing behind it") {
		t.Errorf("err = %v", err)
	}
}
