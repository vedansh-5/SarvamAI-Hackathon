package store

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/uiact"
)

func windowProgram() uiact.Program {
	return uiact.Program{App: "Telegram", Steps: []uiact.Step{{
		Do:     "type",
		Target: uiact.Selector{Role: "AXTextArea", Title: "Message"},
		Text:   "{s0}",
	}}}
}

func TestWindowActionRoundTrips(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if _, found, err := s.WindowAction(ctx, "Telegram", "send {s0}"); err != nil || found {
		t.Fatalf("found=%v err=%v before anything was saved", found, err)
	}

	if err := s.SaveWindowAction(ctx, "Telegram", "send {s0}", windowProgram(), "test-model"); err != nil {
		t.Fatalf("SaveWindowAction: %v", err)
	}

	got, found, err := s.WindowAction(ctx, "Telegram", "send {s0}")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("round trip lost the steps: %+v", got)
	}
	if got.Steps[0].Text != "{s0}" || got.Steps[0].Target.Title != "Message" {
		t.Errorf("round trip changed the program: %+v", got.Steps[0])
	}

	n, err := s.WindowActionCount(ctx)
	if err != nil || n != 1 {
		t.Errorf("count = %d, err = %v", n, err)
	}

	if err := s.ForgetWindowAction(ctx, "Telegram", "send {s0}"); err != nil {
		t.Fatalf("ForgetWindowAction: %v", err)
	}
	if _, found, _ := s.WindowAction(ctx, "Telegram", "send {s0}"); found {
		t.Error("the program survived being forgotten")
	}
}

// The placeholder has to come back as a placeholder. A program cached with the
// value of the request that produced it would send that value every time.
func TestWindowActionKeepsThePlaceholder(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveWindowAction(ctx, "Telegram", "send {s0}", windowProgram(), "m"); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.WindowAction(ctx, "Telegram", "send {s0}")
	if err != nil {
		t.Fatal(err)
	}
	if baked := uiact.Baked(got, map[string]string{"s0": "on my way"}); len(baked) != 0 {
		t.Errorf("a value was baked into the cached program: %v", baked)
	}
}

// The two tiers share a key space but not a table, so caching a window program
// must not make the application tier think it has one.
func TestWindowActionsAreSeparateFromApplicationActions(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	if err := s.SaveWindowAction(ctx, "Notes", "add {s0}", windowProgram(), "m"); err != nil {
		t.Fatal(err)
	}
	if _, found, err := s.Action(ctx, "Notes", "add {s0}"); err != nil || found {
		t.Errorf("a window program answered as an application program (found=%v, err=%v)", found, err)
	}
}
