package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

func TestAClickIsNotedOnlyWhileWatchingIsOn(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	e := &Engine{Clicks: st, ClickAt: func(int, int) (uiact.Noticed, bool) {
		return uiact.Noticed{App: "Mail", Role: "AXButton", Title: "Archive"}, true
	}}

	if kept, err := e.NoteClick(ctx, 10, 10); err != nil || !kept {
		t.Fatalf("kept = %v, err = %v; want the click noted with watching on by default", kept, err)
	}
	if err := st.SetWatching(ctx, false); err != nil {
		t.Fatal(err)
	}
	if kept, _ := e.NoteClick(ctx, 10, 10); kept {
		t.Error("a click was noted with watching switched off")
	}

	got, err := e.NotedClicks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Archive" || got[0].App != "Mail" {
		t.Errorf("noted %+v, want the one Archive click in Mail", got)
	}
}

func TestAClickOnSomethingThatIsNotAControlIsNotNoted(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	e := &Engine{Clicks: st, ClickAt: func(int, int) (uiact.Noticed, bool) { return uiact.Noticed{}, false }}
	if kept, _ := e.NoteClick(ctx, 10, 10); kept {
		t.Error("a click on something that is not a control was noted")
	}
}
