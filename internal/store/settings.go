package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

type Rules struct {
	Never      []string `json:"never"`
	SpendLimit float64  `json:"spend_limit"`
}

const rulesKey = "rules"

func (s *Store) Rules(ctx context.Context) (Rules, error) {
	raw, err := s.Setting(ctx, rulesKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return Rules{}, nil
	}
	var r Rules
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return Rules{}, fmt.Errorf("store: read rules: %w", err)
	}
	return r, nil
}

func (s *Store) SetRules(ctx context.Context, r Rules) error {
	var kept []string
	seen := map[string]bool{}
	for _, name := range r.Never {
		name = strings.TrimSpace(name)
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		kept = append(kept, name)
	}
	r.Never = kept
	if r.SpendLimit < 0 {
		r.SpendLimit = 0
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("store: encode rules: %w", err)
	}
	return s.SetSetting(ctx, rulesKey, string(raw))
}
