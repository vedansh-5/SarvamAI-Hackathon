package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vedansh/vans/internal/route"
)

// This file implements route.Cache. The decision is stored as the JSON the
// model produced rather than as columns, because its shape belongs to the
// route package and duplicating it here would mean migrating the database
// every time a kind gains a slot.

// Route returns the decision cached for a shape.
func (s *Store) Route(ctx context.Context, shape string) (route.Decision, bool, error) {
	var raw, specs string
	err := s.db.QueryRowContext(ctx,
		`SELECT decision, specs FROM routes WHERE shape = ?`, shape).Scan(&raw, &specs)
	if errors.Is(err, sql.ErrNoRows) {
		return route.Decision{}, false, nil
	}
	if err != nil {
		return route.Decision{}, false, fmt.Errorf("store: look up route: %w", err)
	}

	if specs != route.SpecsFingerprint() {
		if err := s.ForgetRoute(ctx, shape); err != nil {
			return route.Decision{}, false, err
		}
		return route.Decision{}, false, nil
	}

	var d route.Decision
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		// Unreadable rather than absent. Drop it instead of failing the task:
		// the router will decide again, which is the same recovery as a miss.
		if err := s.ForgetRoute(ctx, shape); err != nil {
			return route.Decision{}, false, err
		}
		return route.Decision{}, false, nil
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE routes SET uses = uses + 1, last_used_at = ? WHERE shape = ?`,
		time.Now().Unix(), shape); err != nil {
		return route.Decision{}, false, fmt.Errorf("store: record route use: %w", err)
	}
	return d, true, nil
}

// SaveRoute stores a decision against a shape.
//
// One decision per shape, replaced rather than accumulated — unlike programs
// (D-029), which keep several because a page may legitimately serve variants
// that one signature collapses. A shape has no variants: it is already the
// generalisation, so a second decision for it means the first was wrong.
func (s *Store) SaveRoute(ctx context.Context, shape string, d route.Decision, model string) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("store: encode decision: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO routes (shape, decision, model, created_at, last_used_at, uses, specs)
VALUES (?, ?, ?, ?, ?, 0, ?)
ON CONFLICT(shape) DO UPDATE SET
	decision   = excluded.decision,
	model      = excluded.model,
	created_at = excluded.created_at,
	specs      = excluded.specs`,
		shape, string(raw), model, time.Now().Unix(), 0, route.SpecsFingerprint())
	if err != nil {
		return fmt.Errorf("store: save route: %w", err)
	}
	return nil
}

// ForgetRoute drops a shape's decision.
func (s *Store) ForgetRoute(ctx context.Context, shape string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM routes WHERE shape = ?`, shape); err != nil {
		return fmt.Errorf("store: forget route: %w", err)
	}
	return nil
}

// RouteCount reports how many decisions are cached.
func (s *Store) RouteCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routes`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count routes: %w", err)
	}
	return n, nil
}
