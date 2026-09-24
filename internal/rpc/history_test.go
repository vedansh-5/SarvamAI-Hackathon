package rpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/store"
)

type deedsInHand struct{ deeds []store.Deed }

func (d *deedsInHand) RecentDeeds(context.Context, int) ([]store.Deed, error) {
	return d.deeds, nil
}

func TestHistoryReportsWhatWasDone(t *testing.T) {
	s := &Server{Deeds: &deedsInHand{deeds: []store.Deed{
		{
			ID: 2, At: time.Unix(1700000000, 0), Kind: "restyle", Where: "example.com",
			Asked: "highlight the headings", Did: "Highlight every heading",
			Verified: true, Undo: "restyle",
		},
		{
			ID: 1, At: time.Unix(1699999000, 0), Kind: "steps", Where: "TextEdit",
			Asked: "type hello", Did: "Type hello", Verified: true,
		},
	}}}

	got, err := s.history(context.Background())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(got.Deeds) != 2 {
		t.Fatalf("returned %d, want 2", len(got.Deeds))
	}

	// The panel is told the answer, not the mechanism.
	if !got.Deeds[0].Undoable {
		t.Error("the restyle can be taken off and is not being offered as such")
	}
	if got.Deeds[1].Undoable {
		t.Error("typing has no inverse yet and must not be offered as undoable")
	}

	// It has to survive the wire as the panel will read it.
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var back HistoryResult
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.Deeds[0].Where != "example.com" || back.Deeds[0].Did != "Highlight every heading" {
		t.Errorf("came back wrong over the wire: %+v", back.Deeds[0])
	}
}

// A build that remembers nothing answers honestly rather than failing.
func TestHistoryWithoutAStore(t *testing.T) {
	s := &Server{}
	got, err := s.history(context.Background())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(got.Deeds) != 0 {
		t.Errorf("returned %d, want none", len(got.Deeds))
	}
}
