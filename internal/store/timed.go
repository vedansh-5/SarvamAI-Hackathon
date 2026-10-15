package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type TimedRow struct {
	Key        string
	Trigger    string
	Action     string
	Days       int
	Yes        int
	Skips      int
	Always     bool
	OfferedOn  string
	LaterUntil time.Time
	Stopped    bool
}

func (s *Store) KeepTimed(ctx context.Context, r TimedRow) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO timed (key, trigger, action, days) VALUES (?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET days = excluded.days`,
		r.Key, r.Trigger, r.Action, r.Days)
	if err != nil {
		return fmt.Errorf("store: keep timed habit: %w", err)
	}
	return nil
}

const timedColumns = `key, trigger, action, days, yes, skips, always, offered_on, later_until, stopped`

func scanTimed(row interface{ Scan(...any) error }) (TimedRow, error) {
	var r TimedRow
	var later int64
	err := row.Scan(&r.Key, &r.Trigger, &r.Action, &r.Days, &r.Yes, &r.Skips, &r.Always, &r.OfferedOn, &later, &r.Stopped)
	if later > 0 {
		r.LaterUntil = time.Unix(later, 0)
	}
	return r, err
}

func (s *Store) TimedRows(ctx context.Context) ([]TimedRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+timedColumns+` FROM timed WHERE stopped = 0 ORDER BY days DESC, key`)
	if err != nil {
		return nil, fmt.Errorf("store: read timed habits: %w", err)
	}
	defer rows.Close()
	var out []TimedRow
	for rows.Next() {
		r, err := scanTimed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) TimedRow(ctx context.Context, key string) (TimedRow, bool, error) {
	r, err := scanTimed(s.db.QueryRowContext(ctx, `SELECT `+timedColumns+` FROM timed WHERE key = ?`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return TimedRow{}, false, nil
	}
	return r, err == nil, err
}

func (s *Store) PutTimed(ctx context.Context, r TimedRow) error {
	var later int64
	if !r.LaterUntil.IsZero() {
		later = r.LaterUntil.Unix()
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE timed SET yes = ?, skips = ?, always = ?, offered_on = ?, later_until = ?, stopped = ? WHERE key = ?`,
		r.Yes, r.Skips, r.Always, r.OfferedOn, later, r.Stopped, r.Key)
	if err != nil {
		return fmt.Errorf("store: save timed habit: %w", err)
	}
	return nil
}

func (s *Store) ClearTimed(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM timed WHERE yes = 0 AND always = 0`); err != nil {
		return fmt.Errorf("store: clear timed habits: %w", err)
	}
	return nil
}

func (s *Store) AllTimed(ctx context.Context) ([]TimedRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+timedColumns+` FROM timed ORDER BY days DESC, key`)
	if err != nil {
		return nil, fmt.Errorf("store: read timed habits: %w", err)
	}
	defer rows.Close()
	var out []TimedRow
	for rows.Next() {
		r, err := scanTimed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
