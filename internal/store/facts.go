package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const MostFacts = 200

func (s *Store) Remember(ctx context.Context, fact string) error {
	fact = strings.Join(strings.Fields(fact), " ")
	if fact == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO facts (text, at) VALUES (?, ?) ON CONFLICT (text) DO UPDATE SET at = excluded.at`,
		fact, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("store: remember: %w", err)
	}
	return nil
}

func (s *Store) Facts(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT text FROM facts ORDER BY at DESC, id DESC LIMIT ?`, MostFacts)
	if err != nil {
		return nil, fmt.Errorf("store: read what is remembered: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var text string
		if err := rows.Scan(&text); err != nil {
			return nil, err
		}
		out = append(out, text)
	}
	return out, rows.Err()
}

func (s *Store) Forget(ctx context.Context, about string) (int, error) {
	about = strings.ToLower(strings.TrimSpace(about))
	if about == "" {
		return 0, nil
	}
	facts, err := s.Facts(ctx)
	if err != nil {
		return 0, err
	}
	gone := 0
	for _, fact := range facts {
		if !strings.Contains(strings.ToLower(fact), about) {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM facts WHERE text = ?`, fact); err != nil {
			return gone, fmt.Errorf("store: forget: %w", err)
		}
		gone++
	}
	return gone, nil
}
