package engine

import (
	"testing"

	"github.com/vedansh/vans/internal/route"
)

// The list is a promise. Offering something this build cannot carry out makes
// the panel lie, which is worse than offering nothing.
func TestSuggestionsOnlyOfferWhatThisBuildCanRun(t *testing.T) {
	e := &Engine{Sinks: DefaultSinks()}

	got := e.Suggestions("")
	if len(got) == 0 {
		t.Fatal("no suggestions at all")
	}

	for _, s := range got {
		if !e.runs(s.Kind) {
			t.Errorf("offers %q, which this build cannot run", s.Text)
		}
		if s.Destination == "" {
			continue
		}
		if _, err := e.sinkFor(s.Destination); err != nil {
			t.Errorf("offers %q to %q, which does not exist: %v", s.Text, s.Destination, err)
		}
	}
}

// Notes is not among the default sinks, so the suggestion naming it must not
// survive a build that was assembled without one.
func TestASuggestionDisappearsWithItsDestination(t *testing.T) {
	without := &Engine{Sinks: DefaultSinks()}
	for _, s := range without.Suggestions("") {
		if s.Destination == "notes" {
			t.Fatalf("offered %q to notes with no notes sink", s.Text)
		}
	}

	with := &Engine{Sinks: DefaultSinks(NotesSink{})}
	var found bool
	for _, s := range with.Suggestions("") {
		if s.Destination == "notes" {
			found = true
		}
	}
	if !found {
		t.Error("a notes sink is present and nothing offers it")
	}
}

func TestABrowserInFrontPutsThePageFirst(t *testing.T) {
	e := &Engine{Sinks: DefaultSinks()}

	if got := e.Suggestions("Google Chrome"); got[0].Kind != route.KindPageHere {
		t.Errorf("first suggestion is %q with a browser in front", got[0].Text)
	}

	plain := e.Suggestions("Finder")
	if len(plain) == 0 {
		t.Fatal("no suggestions")
	}
}
