package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Routine struct {
	Name        string
	Instruction string
}

func routineKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

func (s *Store) SaveRoutine(ctx context.Context, name, instruction string) error {
	key, instruction := routineKey(name), strings.TrimSpace(instruction)
	if key == "" || instruction == "" {
		return fmt.Errorf("store: a routine needs a name and something to do")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO routines (name, shown, instruction, at) VALUES (?, ?, ?, ?)
ON CONFLICT (name) DO UPDATE SET shown = excluded.shown, instruction = excluded.instruction, at = excluded.at`,
		key, strings.TrimSpace(name), instruction, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("store: save routine: %w", err)
	}
	return nil
}

func (s *Store) Routine(ctx context.Context, name string) (string, bool, error) {
	var instruction string
	err := s.db.QueryRowContext(ctx, `SELECT instruction FROM routines WHERE name = ?`, routineKey(name)).Scan(&instruction)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: read routine: %w", err)
	}
	return instruction, true, nil
}

func (s *Store) Routines(ctx context.Context) ([]Routine, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT shown, instruction FROM routines ORDER BY at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: read routines: %w", err)
	}
	defer rows.Close()
	var out []Routine
	for rows.Next() {
		var r Routine
		if err := rows.Scan(&r.Name, &r.Instruction); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ForgetRoutine(ctx context.Context, name string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM routines WHERE name = ?`, routineKey(name))
	if err != nil {
		return false, fmt.Errorf("store: forget routine: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
