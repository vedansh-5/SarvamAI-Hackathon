package engine

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/uiact"
)

func notesWindow() uiact.Tree {
	return uiact.Tree{
		App:    "Notes",
		Window: "Notes — 6 notes",
		Elements: []uiact.Element{
			{Role: "AXStaticText", Title: "Groceries", Enabled: true},
			{Role: "AXTextArea", Title: "Body", Value: "milk, bread, coffee", Enabled: true, Focused: true},
			{Role: "AXGroup", Enabled: true},
		},
	}
}

func TestAskingAboutWhatIsInFrontReadsANativeWindow(t *testing.T) {
	screen := &fakeScreen{tree: notesWindow(), front: "Notes"}
	e := &Engine{UIScreen: screen}

	plan, err := e.readWhatIsInFront(context.Background(), "what does this say")
	if err != nil {
		t.Fatalf("here: %v", err)
	}
	if plan.Task.Title != "Notes — 6 notes" {
		t.Errorf("title = %q", plan.Task.Title)
	}
	if len(plan.Rows) != 2 {
		t.Fatalf("got %d rows, want the two that carry something: %+v", len(plan.Rows), plan.Rows)
	}
	if plan.NeedsCommit() {
		t.Error("reading a window asked to change something")
	}
}

// A question about content is the one case where what a control holds is the
// answer, unlike acting on it (D-058).
func TestReadingAWindowKeepsTheTextTheQuestionIsAbout(t *testing.T) {
	rows := windowRows(notesWindow())

	var found bool
	for _, r := range rows {
		if r["text"] == "milk, bread, coffee" {
			found = true
		}
	}
	if !found {
		t.Errorf("the text the question is about was dropped: %+v", rows)
	}
}

func TestABrowserInFrontIsLeftToThePageTier(t *testing.T) {
	screen := &fakeScreen{tree: notesWindow(), front: "Google Chrome"}
	e := &Engine{UIScreen: screen}

	if _, err := e.readWhatIsInFront(context.Background(), "what is this page about"); err == nil {
		t.Fatal("a browser page was answered from the window tier")
	}
	if screen.captures != 0 {
		t.Errorf("captured a window %d time(s) for a browser page", screen.captures)
	}
}
