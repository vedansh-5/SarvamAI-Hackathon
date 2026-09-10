package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vedansh/vans/internal/appact"
)

// Action returns the program cached for an application and action shape.
func (s *Store) Action(ctx context.Context, app, shape string) (appact.Program, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT program FROM actions WHERE app = ? AND shape = ?`, app, shape).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return appact.Program{}, false, nil
	}
	if err != nil {
		return appact.Program{}, false, fmt.Errorf("store: look up action: %w", err)
	}

	var p appact.Program
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		// Unreadable rather than absent, and dropped rather than fatal: the
		// engine writes it again, which is the same recovery as a miss.
		if err := s.ForgetAction(ctx, app, shape); err != nil {
			return appact.Program{}, false, err
		}
		return appact.Program{}, false, nil
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE actions SET uses = uses + 1, last_used_at = ? WHERE app = ? AND shape = ?`,
		time.Now().Unix(), app, shape); err != nil {
		return appact.Program{}, false, fmt.Errorf("store: record action use: %w", err)
	}
	return p, true, nil
}

// SaveAction stores a program against an application and action shape.
func (s *Store) SaveAction(ctx context.Context, app, shape string, p appact.Program, model string) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("store: encode program: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO actions (app, shape, program, model, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (app, shape) DO UPDATE SET
	program    = excluded.program,
	model      = excluded.model,
	created_at = excluded.created_at,
	uses       = 0`,
		app, shape, string(raw), model, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("store: save action: %w", err)
	}
	return nil
}

// ForgetAction drops a cached program.
func (s *Store) ForgetAction(ctx context.Context, app, shape string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM actions WHERE app = ? AND shape = ?`, app, shape); err != nil {
		return fmt.Errorf("store: forget action: %w", err)
	}
	return nil
}

// ActionCount is how many application programs are cached.
func (s *Store) ActionCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM actions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count actions: %w", err)
	}
	return n, nil
}
