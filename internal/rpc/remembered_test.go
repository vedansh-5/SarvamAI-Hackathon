package rpc

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/store"
)

type askedBefore struct {
	recent    []store.Remembered
	forgotten []string
}

func (a *askedBefore) RecordInstruction(context.Context, string, string) error { return nil }
func (a *askedBefore) FinishInstruction(context.Context, string, int) error    { return nil }
func (a *askedBefore) RecentInstructions(context.Context) ([]store.Remembered, error) {
	return a.recent, nil
}
func (a *askedBefore) ForgetInstruction(_ context.Context, text string) error {
	a.forgotten = append(a.forgotten, text)
	return nil
}

func TestTheWindowIsToldWhichSuggestionsAreRemembered(t *testing.T) {
	kept := &askedBefore{recent: []store.Remembered{
		{Text: "the top stories on example.com", Kind: "web.extract", Done: true},
		{Text: "search for a lens hood", Kind: "web.search", Done: true, Calls: 2},
	}}
	s := run(t, func(srv *Server, _ *session) { srv.Engine.History = kept },
		req(1, "suggestions", SuggestionsParams{}),
	)

	got := decode[[]Suggestion](t, s.ok(1))
	if len(got) < 3 {
		t.Fatalf("suggestions = %+v, want the remembered two and some examples", got)
	}
	if !got[0].Remembered || !got[0].Instant {
		t.Errorf("first = %+v, want remembered and instant", got[0])
	}
	if !got[1].Remembered || got[1].Instant {
		t.Errorf("second = %+v, want remembered and not instant", got[1])
	}
	if last := got[len(got)-1]; last.Remembered {
		t.Errorf("an example was sent as remembered: %+v", last)
	}
}

func TestOneRememberedInstructionIsForgottenByName(t *testing.T) {
	kept := &askedBefore{}
	s := run(t, func(srv *Server, _ *session) { srv.Engine.History = kept },
		req(1, "forget_instruction", ForgetInstructionParams{Text: "search for a lens hood"}),
		req(2, "forget_instruction", ForgetInstructionParams{Text: "  "}),
	)

	if got := decode[ForgetInstructionResult](t, s.ok(1)); !got.Forgotten {
		t.Errorf("result = %+v, want it forgotten", got)
	}
	if len(kept.forgotten) != 1 || kept.forgotten[0] != "search for a lens hood" {
		t.Errorf("forgotten = %v", kept.forgotten)
	}
	if e := s.failure(2); e == nil || e.Code != CodeBadRequest {
		t.Errorf("forgetting nothing in particular = %+v, want a bad request", e)
	}
}
