package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestADatabaseFromBeforeTheStepTimesKeepsRecordingRuns(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v19.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `
CREATE TABLE runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, app TEXT NOT NULL DEFAULT '',
	shape TEXT NOT NULL DEFAULT '', mode TEXT NOT NULL DEFAULT '', steps INTEGER NOT NULL DEFAULT 0,
	replayed INTEGER NOT NULL DEFAULT 0, calls INTEGER NOT NULL DEFAULT 0, finished INTEGER NOT NULL DEFAULT 0,
	failed TEXT NOT NULL DEFAULT '', tokens_in INTEGER NOT NULL DEFAULT 0, tokens_out INTEGER NOT NULL DEFAULT 0);
INSERT INTO runs (at, app, steps) VALUES (1, 'Postman', 8);
PRAGMA user_version = 19;`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening a version-19 database failed: %v", err)
	}
	defer s.Close()

	if err := s.RecordTask(ctx, Run{App: "Preview", Steps: 3, Clicked: 2, NextPressed: 1, Seen: 0}); err != nil {
		t.Fatalf("a run could not be kept after the upgrade: %v", err)
	}
	got, err := s.RecentTasks(ctx, 5)
	if err != nil {
		t.Fatalf("the runs could not be read after the upgrade: %v", err)
	}
	if len(got) != 2 || got[0].App != "Preview" || got[0].Clicked != 2 || got[0].NextPressed != 1 || got[1].App != "Postman" {
		t.Errorf("after the upgrade: %+v", got)
	}
}
