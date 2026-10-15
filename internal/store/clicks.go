package store

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

type Click struct {
	At    time.Time `json:"at"`
	App   string    `json:"app"`
	Role  string    `json:"role"`
	Title string    `json:"title"`
}

const watchingKey = "watching"

func (s *Store) Watching(ctx context.Context) (bool, error) {
	v, err := s.Setting(ctx, watchingKey)
	if err != nil || v == "" {
		return true, err
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		return true, nil
	}
	return on, nil
}

func (s *Store) SetWatching(ctx context.Context, on bool) error {
	return s.SetSetting(ctx, watchingKey, strconv.FormatBool(on))
}

func (s *Store) NoteClick(ctx context.Context, c Click) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO clicks (at, app, role, title) VALUES (?, ?, ?, ?)`,
		c.At.UnixMilli(), c.App, c.Role, c.Title)
	if err != nil {
		return fmt.Errorf("store: note click: %w", err)
	}
	return nil
}

func (s *Store) Clicks(ctx context.Context, since time.Time) ([]Click, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT at, app, role, title FROM clicks WHERE at >= ? ORDER BY at, id`, since.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("store: read clicks: %w", err)
	}
	defer rows.Close()
	var out []Click
	for rows.Next() {
		var c Click
		var at int64
		if err := rows.Scan(&at, &c.App, &c.Role, &c.Title); err != nil {
			return nil, err
		}
		c.At = time.UnixMilli(at)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ForgetClicksBefore(ctx context.Context, before time.Time) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM clicks WHERE at < ?`, before.UnixMilli()); err != nil {
		return fmt.Errorf("store: forget old clicks: %w", err)
	}
	return nil
}

func (s *Store) ClearClicks(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM clicks`); err != nil {
		return fmt.Errorf("store: clear clicks: %w", err)
	}
	return nil
}
