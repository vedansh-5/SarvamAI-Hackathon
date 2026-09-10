package answer

import (
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/runner"
)

func rows() []runner.Row {
	return []runner.Row{
		{"title": "Widget Co", "link": "https://widget.example/about", "description": "Makes widgets."},
		{"title": "Gadget Ltd", "link": "https://gadget.example", "description": "Makes gadgets."},
	}
}

func reasons(t *testing.T, err error) []string {
	t.Helper()
	var ungrounded *UngroundedError
	if !errors.As(err, &ungrounded) {
		t.Fatalf("err = %v, want UngroundedError", err)
	}
	return ungrounded.Reasons
}

func mentions(rs []string, want string) bool {
	for _, r := range rs {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func TestAGroundedAnswerIsAccepted(t *testing.T) {
	a := Answer{Text: "Widget Co makes widgets.", Cites: []int{1}}
	if err := Validate(a, rows()); err != nil {
		t.Errorf("a cited answer was refused: %v", err)
	}
}

// The failure worth catching. An answer with no citations came from the
// model's memory rather than the page, and reads exactly like one that did
// not — fluent, plausible, and about something else entirely.
func TestAnUncitedAnswerIsRefused(t *testing.T) {
	a := Answer{Text: "Widget Co was founded in 1994 by two brothers."}
	if !mentions(reasons(t, Validate(a, rows())), "points at none") {
		t.Error("an answer citing nothing was accepted")
	}
}

func TestACitationOutOfRangeIsRefused(t *testing.T) {
	a := Answer{Text: "Something.", Cites: []int{1, 7}}
	if !mentions(reasons(t, Validate(a, rows())), "cites result 7") {
		t.Error("a citation past the end of the results was accepted")
	}
}

// A fabricated URL is the one kind of invention that cannot be judged by
// reading: it looks exactly like a real one.
func TestAnInventedLinkIsRefused(t *testing.T) {
	a := Answer{
		Text:  "See https://widget.example/pricing for details.",
		Cites: []int{1},
	}
	if !mentions(reasons(t, Validate(a, rows())), "not in the results") {
		t.Error("a link that appears nowhere in the rows was accepted")
	}

	real := Answer{Text: "See https://gadget.example for details.", Cites: []int{2}}
	if err := Validate(real, rows()); err != nil {
		t.Errorf("a link that is in the rows was refused: %v", err)
	}
}

func TestAnEmptyAnswerIsRefused(t *testing.T) {
	if !mentions(reasons(t, Validate(Answer{Cites: []int{1}}, rows())), "says nothing") {
		t.Error("an empty answer was accepted")
	}
}

func TestCitedReturnsTheRowsInCitedOrder(t *testing.T) {
	got := Cited(Answer{Cites: []int{2, 1, 2, 9}}, rows())
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 unique in range", len(got))
	}
	if got[0]["title"] != "Gadget Ltd" || got[1]["title"] != "Widget Co" {
		t.Errorf("rows came back in the wrong order: %v", got)
	}
}
