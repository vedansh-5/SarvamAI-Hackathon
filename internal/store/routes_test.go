package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/vedansh/vans/internal/route"
)

func decision() route.Decision {
	return route.Decision{
		Kind:  route.KindWebExtract,
		Slots: map[string]string{"url": "{u0}", "intent": "top {n0} stories"},
		Why:   "it names a page and what to read off it",
	}
}

func routeStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "vans.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestADecisionSurvivesBeingStored(t *testing.T) {
	ctx := context.Background()
	s := routeStore(t)

	const shape = "the top {n0} stories from {u0}"
	if err := s.SaveRoute(ctx, shape, decision(), "test-model"); err != nil {
		t.Fatal(err)
	}

	got, ok, err := s.Route(ctx, shape)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("saved decision was not found")
	}
	if got.Kind != route.KindWebExtract {
		t.Errorf("kind = %q", got.Kind)
	}
	if got.Slots["intent"] != "top {n0} stories" {
		t.Errorf("templates did not survive: %#v", got.Slots)
	}
	if got.Why == "" {
		t.Error("the model's reasoning was dropped")
	}
}

func TestAnUnknownShapeIsAMissNotAnError(t *testing.T) {
	_, ok, err := routeStore(t).Route(context.Background(), "never seen")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("reported a hit for a shape never stored")
	}
}

// A shape is already the generalisation, so a second decision for it means the
// first was wrong. Replaced, not accumulated.
func TestSavingAgainReplacesRatherThanAccumulates(t *testing.T) {
	ctx := context.Background()
	s := routeStore(t)
	const shape = "do {s0} in {s1}"

	if err := s.SaveRoute(ctx, shape, decision(), "a"); err != nil {
		t.Fatal(err)
	}
	second := route.Decision{
		Kind:  route.KindAppAction,
		Slots: map[string]string{"app": "{s1}", "action": "{s0}"},
	}
	if err := s.SaveRoute(ctx, shape, second, "b"); err != nil {
		t.Fatal(err)
	}

	got, _, err := s.Route(ctx, shape)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != route.KindAppAction {
		t.Errorf("kind = %q, want the replacement", got.Kind)
	}

	n, err := s.RouteCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("%d rows for one shape, want 1", n)
	}
}

func TestForgettingARouteRemovesIt(t *testing.T) {
	ctx := context.Background()
	s := routeStore(t)
	const shape = "the top {n0} stories from {u0}"

	if err := s.SaveRoute(ctx, shape, decision(), "m"); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetRoute(ctx, shape); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Route(ctx, shape); ok {
		t.Fatal("decision survived being forgotten")
	}
}

// Corruption should degrade to a miss. The router decides again, which is the
// same recovery path — failing the task instead would be a worse outcome for
// something the runtime can trivially rebuild.
func TestAnUnreadableDecisionDegradesToAMiss(t *testing.T) {
	ctx := context.Background()
	s := routeStore(t)
	const shape = "the top {n0} stories from {u0}"

	if err := s.SaveRoute(ctx, shape, decision(), "m"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE routes SET decision = ? WHERE shape = ?`, "{not json", shape); err != nil {
		t.Fatal(err)
	}

	_, ok, err := s.Route(ctx, shape)
	if err != nil {
		t.Fatalf("corruption became an error instead of a miss: %v", err)
	}
	if ok {
		t.Fatal("unreadable decision was reported as a hit")
	}
	if n, _ := s.RouteCount(ctx); n != 0 {
		t.Errorf("%d unreadable rows left behind, want them dropped", n)
	}
}

// A database written before this table existed must gain it on open rather
// than failing every routing lookup for the life of the install.
func TestAnOlderDatabaseGainsTheRoutesTable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")

	// Build a version-1 database by hand: programs only, no routes.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `
CREATE TABLE programs (
	id INTEGER PRIMARY KEY AUTOINCREMENT, origin TEXT NOT NULL,
	signature TEXT NOT NULL, intent_hash TEXT NOT NULL, intent TEXT NOT NULL,
	program TEXT NOT NULL, model TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL, last_success_at INTEGER NOT NULL DEFAULT 0,
	runs INTEGER NOT NULL DEFAULT 0, failures INTEGER NOT NULL DEFAULT 0,
	consecutive_failures INTEGER NOT NULL DEFAULT 0,
	UNIQUE (origin, signature, intent_hash, program));
PRAGMA user_version = 1;`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("opening a version-1 database failed: %v", err)
	}
	defer s.Close()

	if err := s.SaveRoute(ctx, "shape {u0}", decision(), "m"); err != nil {
		t.Fatalf("routes table was not created by the migration: %v", err)
	}
}
