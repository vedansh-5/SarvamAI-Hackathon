package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const RememberedInstructions = 3

type Remembered struct {
	Text string
	Kind string

	Done  bool
	Calls int
}

func (s *Store) RecordInstruction(ctx context.Context, text, kind string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO instructions (text, kind, used_at) VALUES (?, ?, ?)
ON CONFLICT(text) DO UPDATE SET
	kind    = excluded.kind,
	used_at = excluded.used_at`,
		text, kind, time.Now().Unix()); err != nil {
		return fmt.Errorf("store: remember instruction: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
DELETE FROM instructions WHERE text NOT IN (
	SELECT text FROM instructions
	ORDER BY (text = ?) DESC, (done_at > 0) DESC, used_at DESC, rowid DESC LIMIT ?)`,
		text, RememberedInstructions); err != nil {
		return fmt.Errorf("store: forget the oldest instruction: %w", err)
	}
	return nil
}

func (s *Store) FinishInstruction(ctx context.Context, text string, calls int) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE instructions SET done_at = ?, calls = ? WHERE text = ?`,
		time.Now().Unix(), calls, text); err != nil {
		return fmt.Errorf("store: mark instruction finished: %w", err)
	}
	return nil
}

func (s *Store) RecentInstructions(ctx context.Context) ([]Remembered, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT text, kind, done_at > 0, calls FROM instructions ORDER BY used_at DESC, rowid DESC LIMIT ?`,
		RememberedInstructions)
	if err != nil {
		return nil, fmt.Errorf("store: read remembered instructions: %w", err)
	}
	defer rows.Close()

	var out []Remembered
	for rows.Next() {
		var r Remembered
		if err := rows.Scan(&r.Text, &r.Kind, &r.Done, &r.Calls); err != nil {
			return nil, fmt.Errorf("store: read remembered instruction: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ForgetInstruction(ctx context.Context, text string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM instructions WHERE text = ?`, strings.TrimSpace(text)); err != nil {
		return fmt.Errorf("store: forget instruction: %w", err)
	}
	return nil
}

func (s *Store) ForgetInstructions(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM instructions`); err != nil {
		return fmt.Errorf("store: forget instructions: %w", err)
	}
	return nil
}
