package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Setting reads a durable fact. A missing key is empty rather than an error:
// every caller's answer to "not set yet" is the same as its answer to "".
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: read setting %q: %w", key, err)
	}
	return v, nil
}

// SetSetting writes a durable fact.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO settings (key, value) VALUES (?, ?)
ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("store: write setting %q: %w", key, err)
	}
	return nil
}
