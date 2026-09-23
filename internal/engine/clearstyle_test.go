package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// sweeper reaches every tab. The live browser adapter does; the debugged one,
// which drives a single tab, does not.
type sweeper struct {
	tabRecorder

	tabsCarryingAStyle int
	sweeps             int
	sweepErr           error
	lastScript         string
}

func (s *sweeper) DoOnEveryTab(_ context.Context, script string) (int, error) {
	s.sweeps++
	s.lastScript = script
	if s.sweepErr != nil {
		return 0, s.sweepErr
	}
	return s.tabsCarryingAStyle, nil
}

// The bug this fixes: highlight one page, move to another, ask for the
// highlighting to go, and the first page stays marked with nothing left
// pointing at it.
func TestClearStyleSweepsEveryTab(t *testing.T) {
	rec := &sweeper{tabsCarryingAStyle: 3}
	e := &Engine{Browser: rec}

	if err := e.ClearStyle(context.Background()); err != nil {
		t.Fatalf("ClearStyle: %v", err)
	}
	if rec.sweeps != 1 {
		t.Errorf("swept %d time(s), want 1", rec.sweeps)
	}
	if rec.frontCalls != 0 {
		t.Error("cleared the front tab as well as sweeping, which does it twice")
	}
	if !strings.Contains(rec.lastScript, "removed") {
		t.Error("the sweep ran a script that cannot report whether it removed anything")
	}
}

// A committed "clear the highlighting" plan is the same act as the button, and
// must not be the one that still only reaches the tab in front.
func TestACommittedClearSweepsEveryTab(t *testing.T) {
	rec := &sweeper{tabsCarryingAStyle: 2}
	e := &Engine{Browser: rec}

	plan := &Plan{Style: &StylePlan{URL: "https://example.com/", Clears: true, script: "noop"}}
	if _, err := e.runStyle(context.Background(), plan); err != nil {
		t.Fatalf("runStyle: %v", err)
	}
	if rec.sweeps != 1 {
		t.Errorf("swept %d time(s), want 1", rec.sweeps)
	}
}

// Putting a restyle on is about the page being looked at, and must not spread.
func TestApplyingAStyleStaysOnTheFrontTab(t *testing.T) {
	rec := &sweeper{}
	e := &Engine{Browser: rec}

	plan := &Plan{Style: &StylePlan{URL: "https://example.com/", script: "noop"}}
	if _, err := e.runStyle(context.Background(), plan); err != nil {
		t.Fatalf("runStyle: %v", err)
	}
	if rec.sweeps != 0 {
		t.Error("restyling swept every tab")
	}
	if rec.frontCalls != 1 {
		t.Errorf("acted on the front tab %d time(s), want 1", rec.frontCalls)
	}
}

// An adapter that cannot reach every tab still clears the one in front. A
// partial undo beats leaving the highlighting up because the sweep was
// unavailable.
func TestClearStyleFallsBackToTheFrontTab(t *testing.T) {
	rec := &sweeper{sweepErr: errors.New("appleevents: Chrome has no window open")}
	e := &Engine{Browser: rec}

	if err := e.ClearStyle(context.Background()); err != nil {
		t.Fatalf("ClearStyle: %v", err)
	}
	if rec.frontCalls != 1 {
		t.Errorf("cleared the front tab %d time(s), want 1 after the sweep failed", rec.frontCalls)
	}
}

func TestClearStyleWithoutASweeperClearsTheFrontTab(t *testing.T) {
	rec := &tabRecorder{}
	e := &Engine{Browser: rec}

	if err := e.ClearStyle(context.Background()); err != nil {
		t.Fatalf("ClearStyle: %v", err)
	}
	if rec.frontCalls != 1 {
		t.Errorf("cleared the front tab %d time(s), want 1", rec.frontCalls)
	}
}
