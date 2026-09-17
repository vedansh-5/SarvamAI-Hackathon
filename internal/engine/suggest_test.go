package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/store"
)

// The list is a promise. Offering something this build cannot carry out makes
// the panel lie, which is worse than offering nothing.
func TestSuggestionsOnlyOfferWhatThisBuildCanRun(t *testing.T) {
	e := &Engine{Sinks: DefaultSinks()}

	got := e.Suggestions(context.Background(), "")
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
	for _, s := range without.Suggestions(context.Background(), "") {
		if s.Destination == "notes" {
			t.Fatalf("offered %q to notes with no notes sink", s.Text)
		}
	}

	with := &Engine{Sinks: DefaultSinks(NotesSink{})}
	var found bool
	for _, s := range with.Suggestions(context.Background(), "") {
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

	if got := e.Suggestions(context.Background(), "Google Chrome"); got[0].Kind != route.KindPageHere {
		t.Errorf("first suggestion is %q with a browser in front", got[0].Text)
	}

	plain := e.Suggestions(context.Background(), "Finder")
	if len(plain) == 0 {
		t.Fatal("no suggestions")
	}
}

type fakeHistory struct{ recent []store.Remembered }

func (f fakeHistory) RecordInstruction(context.Context, string, string) error { return nil }

func (f fakeHistory) RecentInstructions(context.Context) ([]store.Remembered, error) {
	return f.recent, nil
}

func TestWhatWasAskedBeforeComesFirst(t *testing.T) {
	e := &Engine{
		Sinks:   DefaultSinks(),
		History: fakeHistory{recent: []store.Remembered{{Text: "search for a lens hood", Kind: "web.search"}}},
	}

	got := e.Suggestions(context.Background(), "")
	if len(got) == 0 {
		t.Fatal("no suggestions")
	}
	if got[0].Text != "search for a lens hood" {
		t.Errorf("first suggestion is %q, want what was asked before", got[0].Text)
	}
}

func TestARememberedInstructionThisBuildCannotRunIsNotOffered(t *testing.T) {
	e := &Engine{
		Sinks:   DefaultSinks(),
		History: fakeHistory{recent: []store.Remembered{{Text: "add milk to my list", Kind: "app.act"}}},
	}

	for _, s := range e.Suggestions(context.Background(), "") {
		if s.Text == "add milk to my list" {
			t.Fatal("offered a remembered instruction this build cannot carry out")
		}
	}
}

func TestARememberedInstructionIsNotOfferedTwice(t *testing.T) {
	e := &Engine{
		Sinks:   DefaultSinks(),
		History: fakeHistory{recent: []store.Remembered{{Text: "what is this page about", Kind: "page.here"}}},
	}

	seen := 0
	for _, s := range e.Suggestions(context.Background(), "") {
		if s.Text == "what is this page about" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("offered %d times, want once", seen)
	}
}

// Naming an application in a suggestion is a claim that it is there. Offering
// an action in something this machine does not have is the panel lying, the
// same way offering a destination that does not exist would be.
func TestAnAppExampleNamesSomethingInstalled(t *testing.T) {
	got, ok := appExampleFor([]string{"Notes", "Safari"}, "")
	if !ok {
		t.Fatal("Notes is installed and no action was offered")
	}
	if !strings.Contains(got.Text, "Notes") {
		t.Errorf("offered %q, which does not name Notes", got.Text)
	}
	if got.Kind != route.KindAppAction {
		t.Errorf("kind = %q, want an application action", got.Kind)
	}
}

func TestAnAppExampleFallsBackToWhatIsInstalled(t *testing.T) {
	got, ok := appExampleFor([]string{"Safari", "Reminders"}, "")
	if !ok || !strings.Contains(got.Text, "Reminders") {
		t.Errorf("offered %q (ok=%v), want the Reminders example", got.Text, ok)
	}
}

func TestNothingIsOfferedWhenNeitherApplicationIsThere(t *testing.T) {
	if got, ok := appExampleFor([]string{"Safari", "Terminal"}, ""); ok {
		t.Errorf("offered %q with neither application installed", got.Text)
	}
}

// A build that cannot act in an application must not advertise that it can,
// and one with no catalogue has no way to know what is installed.
func TestNoAppExampleWithoutTheMeansToAct(t *testing.T) {
	e := &Engine{Sinks: DefaultSinks()}
	if got, ok := e.appExample(""); ok {
		t.Errorf("offered %q from a build that cannot act in an application", got.Text)
	}
}

// The application someone is looking at is the one an example is most likely to
// be about, so it wins over the order the examples happen to be written in.
func TestAnAppExamplePrefersWhatIsInFront(t *testing.T) {
	got, ok := appExampleFor([]string{"Notes", "Reminders"}, "Reminders")
	if !ok {
		t.Fatal("nothing offered with both installed")
	}
	if !strings.Contains(got.Text, "Reminders") {
		t.Errorf("offered %q with Reminders in front", got.Text)
	}
}

func TestTheFrontApplicationIsIgnoredWhenItIsNotInstalled(t *testing.T) {
	got, ok := appExampleFor([]string{"Notes"}, "Reminders")
	if !ok || !strings.Contains(got.Text, "Notes") {
		t.Errorf("offered %q (ok=%v), want the installed application", got.Text, ok)
	}
}
