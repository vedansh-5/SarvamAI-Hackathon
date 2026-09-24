package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vedansh/vans/internal/store"
)

// DeedStore reads back and marks off what VANS has done. Implemented by store.
type DeedStore interface {
	Deed(ctx context.Context, id int64) (store.Deed, bool, error)
	MarkUndone(ctx context.Context, id int64) error
}

// ErrNothingToUndo is an id that names no change, or one already put back.
var ErrNothingToUndo = errors.New("engine: there is nothing there to put back")

// ErrCannotBeUndone is a change VANS has no inverse for.
//
// Its own error because it is the honest and common answer, and because it is
// not a failure: the change happened, it stands, and saying so plainly is the
// point of having recorded it at all.
var ErrCannotBeUndone = errors.New("engine: that cannot be undone")

// howToUndo is a tier's own description of its inverse.
//
// Held as the tier's description rather than as a general program, because what
// reversing costs differs per tier: a restyle is one script and a window action
// is a program against a window that has since moved on. A shared shape would
// have to be the union of all of them and would fit none.
type howToUndo struct {
	Kind string `json:"kind"`
	App  string `json:"app,omitempty"`
}

const (
	undoByClearingTheStyle = "restyle"
)

func marshalUndo(u howToUndo) string {
	encoded, err := json.Marshal(u)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// Undo puts back one recorded change.
//
// Never a confirmation of its own. Undoing is not a side effect anyone needs
// protecting from, and a gate in front of putting something back teaches people
// to dismiss gates (the same reasoning as D-100).
func (e *Engine) Undo(ctx context.Context, id int64) (store.Deed, error) {
	if e.Recorded == nil {
		return store.Deed{}, fmt.Errorf("engine: this build remembers nothing, so it can put nothing back")
	}

	deed, found, err := e.Recorded.Deed(ctx, id)
	if err != nil {
		return store.Deed{}, err
	}
	if !found {
		return store.Deed{}, ErrNothingToUndo
	}
	if !deed.UndoneAt.IsZero() {
		return deed, fmt.Errorf("%w: it was already put back", ErrNothingToUndo)
	}
	if deed.Undo == "" {
		return deed, fmt.Errorf("%w: %s", ErrCannotBeUndone, deed.Did)
	}

	var how howToUndo
	if err := json.Unmarshal([]byte(deed.Undo), &how); err != nil {
		return deed, fmt.Errorf("engine: the record of how to put that back is unreadable: %w", err)
	}

	if err := e.putItBack(ctx, how); err != nil {
		return deed, err
	}

	// Marked only once it is actually back. A change recorded as undone that is
	// still there is worse than no record: it is a wrong answer to the question
	// the ledger exists to answer.
	if err := e.Recorded.MarkUndone(ctx, id); err != nil {
		e.logf("put it back but could not write that down: %v", err)
	}

	deed.UndoneAt = time.Now()
	e.logf("put back: %s", deed.Did)
	return deed, nil
}

func (e *Engine) putItBack(ctx context.Context, how howToUndo) error {
	switch how.Kind {
	case undoByClearingTheStyle:
		return e.ClearStyle(ctx)
	default:
		return fmt.Errorf("%w: nothing here knows how to reverse a %q", ErrCannotBeUndone, how.Kind)
	}
}
