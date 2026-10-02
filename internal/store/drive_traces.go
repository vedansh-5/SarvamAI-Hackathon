package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/vedansh/vans/internal/uiact"
)

// MaxDivergences is how many times a trace may go off before it is dropped.
//
// Two, not one. A single divergence is usually the world having moved — a dialog
// that happened to be open, a window a different size — and the repaired trace
// saved in its place is likely to be right. Twice in a row is the trace being
// wrong about the task rather than unlucky, and then the model is cheaper than
// another wasted replay.
const MaxDivergences = 2

// DriveTrace is the step sequence recorded for an app and an instruction shape.
//
// A miss is not an error. The caller asks the model instead, which is what it
// would have done anyway, so there is nothing here worth failing a run over.
//
// literals is what the request being replayed carries. A trace recorded with
// pinned literals only answers for those exact values — see SaveDriveTrace.
func (s *Store) DriveTrace(ctx context.Context, app, shape string, literals map[string]string) (uiact.Program, bool, error) {
	var program, pinned string
	err := s.db.QueryRowContext(ctx,
		`SELECT program, pinned FROM drive_traces WHERE app = ? AND shape = ?`,
		app, shape).Scan(&program, &pinned)
	if errors.Is(err, sql.ErrNoRows) {
		return uiact.Program{}, false, nil
	}
	if err != nil {
		return uiact.Program{}, false, fmt.Errorf("store: look up drive trace: %w", err)
	}

	var p uiact.Program
	if err := json.Unmarshal([]byte(program), &p); err != nil {
		// Unreadable is the same as absent, and keeping it would mean failing
		// this lookup and every later one for the same key.
		if err := s.ForgetDriveTrace(ctx, app, shape); err != nil {
			return uiact.Program{}, false, err
		}
		return uiact.Program{}, false, nil
	}

	if pinned != "" {
		var want map[string]string
		if err := json.Unmarshal([]byte(pinned), &want); err != nil {
			if err := s.ForgetDriveTrace(ctx, app, shape); err != nil {
				return uiact.Program{}, false, err
			}
			return uiact.Program{}, false, nil
		}
		if !pinsHold(want, literals) {
			return uiact.Program{}, false, nil
		}
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE drive_traces SET uses = uses + 1, last_used_at = ? WHERE app = ? AND shape = ?`,
		s.Now().Unix(), app, shape); err != nil {
		return uiact.Program{}, false, fmt.Errorf("store: record drive trace use: %w", err)
	}
	return p, true, nil
}

// SaveDriveTrace keeps the steps that got a task done, replacing whatever was
// there. One trace per key: the newest run that worked is the best guess for the
// next one, so they are replaced rather than accumulated.
//
// pinned is empty for a trace whose every literal was lifted out into a
// placeholder, which is the case worth having — it answers for any values of that
// shape. When a literal could not be substituted safely, the caller passes just
// that value and the trace is only replayed where the request carries it too;
// anything else in the request is a placeholder and is filled in fresh. That is a
// much narrower cache than a templated trace and still free for the common case
// of asking the same thing twice.
func (s *Store) SaveDriveTrace(ctx context.Context, app, shape string, p uiact.Program, pinned map[string]string, model string) error {
	program, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("store: encode drive trace: %w", err)
	}

	values := ""
	if len(pinned) > 0 {
		encoded, err := json.Marshal(pinned)
		if err != nil {
			return fmt.Errorf("store: encode drive trace literals: %w", err)
		}
		values = string(encoded)
	}

	// uses and divergences start again with the trace. They describe how a
	// particular sequence has fared, and this is a different sequence.
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO drive_traces (app, shape, program, pinned, model, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (app, shape) DO UPDATE SET
	program     = excluded.program,
	pinned      = excluded.pinned,
	model       = excluded.model,
	created_at  = excluded.created_at,
	uses        = 0,
	divergences = 0`,
		app, shape, string(program), values, model, s.Now().Unix()); err != nil {
		return fmt.Errorf("store: save drive trace: %w", err)
	}
	return nil
}

// DriveTraceWorked records a replay that ran to the end without going off.
//
// It clears the divergence count, because that count exists to find a trace that
// is reliably wrong. Without the reset, a trace that works most of the time would
// still be dropped eventually by divergences spread across weeks.
func (s *Store) DriveTraceWorked(ctx context.Context, app, shape string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE drive_traces SET divergences = 0 WHERE app = ? AND shape = ?`,
		app, shape); err != nil {
		return fmt.Errorf("store: record drive trace success: %w", err)
	}
	return nil
}

// DriveTraceDiverged records a replay that stopped fitting the window, and drops
// the trace once that has happened MaxDivergences times running.
//
// It reports whether the trace was dropped so the caller can say so: a trace
// being forgotten is the thing that explains why tomorrow's run costs model
// calls again, and silently losing it would make that look like a regression.
func (s *Store) DriveTraceDiverged(ctx context.Context, app, shape string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		`UPDATE drive_traces SET divergences = divergences + 1
		 WHERE app = ? AND shape = ?
		 RETURNING divergences`, app, shape).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: record drive trace divergence: %w", err)
	}
	if count < MaxDivergences {
		return false, nil
	}
	if err := s.ForgetDriveTrace(ctx, app, shape); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) ForgetDriveTrace(ctx context.Context, app, shape string) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM drive_traces WHERE app = ? AND shape = ?`, app, shape); err != nil {
		return fmt.Errorf("store: forget drive trace: %w", err)
	}
	return nil
}

func (s *Store) DriveTraceCount(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM drive_traces`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count drive traces: %w", err)
	}
	return n, nil
}

// pinsHold reports that a request carries the values a pinned trace was recorded
// with.
//
// It constrains only the values it names, and deliberately does not require the
// two sets to match. A trace is pinned on the values that could not be lifted out
// of it; every other value in the request became a placeholder and is filled in
// fresh. Demanding equality would mean a trace pinned on one value never answered
// any request that also carried a second — which is to say, never answered at all.
func pinsHold(want, got map[string]string) bool {
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

type KeptRun struct {
	Shape   string
	Program uiact.Program
}

const MostKeptRunsRead = 40

func (s *Store) DriveTracesIn(ctx context.Context, app string) ([]KeptRun, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT shape, program FROM drive_traces WHERE app = ? ORDER BY last_used_at DESC, created_at DESC LIMIT ?`,
		app, MostKeptRunsRead)
	if err != nil {
		return nil, fmt.Errorf("store: list drive traces: %w", err)
	}
	defer rows.Close()

	var out []KeptRun
	for rows.Next() {
		var shape, program string
		if err := rows.Scan(&shape, &program); err != nil {
			return nil, fmt.Errorf("store: read drive trace: %w", err)
		}
		var p uiact.Program
		if err := json.Unmarshal([]byte(program), &p); err != nil || len(p.Steps) == 0 {
			continue
		}
		out = append(out, KeptRun{Shape: shape, Program: p})
	}
	return out, rows.Err()
}
