package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/vedansh/vans/internal/uiact"
)

func traceProgram() uiact.Program {
	return uiact.Program{App: "TextEdit", Steps: []uiact.Step{
		{Do: "click", Target: uiact.Selector{Role: "AXTextArea", Title: "Document"}},
		{Do: "type", Target: uiact.Selector{Role: "AXTextArea", Title: "Document"}, Text: "{s0}"},
		{Do: "press", Key: "Enter"},
	}}
}

const traceShape = "type {s0} into the document"

func TestDriveTraceRoundTrips(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, nil); err != nil || found {
		t.Fatalf("found=%v err=%v before anything was saved", found, err)
	}

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "test-model"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	got, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, map[string]string{"s0": "hello"})
	if err != nil || !found {
		t.Fatalf("found=%v err=%v after saving", found, err)
	}
	if len(got.Steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(got.Steps))
	}
	if got.Steps[1].Text != "{s0}" {
		t.Errorf("step 2 text = %q, want the placeholder back", got.Steps[1].Text)
	}

	n, err := s.DriveTraceCount(ctx)
	if err != nil || n != 1 {
		t.Fatalf("count = %d, err = %v, want 1", n, err)
	}
}

// A templated trace is the one worth having: it answers for any values of its
// shape, which is the whole reason the shape is the key.
func TestTemplatedTraceAnswersForAnyLiterals(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	for _, value := range []string{"hello", "goodbye", ""} {
		if _, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, map[string]string{"s0": value}); err != nil || !found {
			t.Errorf("literals %q: found=%v err=%v, want a hit", value, found, err)
		}
	}
}

// A trace that could not be templated is pinned to the values it was recorded
// with. Replaying it for different values would type yesterday's text, which is
// the one failure no downstream check would catch.
func TestPinnedTraceOnlyAnswersForItsOwnLiterals(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	recorded := map[string]string{"s0": "a"}
	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), recorded, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	if _, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, map[string]string{"s0": "a"}); err != nil || !found {
		t.Errorf("same literals: found=%v err=%v, want a hit", found, err)
	}
	if _, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, map[string]string{"s0": "b"}); err != nil || found {
		t.Errorf("different literals: found=%v err=%v, want a miss", found, err)
	}
	if _, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, nil); err != nil || found {
		t.Errorf("no literals: found=%v err=%v, want a miss", found, err)
	}
}

// A pin names only the values that could not be lifted out of the trace. Every
// other value in the request is a placeholder, filled in fresh, so a request that
// carries more than the pin names still matches. Requiring the two sets to be
// equal made a pinned trace unfindable, which made the whole mechanism dead.
func TestAPinConstrainsOnlyTheValuesItNames(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	const shape = "close tab {n0} in {s0}"
	if err := s.SaveDriveTrace(ctx, "Chrome", shape, traceProgram(),
		map[string]string{"n0": "2"}, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	if _, found, err := s.DriveTrace(ctx, "Chrome", shape,
		map[string]string{"n0": "2", "s0": "Reading List"}); err != nil || !found {
		t.Errorf("found=%v err=%v; the pinned value matches, so this should hit", found, err)
	}
	if _, found, err := s.DriveTrace(ctx, "Chrome", shape,
		map[string]string{"n0": "5", "s0": "Reading List"}); err != nil || found {
		t.Errorf("found=%v err=%v; the pinned value differs, so this should miss", found, err)
	}
	if _, found, err := s.DriveTrace(ctx, "Chrome", shape,
		map[string]string{"s0": "Reading List"}); err != nil || found {
		t.Errorf("found=%v err=%v; the pinned value is absent, so this should miss", found, err)
	}
}

func TestSavingATraceReplacesTheOldOne(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	shorter := uiact.Program{App: "TextEdit", Steps: traceProgram().Steps[:1]}
	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, shorter, nil, "m"); err != nil {
		t.Fatalf("SaveDriveTrace again: %v", err)
	}

	got, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, nil)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(got.Steps) != 1 {
		t.Errorf("got %d steps, want the replacement's 1", len(got.Steps))
	}
	if n, _ := s.DriveTraceCount(ctx); n != 1 {
		t.Errorf("count = %d, want one row per key", n)
	}
}

func TestATraceIsForgottenAfterTwoDivergences(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	dropped, err := s.DriveTraceDiverged(ctx, "TextEdit", traceShape)
	if err != nil {
		t.Fatalf("first divergence: %v", err)
	}
	if dropped {
		t.Fatal("dropped after one divergence, want it kept for a second chance")
	}
	if _, found, _ := s.DriveTrace(ctx, "TextEdit", traceShape, nil); !found {
		t.Fatal("trace gone after one divergence")
	}

	dropped, err = s.DriveTraceDiverged(ctx, "TextEdit", traceShape)
	if err != nil {
		t.Fatalf("second divergence: %v", err)
	}
	if !dropped {
		t.Errorf("dropped = false after %d divergences, want true", MaxDivergences)
	}
	if _, found, _ := s.DriveTrace(ctx, "TextEdit", traceShape, nil); found {
		t.Error("trace still there after being dropped")
	}
}

// The divergence count exists to find a trace that is reliably wrong. Without a
// reset, one that works most of the time would still be dropped in the end.
func TestAWorkingReplayClearsTheDivergenceCount(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	if _, err := s.DriveTraceDiverged(ctx, "TextEdit", traceShape); err != nil {
		t.Fatalf("divergence: %v", err)
	}
	if err := s.DriveTraceWorked(ctx, "TextEdit", traceShape); err != nil {
		t.Fatalf("DriveTraceWorked: %v", err)
	}

	dropped, err := s.DriveTraceDiverged(ctx, "TextEdit", traceShape)
	if err != nil {
		t.Fatalf("divergence after a good replay: %v", err)
	}
	if dropped {
		t.Error("dropped on the first divergence after a good replay; the count did not reset")
	}
}

func TestDivergingOnATraceThatIsNotThereIsNotAnError(t *testing.T) {
	s := testStore(t)

	dropped, err := s.DriveTraceDiverged(context.Background(), "TextEdit", "never seen")
	if err != nil {
		t.Fatalf("DriveTraceDiverged on a missing key: %v", err)
	}
	if dropped {
		t.Error("dropped = true for a trace that was never stored")
	}
}

// A trace and a single-step window action are keyed the same way and must not be
// confused for one another.
func TestTracesAndWindowActionsAreSeparate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "m"); err != nil {
		t.Fatalf("SaveDriveTrace: %v", err)
	}

	if _, found, err := s.WindowAction(ctx, "TextEdit", traceShape); err != nil || found {
		t.Errorf("WindowAction found=%v err=%v, want a miss", found, err)
	}
}

// Every install that exists today is at version 8 and has no drive_traces. An
// upgrade that does not create it would turn every drive run into a failed
// lookup for the life of that install.
func TestAnOlderDatabaseGainsTheDriveTracesTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v8.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `
CREATE TABLE window_actions (
	app TEXT NOT NULL, shape TEXT NOT NULL, program TEXT NOT NULL,
	model TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL,
	last_used_at INTEGER NOT NULL DEFAULT 0, uses INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (app, shape));
PRAGMA user_version = 8;`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening a version-8 database failed: %v", err)
	}
	defer s.Close()

	if err := s.SaveDriveTrace(ctx, "TextEdit", traceShape, traceProgram(), nil, "m"); err != nil {
		t.Fatalf("drive_traces was not created by the migration: %v", err)
	}
	if _, found, err := s.DriveTrace(ctx, "TextEdit", traceShape, nil); err != nil || !found {
		t.Fatalf("found=%v err=%v after the migration", found, err)
	}
}
