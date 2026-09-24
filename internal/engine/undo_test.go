package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
)

type remembered struct {
	deeds  map[int64]store.Deed
	undone []int64
}

func (r *remembered) Deed(_ context.Context, id int64) (store.Deed, bool, error) {
	d, ok := r.deeds[id]
	return d, ok, nil
}

func (r *remembered) MarkUndone(_ context.Context, id int64) error {
	r.undone = append(r.undone, id)
	d := r.deeds[id]
	d.UndoneAt = time.Now()
	r.deeds[id] = d
	return nil
}

func restyleDeed() store.Deed {
	return store.Deed{
		ID: 1, Kind: "restyle", Where: "example.com",
		Did:  "Highlight every heading",
		Undo: marshalUndo(howToUndo{Kind: undoByClearingTheStyle}),
	}
}

// The first change VANS can actually take back.
func TestUndoPutsARestyleBack(t *testing.T) {
	rec := &sweeper{tabsCarryingAStyle: 2}
	store := &remembered{deeds: map[int64]store.Deed{1: restyleDeed()}}
	e := &Engine{Browser: rec, Recorded: store}

	if _, err := e.Undo(context.Background(), 1); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if rec.sweeps != 1 {
		t.Errorf("swept %d time(s), want 1 — the restyle was never taken off", rec.sweeps)
	}
	if len(store.undone) != 1 || store.undone[0] != 1 {
		t.Errorf("marked %v as put back, want [1]", store.undone)
	}
}

// Recorded as undone while still on screen is worse than no record at all: it
// is a wrong answer to the question the ledger exists to answer.
func TestSomethingIsOnlyMarkedUndoneOnceItIsBack(t *testing.T) {
	rec := &sweeper{sweepErr: errors.New("appleevents: Chrome is not running")}
	rec.frontErr = errors.New("appleevents: Chrome is not running")
	store := &remembered{deeds: map[int64]store.Deed{1: restyleDeed()}}
	e := &Engine{Browser: rec, Recorded: store}

	if _, err := e.Undo(context.Background(), 1); err == nil {
		t.Fatal("want an error: nothing could be put back")
	}
	if len(store.undone) != 0 {
		t.Error("marked as put back though it never was")
	}
}

// The common answer, and it is not a failure.
func TestUndoSaysPlainlyWhenThereIsNoWayBack(t *testing.T) {
	store := &remembered{deeds: map[int64]store.Deed{
		2: {ID: 2, Kind: "steps", Did: "Type hello into Message"},
	}}
	e := &Engine{Recorded: store}

	_, err := e.Undo(context.Background(), 2)
	if !errors.Is(err, ErrCannotBeUndone) {
		t.Fatalf("err = %v, want ErrCannotBeUndone", err)
	}
	// What it was, so the person is not left guessing which change was refused.
	if !strings.Contains(err.Error(), "Type hello into Message") {
		t.Errorf("err = %v, want it to name the change", err)
	}
}

func TestUndoRefusesSomethingAlreadyPutBack(t *testing.T) {
	d := restyleDeed()
	d.UndoneAt = time.Now()
	store := &remembered{deeds: map[int64]store.Deed{1: d}}
	e := &Engine{Recorded: store}

	if _, err := e.Undo(context.Background(), 1); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("err = %v, want ErrNothingToUndo", err)
	}
}

func TestUndoRefusesAnIdThatIsNotThere(t *testing.T) {
	e := &Engine{Recorded: &remembered{deeds: map[int64]store.Deed{}}}

	if _, err := e.Undo(context.Background(), 99); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("err = %v, want ErrNothingToUndo", err)
	}
}

// What is written down has to be what can be acted on later.
func TestWhatIsRecordedIsWhatCanBePutBack(t *testing.T) {
	plan := &Plan{Style: &StylePlan{URL: "https://example.com/", Preview: "Highlight"}}

	recorded := undoFor(plan)
	if recorded == "" {
		t.Fatal("a restyle recorded no way back")
	}

	rec := &sweeper{}
	store := &remembered{deeds: map[int64]store.Deed{
		7: {ID: 7, Kind: "restyle", Did: "Highlight", Undo: recorded},
	}}
	e := &Engine{Browser: rec, Recorded: store}

	if _, err := e.Undo(context.Background(), 7); err != nil {
		t.Fatalf("what the ledger wrote down could not be carried out: %v", err)
	}
}
