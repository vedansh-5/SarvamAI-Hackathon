package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOnlyTheLastFewInstructionsAreKept(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, in := range []string{"first thing", "second thing", "third thing", "fourth thing"} {
		if err := s.RecordInstruction(ctx, in, "web.search"); err != nil {
			t.Fatalf("RecordInstruction(%q): %v", in, err)
		}
	}

	got, err := s.RecentInstructions(ctx)
	if err != nil {
		t.Fatalf("RecentInstructions: %v", err)
	}
	if len(got) != RememberedInstructions {
		t.Fatalf("kept %d instructions, want %d", len(got), RememberedInstructions)
	}
	if got[0].Text != "fourth thing" {
		t.Errorf("most recent is %q, want the last one asked", got[0].Text)
	}
	for _, r := range got {
		if r.Text == "first thing" {
			t.Error("the oldest instruction outlived the cap")
		}
	}
}

func TestAskingTheSameThingAgainDoesNotDuplicateIt(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := s.RecordInstruction(ctx, "the same thing", "web.search"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordInstruction(ctx, "something else", "page.here"); err != nil {
		t.Fatal(err)
	}

	got, err := s.RecentInstructions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("kept %d instructions, want 2: %+v", len(got), got)
	}
}

func TestTheKindIsKeptWithTheInstruction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.RecordInstruction(ctx, "what is this page about", "page.here"); err != nil {
		t.Fatal(err)
	}
	got, err := s.RecentInstructions(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if got[0].Kind != "page.here" {
		t.Errorf("kind = %q, want page.here", got[0].Kind)
	}
}

func TestInstructionsCanBeForgotten(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.RecordInstruction(ctx, "something private", "web.search"); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetInstructions(ctx); err != nil {
		t.Fatalf("ForgetInstructions: %v", err)
	}

	got, err := s.RecentInstructions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("%d instructions survived being forgotten", len(got))
	}
}

func textsOf(t *testing.T, s *Store) map[string]Remembered {
	t.Helper()
	got, err := s.RecentInstructions(context.Background())
	if err != nil {
		t.Fatalf("RecentInstructions: %v", err)
	}
	out := map[string]Remembered{}
	for _, r := range got {
		out[r.Text] = r
	}
	return out
}

func TestAFinishedInstructionOutlivesOnesThatNeverFinished(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.RecordInstruction(ctx, "the one that worked", "web.extract"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishInstruction(ctx, "the one that worked", 2); err != nil {
		t.Fatalf("FinishInstruction: %v", err)
	}
	for _, in := range []string{"failed once", "failed twice", "just asked"} {
		if err := s.RecordInstruction(ctx, in, "app.act"); err != nil {
			t.Fatal(err)
		}
	}

	kept := textsOf(t, s)
	if len(kept) != RememberedInstructions {
		t.Fatalf("kept %d, want the cap of %d: %+v", len(kept), RememberedInstructions, kept)
	}
	if !kept["the one that worked"].Done {
		t.Errorf("the finished instruction was dropped or lost its mark: %+v", kept)
	}
	if _, ok := kept["just asked"]; !ok {
		t.Errorf("the instruction just asked was dropped: %+v", kept)
	}
	if _, ok := kept["failed once"]; ok {
		t.Errorf("the oldest unfinished instruction outlived a finished one's rival: %+v", kept)
	}
}

func TestFinishingKeepsWhatItCostAndSurvivesBeingAskedAgain(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.RecordInstruction(ctx, "top stories", "web.extract"); err != nil {
		t.Fatal(err)
	}
	if got := textsOf(t, s)["top stories"]; got.Done {
		t.Fatalf("marked finished before it finished: %+v", got)
	}
	if err := s.FinishInstruction(ctx, "top stories", 0); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordInstruction(ctx, "top stories", "web.extract"); err != nil {
		t.Fatal(err)
	}

	got := textsOf(t, s)["top stories"]
	if !got.Done || got.Calls != 0 {
		t.Errorf("after finishing with no calls and being asked again: %+v", got)
	}
}

func TestFinishingSomethingNeverAskedKeepsNothing(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.FinishInstruction(ctx, "never asked", 1); err != nil {
		t.Fatal(err)
	}
	if kept := textsOf(t, s); len(kept) != 0 {
		t.Errorf("finishing created a row: %+v", kept)
	}
}

func TestOneInstructionCanBeForgotten(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	for _, in := range []string{"keep me", "forget me"} {
		if err := s.RecordInstruction(ctx, in, "web.search"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ForgetInstruction(ctx, "forget me"); err != nil {
		t.Fatalf("ForgetInstruction: %v", err)
	}

	kept := textsOf(t, s)
	if _, ok := kept["forget me"]; ok || len(kept) != 1 {
		t.Errorf("kept %+v, want only the other one", kept)
	}
}

func TestAnOlderDatabaseLearnsWhichInstructionsFinished(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v15.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `
CREATE TABLE instructions (
	text TEXT PRIMARY KEY, kind TEXT NOT NULL DEFAULT '', used_at INTEGER NOT NULL);
INSERT INTO instructions (text, kind, used_at) VALUES ('asked before the upgrade', 'web.search', 1);
PRAGMA user_version = 15;`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening a version-15 database failed: %v", err)
	}
	defer s.Close()

	if err := s.FinishInstruction(ctx, "asked before the upgrade", 0); err != nil {
		t.Fatalf("the new columns were not added by the migration: %v", err)
	}
	if got := textsOf(t, s)["asked before the upgrade"]; !got.Done {
		t.Errorf("after the migration: %+v", got)
	}
}
