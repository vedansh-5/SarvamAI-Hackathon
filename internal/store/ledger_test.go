package store

import (
	"context"
	"testing"
	"time"
)

func TestTheLedgerRemembersWhatWasDone(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	id, err := s.RecordDeed(ctx, Deed{
		Kind: "restyle", Where: "example.com",
		Asked: "highlight the headings", Did: "Highlight every heading",
		Verified: true, Undo: "restyle",
	})
	if err != nil {
		t.Fatalf("RecordDeed: %v", err)
	}
	if id == 0 {
		t.Fatal("no id came back, so nothing can be undone by reference")
	}

	got, err := s.RecentDeeds(ctx, 10)
	if err != nil {
		t.Fatalf("RecentDeeds: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("read back %d, want 1", len(got))
	}
	if got[0].Where != "example.com" || got[0].Asked != "highlight the headings" {
		t.Errorf("came back wrong: %+v", got[0])
	}
	if !got[0].Verified {
		t.Error("Verified was not kept")
	}
	if !got[0].Undoable() {
		t.Error("a change with an inverse recorded is not being offered as undoable")
	}
	if got[0].At.IsZero() {
		t.Error("no time was recorded")
	}
}

func TestTheNewestChangeComesFirst(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, d := range []Deed{
		{Kind: "page", Did: "older", At: time.Now().Add(-time.Hour)},
		{Kind: "page", Did: "newer", At: time.Now()},
	} {
		if _, err := s.RecordDeed(ctx, d); err != nil {
			t.Fatalf("RecordDeed: %v", err)
		}
	}

	got, err := s.RecentDeeds(ctx, 10)
	if err != nil {
		t.Fatalf("RecentDeeds: %v", err)
	}
	if len(got) != 2 || got[0].Did != "newer" {
		t.Errorf("order is wrong: %+v", got)
	}
}

// A record of everywhere somebody has been is not what this is for.
func TestTheLedgerForgetsTheOldest(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < KeptDeeds+20; i++ {
		if _, err := s.RecordDeed(ctx, Deed{Kind: "page", Did: "one"}); err != nil {
			t.Fatalf("RecordDeed: %v", err)
		}
	}

	got, err := s.RecentDeeds(ctx, KeptDeeds+50)
	if err != nil {
		t.Fatalf("RecentDeeds: %v", err)
	}
	if len(got) != KeptDeeds {
		t.Errorf("kept %d, want %d", len(got), KeptDeeds)
	}
}

// Undoing is recorded so it is not offered twice.
func TestSomethingPutBackIsNoLongerOffered(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	id, err := s.RecordDeed(ctx, Deed{Kind: "restyle", Undo: "restyle"})
	if err != nil {
		t.Fatalf("RecordDeed: %v", err)
	}

	d, found, err := s.Deed(ctx, id)
	if err != nil || !found {
		t.Fatalf("Deed(%d) = %v, %v", id, found, err)
	}
	if !d.Undoable() {
		t.Fatal("it should be undoable before it has been undone")
	}

	if err := s.MarkUndone(ctx, id); err != nil {
		t.Fatalf("MarkUndone: %v", err)
	}

	d, _, err = s.Deed(ctx, id)
	if err != nil {
		t.Fatalf("Deed: %v", err)
	}
	if d.Undoable() {
		t.Error("it is still being offered after being put back")
	}
	if d.UndoneAt.IsZero() {
		t.Error("when it was put back was not recorded")
	}
}

func TestReadingAChangeThatIsNotThere(t *testing.T) {
	s := testStore(t)
	if _, found, err := s.Deed(context.Background(), 9999); found || err != nil {
		t.Errorf("Deed(9999) = %v, %v; want not found and no error", found, err)
	}
}
