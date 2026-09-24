package engine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/store"
)

type recordedDeeds struct {
	deeds []store.Deed
	err   error
}

func (r *recordedDeeds) RecordDeed(_ context.Context, d store.Deed) (int64, error) {
	if r.err != nil {
		return 0, r.err
	}
	r.deeds = append(r.deeds, d)
	return int64(len(r.deeds)), nil
}

// Every side effect goes through Commit, which is the whole reason the ledger
// can be complete: nothing can change the world without being written down.
func TestEveryCommittedChangeIsWrittenDown(t *testing.T) {
	rec := &tabRecorder{}
	deeds := &recordedDeeds{}
	e := &Engine{Browser: rec, Deeds: deeds}

	plan := &Plan{
		Task: Task{Intent: "sign me in"},
		Web:  &WebPlan{URL: "https://example.com/", Preview: "Click Sign in", script: "noop"},
	}
	if _, err := e.Commit(context.Background(), plan); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if len(deeds.deeds) != 1 {
		t.Fatalf("recorded %d change(s), want 1", len(deeds.deeds))
	}
	d := deeds.deeds[0]
	if d.Kind != "page" {
		t.Errorf("Kind = %q, want page", d.Kind)
	}
	if d.Where != "example.com" {
		t.Errorf("Where = %q, want example.com", d.Where)
	}
	if d.Asked != "sign me in" {
		t.Errorf("Asked = %q", d.Asked)
	}
	// The sentence agreed to, not one written afterwards to fit the outcome.
	if d.Did != "Click Sign in" {
		t.Errorf("Did = %q, want the preview the person confirmed", d.Did)
	}
}

// A change that failed halfway is the one somebody most needs to see.
func TestAChangeThatFailedIsStillWrittenDown(t *testing.T) {
	deeds := &recordedDeeds{}
	e := &Engine{Deeds: deeds}

	plan := &Plan{
		Task:  Task{Intent: "highlight the headings"},
		Style: &StylePlan{URL: "https://example.com/", Preview: "Highlight the headings"},
	}
	if _, err := e.Commit(context.Background(), plan); err == nil {
		t.Fatal("want an error: there is no browser to restyle through")
	}

	if len(deeds.deeds) != 1 {
		t.Fatalf("recorded %d change(s), want 1 even though it failed", len(deeds.deeds))
	}
	d := deeds.deeds[0]
	if d.Verified {
		t.Error("a change that did not finish was recorded as verified")
	}
	if !strings.Contains(d.Changed, "did not finish") {
		t.Errorf("Changed = %q, want it to say it did not finish", d.Changed)
	}
}

// Only the tier that has a real inverse claims one. A hopeful guess is worse
// than being told plainly that something cannot be put back.
func TestOnlyAReversibleChangeSaysItCanBeUndone(t *testing.T) {
	restyle := &Plan{Style: &StylePlan{URL: "https://example.com/", Preview: "Highlight"}}
	if undoFor(restyle) == "" {
		t.Error("a restyle can be taken off and should say so")
	}

	clearing := &Plan{Style: &StylePlan{URL: "https://example.com/", Clears: true}}
	if undoFor(clearing) != "" {
		t.Error("taking the highlighting off is already the undo; it needs no undo of its own")
	}

	typed := &Plan{Drive: &DrivePlan{App: "TextEdit", Task: "type hello"}}
	if undoFor(typed) != "" {
		t.Error("typing has no inverse yet and must not claim one")
	}
}

// Recording is not a precondition for acting.
func TestAFailedRecordingNeverFailsTheChange(t *testing.T) {
	rec := &tabRecorder{}
	e := &Engine{Browser: rec, Deeds: &recordedDeeds{err: errors.New("store: disk is full")}}

	plan := &Plan{Web: &WebPlan{URL: "https://example.com/", script: "noop"}}
	if _, err := e.Commit(context.Background(), plan); err != nil {
		t.Fatalf("the change was reported as failed because writing it down failed: %v", err)
	}
	if rec.frontCalls != 1 {
		t.Errorf("acted %d time(s), want 1", rec.frontCalls)
	}
}

// A build with no store acts exactly as it did before.
func TestWithoutALedgerNothingChanges(t *testing.T) {
	rec := &tabRecorder{}
	e := &Engine{Browser: rec}

	plan := &Plan{Web: &WebPlan{URL: "https://example.com/", script: "noop"}}
	if _, err := e.Commit(context.Background(), plan); err != nil {
		t.Fatalf("Commit: %v", err)
	}
}
