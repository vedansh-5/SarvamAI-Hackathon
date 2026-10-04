package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
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
		if !e.canRun(s.Kind) {
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
func (f fakeHistory) FinishInstruction(context.Context, string, int) error    { return nil }
func (f fakeHistory) ForgetInstruction(context.Context, string) error         { return nil }

func (f fakeHistory) RecentInstructions(context.Context) ([]store.Remembered, error) {
	return f.recent, nil
}

func TestWhatWasAskedBeforeComesFirst(t *testing.T) {
	e := &Engine{
		Sinks:   DefaultSinks(),
		History: fakeHistory{recent: []store.Remembered{{Text: "search for a lens hood", Kind: "web.search", Done: true}}},
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
		History: fakeHistory{recent: []store.Remembered{{Text: "add milk to my list", Kind: "app.act", Done: true}}},
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
		History: fakeHistory{recent: []store.Remembered{{Text: "what is this page about", Kind: "page.here", Done: true}}},
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

type keptHistory struct {
	recent    []store.Remembered
	finished  map[string]int
	forgotten []string
}

func (k *keptHistory) RecordInstruction(context.Context, string, string) error { return nil }
func (k *keptHistory) RecentInstructions(context.Context) ([]store.Remembered, error) {
	return k.recent, nil
}
func (k *keptHistory) FinishInstruction(_ context.Context, text string, calls int) error {
	if k.finished == nil {
		k.finished = map[string]int{}
	}
	k.finished[text] = calls
	return nil
}
func (k *keptHistory) ForgetInstruction(_ context.Context, text string) error {
	k.forgotten = append(k.forgotten, text)
	return nil
}

func TestOnlyWhatFinishedIsOfferedAsRemembered(t *testing.T) {
	e := &Engine{
		Sinks: DefaultSinks(),
		History: fakeHistory{recent: []store.Remembered{
			{Text: "search for a tripod", Kind: "web.search"},
			{Text: "search for a lens hood", Kind: "web.search", Done: true, Calls: 2},
			{Text: "search for a camera bag", Kind: "web.search", Done: true},
		}},
	}

	got := e.Suggestions(context.Background(), "")
	if len(got) < 2 || !got[0].Remembered || !got[1].Remembered {
		t.Fatalf("the two finished instructions do not lead as remembered: %+v", got)
	}
	if got[0].Instant || !got[1].Instant {
		t.Errorf("instant is %v then %v, want it only where the last run asked the model nothing", got[0].Instant, got[1].Instant)
	}
	for _, s := range got {
		if s.Text == "search for a tripod" {
			t.Error("an instruction that never finished was offered")
		}
		if !s.Remembered && s.Instant {
			t.Errorf("%q is an example and cannot be instant", s.Text)
		}
	}
}

func TestAnExampleThatIsRememberedIsShownAsRemembered(t *testing.T) {
	e := &Engine{
		Sinks:   DefaultSinks(),
		History: fakeHistory{recent: []store.Remembered{{Text: "What is this page about", Kind: "page.here", Done: true}}},
	}

	var found []Suggestion
	for _, s := range e.Suggestions(context.Background(), "") {
		if strings.EqualFold(s.Text, "what is this page about") {
			found = append(found, s)
		}
	}
	if len(found) != 1 || !found[0].Remembered {
		t.Errorf("found %+v, want it once and as remembered", found)
	}
}

func TestAnAnswerInTheWindowCountsAsFinished(t *testing.T) {
	answered := &Plan{Asked: "translate this", Copy: "नमस्ते", Say: "नमस्ते"}
	if !answered.answers() {
		t.Error("an answer given in the window was not counted as finished")
	}
	for name, plan := range map[string]*Plan{
		"a refusal said in words":   {Asked: "translate this", Say: "Select the text first"},
		"a guide still to be run":   {Asked: "how do I mute", Guide: &GuidePlan{}, Copy: "x"},
		"a change still to confirm": {Asked: "mute it", Web: &WebPlan{}, Copy: "x"},
	} {
		if plan.answers() {
			t.Errorf("%s was counted as finished", name)
		}
	}
}

func TestFinishingIsNotedAgainstWhatWasAsked(t *testing.T) {
	kept := &keptHistory{}
	e := &Engine{History: kept}

	e.finished(context.Background(), &Plan{Asked: "top stories"}, 0)
	e.finished(context.Background(), &Plan{}, 3)
	e.finished(context.Background(), nil, 3)

	if calls, ok := kept.finished["top stories"]; !ok || calls != 0 || len(kept.finished) != 1 {
		t.Errorf("finished = %+v, want only what was asked, with what it cost", kept.finished)
	}
}

func TestForgettingGoesToTheHistory(t *testing.T) {
	kept := &keptHistory{}
	e := &Engine{History: kept}

	if err := e.ForgetAsked(context.Background(), "top stories"); err != nil {
		t.Fatalf("ForgetAsked: %v", err)
	}
	if len(kept.forgotten) != 1 || kept.forgotten[0] != "top stories" {
		t.Errorf("forgotten = %v", kept.forgotten)
	}
	if err := (&Engine{}).ForgetAsked(context.Background(), "anything"); err != nil {
		t.Errorf("with nothing kept, forgetting failed: %v", err)
	}
}

func TestAskingMarksAnAnswerFinishedAndARefusalNot(t *testing.T) {
	answered := &keptHistory{}
	e := textEngine(t, route.KindText, "Can we move it to Saturday?", &textWorker{answer: synth.TextAnswer{Text: "शनिवार को चलें?"}})
	e.History = answered
	plan, err := e.Ask(context.Background(), Ask{Instruction: "translate this to Hindi", Context: mail})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if plan.Asked != "translate this to Hindi" {
		t.Errorf("the plan carries %q as what was asked", plan.Asked)
	}
	if _, ok := answered.finished["translate this to Hindi"]; !ok {
		t.Errorf("an answered request was not marked finished: %+v", answered.finished)
	}

	refused := &keptHistory{}
	e = textEngine(t, route.KindText, "", &textWorker{})
	e.History = refused
	if _, err := e.Ask(context.Background(), Ask{Instruction: "translate this to Hindi", Context: mail}); err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if len(refused.finished) != 0 {
		t.Errorf("a request that was turned down was marked finished: %+v", refused.finished)
	}
}
