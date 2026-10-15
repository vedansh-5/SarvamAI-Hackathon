package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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

const (
	RoleFront  = "front"
	RoleMoment = "moment"
	MacApp     = "Mac"
)

func (c Click) Clicked() bool { return c.Role != RoleFront && c.Role != RoleMoment }

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
	if _, err := s.db.ExecContext(ctx, `DELETE FROM habits WHERE state = ''`); err != nil {
		return fmt.Errorf("store: clear habits: %w", err)
	}
	return s.ClearTimed(ctx)
}

type Habit struct {
	Key   string  `json:"key"`
	Name  string  `json:"name"`
	Steps []Click `json:"steps"`
	Times int     `json:"times"`
}

const (
	HabitOpen      = ""
	HabitSaved     = "saved"
	HabitShown     = "shown"
	HabitDismissed = "dismissed"
)

func (s *Store) KeepHabit(ctx context.Context, h Habit) (bool, error) {
	steps, err := json.Marshal(h.Steps)
	if err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO habits (key, name, steps, times, found_at) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET times = excluded.times WHERE habits.state = ''`,
		h.Key, h.Name, string(steps), h.Times, time.Now().Unix())
	if err != nil {
		return false, fmt.Errorf("store: keep habit: %w", err)
	}
	var state string
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM habits WHERE key = ?`, h.Key).Scan(&state); err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0 && state == HabitOpen, nil
}

func (s *Store) OpenHabits(ctx context.Context) ([]Habit, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, name, steps, times FROM habits WHERE state = '' ORDER BY times DESC, found_at`)
	if err != nil {
		return nil, fmt.Errorf("store: read habits: %w", err)
	}
	defer rows.Close()
	var out []Habit
	for rows.Next() {
		var h Habit
		var steps string
		if err := rows.Scan(&h.Key, &h.Name, &steps, &h.Times); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(steps), &h.Steps); err != nil {
			continue
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

type HabitState struct {
	Habit
	State string `json:"state"`
}

func (s *Store) AllHabits(ctx context.Context) ([]HabitState, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, name, times, state FROM habits ORDER BY times DESC, key`)
	if err != nil {
		return nil, fmt.Errorf("store: read habits: %w", err)
	}
	defer rows.Close()
	var out []HabitState
	for rows.Next() {
		var h HabitState
		if err := rows.Scan(&h.Key, &h.Name, &h.Times, &h.State); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) Habit(ctx context.Context, key string) (Habit, bool, error) {
	var h Habit
	var steps string
	err := s.db.QueryRowContext(ctx, `SELECT key, name, steps, times FROM habits WHERE key = ?`, key).
		Scan(&h.Key, &h.Name, &steps, &h.Times)
	if errors.Is(err, sql.ErrNoRows) {
		return Habit{}, false, nil
	}
	if err != nil {
		return Habit{}, false, err
	}
	return h, true, json.Unmarshal([]byte(steps), &h.Steps)
}

func (s *Store) SettleHabit(ctx context.Context, key, state string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE habits SET state = ? WHERE key = ?`, state, key); err != nil {
		return fmt.Errorf("store: settle habit: %w", err)
	}
	return nil
}
