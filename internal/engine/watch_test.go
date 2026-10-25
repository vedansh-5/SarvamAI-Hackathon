package engine

import (
	"context"
	"path/filepath"
	"testing"
	"time"

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

func TestAFoundHabitCanBeSavedAsARoutine(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	c := &clicking{at: time.Now().Add(-3 * time.Hour)}
	for i := 0; i < 3; i++ {
		c.do("Mail", "Archive", "Next", "Flag")
		c.pause(time.Hour)
	}
	for _, click := range c.out {
		if err := st.NoteClick(ctx, click); err != nil {
			t.Fatal(err)
		}
	}

	e := &Engine{Clicks: st, Routines: st}
	if open, err := e.LookForHabits(ctx); err != nil || open != 1 {
		t.Fatalf("open = %d, err = %v; want the one habit found", open, err)
	}
	offers, err := e.HabitOffers(ctx)
	if err != nil || len(offers) != 1 {
		t.Fatalf("offers = %+v, err = %v", offers, err)
	}
	if offers[0].Routine != "archive next flag" {
		t.Errorf("suggested routine name %q", offers[0].Routine)
	}

	if _, err := e.SettleHabit(ctx, offers[0].Key, SaveHabit, "tidy mail"); err != nil {
		t.Fatal(err)
	}
	got, found, err := st.Routine(ctx, "tidy mail")
	if err != nil || !found || got != "In Mail, click Archive, then click Next, then click Flag" {
		t.Errorf("routine = %q (found %v, err %v)", got, found, err)
	}
	if offers, _ := e.HabitOffers(ctx); len(offers) != 0 {
		t.Errorf("a saved habit is still offered: %+v", offers)
	}
}

func TestAppsComingForwardAndTheMacsMomentsAreNotedButNotAsClicks(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e := &Engine{Clicks: st}

	for _, m := range []struct{ kind, app string }{
		{"unlock", ""}, {"front", "Mail"}, {"front", "1Password 7"}, {"front", "VANS"}, {"lock", ""}, {"dance", ""},
	} {
		if _, err := e.NoteMoment(ctx, m.kind, m.app); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := e.NotedClicks(ctx)
	if len(got) != 3 || got[0].Title != "unlock" || got[1].App != "Mail" || got[2].Title != "lock" {
		t.Fatalf("noted %+v, want unlock, Mail in front, lock; and nothing for a password manager, Vspk or an unknown kind", got)
	}
	if got[1].Clicked() || got[0].Clicked() {
		t.Error("an app coming forward or the Mac unlocking was counted as a click")
	}
}
