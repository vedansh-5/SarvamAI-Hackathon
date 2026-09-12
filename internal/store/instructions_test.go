package store

import (
	"context"
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
