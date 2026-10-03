package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Schedule struct {
	ID   int64
	What string
	Rule string
	Next time.Time
}

func (s *Store) AddSchedule(ctx context.Context, what, rule string, next time.Time) (int64, error) {
	what = strings.TrimSpace(what)
	if what == "" || strings.TrimSpace(rule) == "" {
		return 0, fmt.Errorf("store: a schedule needs something to do and a time to do it")
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules (what, rule, next, created_at) VALUES (?, ?, ?, ?)`,
		what, rule, next.Unix(), time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("store: add schedule: %w", err)
	}
	return res.LastInsertId()
}

func (s *Store) schedules(ctx context.Context, where string, args ...any) ([]Schedule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, what, rule, next FROM schedules `+where+` ORDER BY next`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: read schedules: %w", err)
	}
	defer rows.Close()

	var out []Schedule
	for rows.Next() {
		var sch Schedule
		var next int64
		if err := rows.Scan(&sch.ID, &sch.What, &sch.Rule, &next); err != nil {
			return nil, fmt.Errorf("store: read schedule: %w", err)
		}
		sch.Next = time.Unix(next, 0)
		out = append(out, sch)
	}
	return out, rows.Err()
}

func (s *Store) Schedules(ctx context.Context) ([]Schedule, error) {
	return s.schedules(ctx, "")
}

func (s *Store) DueSchedules(ctx context.Context, now time.Time) ([]Schedule, error) {
	return s.schedules(ctx, "WHERE next <= ?", now.Unix())
}

func (s *Store) MoveSchedule(ctx context.Context, id int64, next time.Time) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE schedules SET next = ? WHERE id = ?`, next.Unix(), id); err != nil {
		return fmt.Errorf("store: move schedule: %w", err)
	}
	return nil
}

func (s *Store) DropSchedule(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: drop schedule: %w", err)
	}
	return nil
}

func (s *Store) CancelSchedules(ctx context.Context, what string) (int, error) {
	what = strings.ToLower(strings.TrimSpace(what))
	if what == "" {
		return 0, nil
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM schedules WHERE lower(what) = ? OR lower(what) LIKE ?`, what, "%"+what+"%")
	if err != nil {
		return 0, fmt.Errorf("store: cancel schedule: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
