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
	SELECT text FROM instructions ORDER BY used_at DESC, rowid DESC LIMIT ?)`,
		RememberedInstructions); err != nil {
		return fmt.Errorf("store: forget the oldest instruction: %w", err)
	}
	return nil
}

func (s *Store) RecentInstructions(ctx context.Context) ([]Remembered, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT text, kind FROM instructions ORDER BY used_at DESC, rowid DESC LIMIT ?`,
		RememberedInstructions)
	if err != nil {
		return nil, fmt.Errorf("store: read remembered instructions: %w", err)
	}
	defer rows.Close()

	var out []Remembered
	for rows.Next() {
		var r Remembered
		if err := rows.Scan(&r.Text, &r.Kind); err != nil {
			return nil, fmt.Errorf("store: read remembered instruction: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ForgetInstructions(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM instructions`); err != nil {
		return fmt.Errorf("store: forget instructions: %w", err)
	}
	return nil
}
