package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestClicksAreKeptInOrderAndOldOnesGo(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now()
	old := Click{At: now.Add(-8 * 24 * time.Hour), App: "Mail", Role: "AXButton", Title: "Archive"}
	recent := Click{At: now.Add(-time.Hour), App: "Mail", Role: "AXButton", Title: "Archive"}
	latest := Click{At: now, App: "Mail", Role: "AXButton", Title: "Next"}
	for _, c := range []Click{latest, old, recent} {
		if err := s.NoteClick(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ForgetClicksBefore(ctx, now.Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Clicks(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Title != "Archive" || got[1].Title != "Next" {
		t.Fatalf("got %+v, want the recent Archive then Next, and the eight-day-old one gone", got)
	}

	if err := s.ClearClicks(ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Clicks(ctx, time.Time{}); len(got) != 0 {
		t.Errorf("%d click(s) left after clearing", len(got))
	}
}

func TestWatchingIsOnUntilSwitchedOff(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if on, _ := s.Watching(ctx); !on {
		t.Error("watching was off on a fresh install, want on by default")
	}
	if err := s.SetWatching(ctx, false); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.Watching(ctx); on {
		t.Error("watching stayed on after being switched off")
	}
}
