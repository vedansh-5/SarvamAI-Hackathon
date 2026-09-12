package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

func (s *Store) WindowAction(ctx context.Context, app, shape string) (uiact.Program, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx,
		`SELECT program FROM window_actions WHERE app = ? AND shape = ?`, app, shape).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return uiact.Program{}, false, nil
	}
	if err != nil {
		return uiact.Program{}, false, fmt.Errorf("store: look up window action: %w", err)
	}

	var p uiact.Program
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		if err := s.ForgetWindowAction(ctx, app, shape); err != nil {
			return uiact.Program{}, false, err
		}
		return uiact.Program{}, false, nil
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE window_actions SET uses = uses + 1, last_used_at = ? WHERE app = ? AND shape = ?`,
		time.Now().Unix(), app, shape); err != nil {
		return uiact.Program{}, false, fmt.Errorf("store: record window action use: %w", err)
	}
	return p, true, nil
}

func (s *Store) SaveWindowAction(ctx context.Context, app, shape string, p uiact.Program, model string) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("store: encode window program: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO window_actions (app, shape, program, model, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (app, shape) DO UPDATE SET
	program    = excluded.program,
	model      = excluded.model,
	created_at = excluded.created_at,
	uses       = 0`,
		app, shape, string(raw), model, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("store: save window action: %w", err)
	}
	return nil
}

func (s *Store) ForgetWindowAction(ctx context.Context, app, shape string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM window_actions WHERE app = ? AND shape = ?`, app, shape); err != nil {
		return fmt.Errorf("store: forget window action: %w", err)
	}
	return nil
}

func (s *Store) WindowActionCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM window_actions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count window actions: %w", err)
	}
	return n, nil
}
