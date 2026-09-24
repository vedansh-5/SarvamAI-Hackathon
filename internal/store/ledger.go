package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// KeptDeeds is how many changes are remembered.
//
// Enough to answer "what has this thing done to my machine" and to undo
// something from earlier in a sitting, without becoming a permanent record of
// everywhere someone has been. The ledger exists to make VANS accountable and
// reversible, not to watch anybody.
const KeptDeeds = 200

// Deed is one change VANS made, after it was made.
//
// Written at the moment a plan is committed, which is the single place any side
// effect happens, so nothing can change the world without passing through here.
type Deed struct {
	ID int64

	At time.Time

	// Kind is which tier acted: the scripting dictionary, a page, a window
	// through accessibility, a multi-step run, a restyle, a document written.
	Kind string

	// Where is the application or the host it happened in.
	Where string

	// Asked is what the person asked for, in their words.
	Asked string

	// Did is what was going to happen, in the words the confirmation used. The
	// person agreed to this sentence, so it is the honest record of what they
	// agreed to.
	Did string

	// Changed is what the window or page showed afterwards, where it was read
	// back. Empty where the tier cannot see its own effect.
	Changed string

	// Verified reports that the change was seen, not assumed (D-004).
	Verified bool

	// Undo is how to put it back, empty when it cannot be put back. Held as the
	// tier's own description rather than as columns: what an inverse needs
	// differs per tier, and a schema change for each would be the wrong shape.
	Undo string

	// UndoneAt is when it was put back, zero while it still stands.
	UndoneAt time.Time
}

// Undoable reports whether anything was recorded that could put this back.
func (d Deed) Undoable() bool { return strings.TrimSpace(d.Undo) != "" && d.UndoneAt.IsZero() }

func (s *Store) RecordDeed(ctx context.Context, d Deed) (int64, error) {
	at := d.At
	if at.IsZero() {
		at = time.Now()
	}

	res, err := s.db.ExecContext(ctx, `
INSERT INTO deeds (at, kind, where_, asked, did, changed, verified, undo, undone_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		at.Unix(), d.Kind, d.Where, d.Asked, d.Did, d.Changed, d.Verified, d.Undo)
	if err != nil {
		return 0, fmt.Errorf("store: record what was done: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("store: record what was done: %w", err)
	}

	if _, err := s.db.ExecContext(ctx, `
DELETE FROM deeds WHERE id NOT IN (
	SELECT id FROM deeds ORDER BY at DESC, id DESC LIMIT ?)`, KeptDeeds); err != nil {
		return id, fmt.Errorf("store: forget the oldest changes: %w", err)
	}
	return id, nil
}

func (s *Store) RecentDeeds(ctx context.Context, limit int) ([]Deed, error) {
	if limit <= 0 || limit > KeptDeeds {
		limit = KeptDeeds
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, at, kind, where_, asked, did, changed, verified, undo, undone_at
FROM deeds ORDER BY at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: read what was done: %w", err)
	}
	defer rows.Close()

	var out []Deed
	for rows.Next() {
		var d Deed
		var at, undoneAt int64
		if err := rows.Scan(&d.ID, &at, &d.Kind, &d.Where, &d.Asked, &d.Did,
			&d.Changed, &d.Verified, &d.Undo, &undoneAt); err != nil {
			return nil, fmt.Errorf("store: read what was done: %w", err)
		}
		d.At = time.Unix(at, 0)
		if undoneAt > 0 {
			d.UndoneAt = time.Unix(undoneAt, 0)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Deed reads one back, for undoing it.
func (s *Store) Deed(ctx context.Context, id int64) (Deed, bool, error) {
	var d Deed
	var at, undoneAt int64

	err := s.db.QueryRowContext(ctx, `
SELECT id, at, kind, where_, asked, did, changed, verified, undo, undone_at
FROM deeds WHERE id = ?`, id).Scan(&d.ID, &at, &d.Kind, &d.Where, &d.Asked,
		&d.Did, &d.Changed, &d.Verified, &d.Undo, &undoneAt)
	if err != nil {
		return Deed{}, false, nil
	}

	d.At = time.Unix(at, 0)
	if undoneAt > 0 {
		d.UndoneAt = time.Unix(undoneAt, 0)
	}
	return d, true, nil
}

// MarkUndone records that a change was put back, so it is not offered twice.
func (s *Store) MarkUndone(ctx context.Context, id int64) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE deeds SET undone_at = ? WHERE id = ?`, time.Now().Unix(), id); err != nil {
		return fmt.Errorf("store: record that it was put back: %w", err)
	}
	return nil
}
