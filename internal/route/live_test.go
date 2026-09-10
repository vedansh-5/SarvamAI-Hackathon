//go:build integration

// An external test package: store imports route, so an in-package test that
// reached for the store would close an import cycle.
package route_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
)

// Run with: go test -tags integration ./internal/route/
//
// Makes real API calls. Free open-weight default:
//
//	export VANS_LLM_API_KEY=sk-or-v1-...      # openrouter.ai key
//	go test -tags integration -v ./internal/route/
//
// This is where the routing thesis is checked against a real model rather than
// a fake: the unit tests prove the cache returns what it stored, and this
// proves a model can produce something worth storing in the first place.
func TestRouteLive(t *testing.T) {
	if os.Getenv(synth.EnvAPIKey) == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("no API key in the environment")
	}

	decider, err := synth.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := decider.(route.Decider)
	if !ok {
		t.Fatalf("%T cannot route", decider)
	}

	s, err := store.Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r := &route.Router{
		Decider: d,
		Cache:   s,
		Log:     func(f string, a ...any) { t.Logf(f, a...) },
	}

	cases := []struct {
		instruction string
		want        route.Kind
		warm        bool // expected to be answered from cache
	}{
		{"the top 10 stories from https://news.ycombinator.com", route.KindWebExtract, false},
		{"the top 30 stories from https://lobste.rs", route.KindWebExtract, true},
		{"The top 5 stories from example.com", route.KindWebExtract, true},
		{"reply to the last message in Telegram", route.KindAppAction, false},
	}

	for _, tc := range cases {
		start := time.Now()
		res, err := r.Route(context.Background(), tc.instruction)
		elapsed := time.Since(start).Round(time.Millisecond)

		if err != nil {
			t.Errorf("%q: %v", tc.instruction, err)
			continue
		}

		t.Logf("%-52s %7s  calls=%d  %-12s %v",
			tc.instruction, elapsed, res.DecisionCalls, res.Route.Kind, res.Route.Slots)

		if res.Route.Kind != tc.want {
			t.Errorf("%q routed to %s, want %s (model said: %s)",
				tc.instruction, res.Route.Kind, tc.want, res.Route.Why)
		}
		// The claim: a repeat of a known shape costs nothing.
		if tc.warm && res.DecisionCalls != 0 {
			t.Errorf("%q cost %d model calls, want 0 — the shape was already known",
				tc.instruction, res.DecisionCalls)
		}
	}
}

// Declining has to work against a real model too. A model that never says
// "unknown" would route everything to the nearest kind, which is the failure
// this answer exists to prevent.
func TestDeclineLive(t *testing.T) {
	if os.Getenv(synth.EnvAPIKey) == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("no API key in the environment")
	}

	decider, err := synth.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	d, _ := decider.(route.Decider)

	s, err := store.Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	r := &route.Router{Decider: d, Cache: s}
	res, err := r.Route(context.Background(), "translate my fridge into Portuguese")

	if err == nil {
		t.Logf("routed to %s %v (why: %s)", res.Route.Kind, res.Route.Slots, res.Route.Why)
		t.Fatal("a nonsense instruction was routed rather than declined")
	}

	// The error has to be the model declining, not the API failing. Accepting
	// any error here would let a rate limit or an outage pass as proof that
	// declining works, which is the one thing this test exists to show.
	var undecided *route.UndecidedError
	if !errors.As(err, &undecided) {
		t.Fatalf("did not reach a decision at all: %v", err)
	}
	t.Logf("declined, as it should: %s", undecided.Why)
}
