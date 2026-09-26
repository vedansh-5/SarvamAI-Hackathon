package uiact

import (
	"strings"
	"testing"
)

func typeStep(text string) Step {
	return Step{
		Do:     "type",
		Target: Selector{Role: "AXTextArea", Title: "Document"},
		Text:   text,
	}
}

func programOf(steps ...Step) Program {
	return Program{App: "TextEdit", Steps: steps}
}

func TestTemplatizeLiftsTheValueOutOfTypedText(t *testing.T) {
	got := Templatize(programOf(typeStep("hello")), `type "hello" into the document`, map[string]string{"s0": "hello"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if got.Steps() != 1 {
		t.Fatalf("got %d steps, want 1", got.Steps())
	}
	if got.Program.Steps[0].Text != "{s0}" {
		t.Errorf("text = %q, want {s0}", got.Program.Steps[0].Text)
	}
	if len(got.Pinned) != 0 {
		t.Errorf("pinned = %v, want none for a fully templated run", got.Pinned)
	}
}

// The round trip is the whole claim: what comes out, filled with new values, is
// the program that run would have been for those values.
func TestATemplatizedRunFillsBackForNewValues(t *testing.T) {
	got := Templatize(programOf(typeStep("hello")), `type "hello" into the document`, map[string]string{"s0": "hello"})

	filled := got.Program.Fill(func(v string) string {
		if v == "{s0}" {
			return "goodbye"
		}
		return v
	})
	if filled.Steps[0].Text != "goodbye" {
		t.Errorf("text = %q, want goodbye", filled.Steps[0].Text)
	}
}

// The one that matters. A composed sentence is not in the request and not a value
// from it, so replaying it would type this run's words into a different request.
func TestATypedSentenceTheModelWroteIsNotKept(t *testing.T) {
	got := Templatize(
		programOf(typeStep("Hi Bob, just confirming our meeting on Tuesday.")),
		`email "Bob" about the meeting`, map[string]string{"s0": "Bob"})

	if got.Keep() {
		t.Fatal("Keep() = true for text the model composed")
	}
	if got.Why == "" {
		t.Error("no reason given; a run that is not cached has to say why")
	}
}

// Text copied off the window is refused for the same reason, and this is stricter
// than it first looks: the window's contents are exactly what will have changed by
// the time the trace is replayed.
func TestTextCopiedFromTheWindowIsNotKept(t *testing.T) {
	got := Templatize(programOf(typeStep("Untitled 3.rtf")), `search for "search"`, map[string]string{"s0": "search"})

	if got.Keep() {
		t.Fatal("Keep() = true for text that came from the window rather than the request")
	}
}

// A value too short to swap out of the middle of a control's name is left where
// it is and the run is pinned to it, rather than thrown away. "close tab 2" ->
// click the row called "Tab 2": the 2 is a value, and it cannot be lifted out of
// the middle of a name without also rewriting any other 2 in it.
func TestAShortValueInsideAControlNamePinsTheRun(t *testing.T) {
	got := Templatize(
		programOf(Step{Do: "click", Target: Selector{Role: "AXRow", Title: "Tab 2"}}),
		"close tab 2", map[string]string{"n0": "2"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if got.Pinned["n0"] != "2" {
		t.Errorf("pinned = %v, want n0 pinned to \"2\"", got.Pinned)
	}
	if title := got.Program.Steps[0].Target.Title; title != "Tab 2" {
		t.Errorf("title = %q, want it left alone", title)
	}
}

// Typed text gets no such latitude. A short value surrounded by anything else
// leaves that anything else unaccounted for, and text is the field nothing
// downstream can check — so the run is refused rather than pinned.
func TestAShortValueInsideTypedTextIsRefused(t *testing.T) {
	got := Templatize(programOf(typeStep("a4")), `type "a"`, map[string]string{"s0": "a"})

	if got.Keep() {
		t.Fatal("Keep() = true for typed text with content that came from nowhere")
	}
}

// Short is only a problem inside a longer string. A field that *is* the value has
// no surrounding text to damage, so it templates at any length and stays general.
func TestAShortValueOnItsOwnStillTemplates(t *testing.T) {
	got := Templatize(programOf(typeStep("a")), `type "a"`, map[string]string{"s0": "a"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if got.Program.Steps[0].Text != "{s0}" {
		t.Errorf("text = %q, want {s0}", got.Program.Steps[0].Text)
	}
	if len(got.Pinned) != 0 {
		t.Errorf("pinned = %v, want none — a whole field is safe to swap", got.Pinned)
	}
}

// A value that contains another has to be swapped first, or the shorter one
// rewrites the middle of it and the placeholder stands for the wrong thing.
func TestTheLongerValueIsLiftedFirst(t *testing.T) {
	got := Templatize(
		programOf(typeStep("hello world")),
		`type "hello world"`, map[string]string{"s0": "hello", "s1": "hello world"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if text := got.Program.Steps[0].Text; text != "{s1}" {
		t.Errorf("text = %q, want {s1} — the longer value should win", text)
	}
}

// A selector is held to a looser standard than typed text, deliberately: a stale
// one fails validation on replay and the run resumes live, so it cannot do harm.
func TestAButtonNameIsNotTreatedAsContributedContent(t *testing.T) {
	got := Templatize(
		programOf(Step{Do: "click", Target: Selector{Role: "AXButton", Title: "Save"}}),
		`type "hello" and save`, map[string]string{"s0": "hello"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q — a constant button name is fine", got.Why)
	}
	if got.Program.Steps[0].Target.Title != "Save" {
		t.Errorf("title = %q, want it left alone", got.Program.Steps[0].Target.Title)
	}
}

func TestAValueInASelectorIsLiftedOut(t *testing.T) {
	got := Templatize(
		programOf(Step{Do: "click", Target: Selector{Role: "AXRow", Title: "Wallpaper"}}),
		`open "Wallpaper"`, map[string]string{"s0": "Wallpaper"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if title := got.Program.Steps[0].Target.Title; title != "{s0}" {
		t.Errorf("title = %q, want {s0}", title)
	}
}

// A description with a stale value in it would make the confirmation describe the
// wrong thing, so it is dropped and Preview writes a fresh one.
func TestADescriptionThatWillNotTemplateIsDropped(t *testing.T) {
	step := typeStep("{s0}")
	step.Text = "hello"
	step.Describe = "Type hello into the document"

	got := Templatize(programOf(step), `type "hello" into the document`, map[string]string{"s0": "hello"})
	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if describe := got.Program.Steps[0].Describe; describe != "Type {s0} into the document" {
		t.Errorf("describe = %q, want the value lifted out", describe)
	}
}

func TestAStepsWithNoTextAndNoValuesIsKeptAsItIs(t *testing.T) {
	got := Templatize(
		programOf(Step{Do: "press", Key: "Enter"}),
		"press enter", map[string]string{})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if got.Program.Steps[0].Key != "Enter" {
		t.Errorf("key = %q, want Enter", got.Program.Steps[0].Key)
	}
}

// Baked is the check on the way in and Templatize the operation on the way out.
// They have to agree about what counts as a value or a program will be stored
// that the other would have rejected.
func TestTemplatizeLeavesNothingBakedBehind(t *testing.T) {
	literals := map[string]string{"s0": "hello"}

	got := Templatize(programOf(typeStep("hello")), `type "hello"`, literals)
	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if baked := Baked(got.Program, literals); len(baked) != 0 {
		t.Errorf("Baked found %v in a templatized program", baked)
	}
}

// A placeholder token carries the value's index as a digit, so "{s0}" contains the
// character "0". Once one value has been lifted out, a request whose own value is
// "0" was then found inside the placeholder standing for a different value, and
// pinned the trace for no reason at all — quietly turning a trace that answers for
// any values into one that answers for almost none.
func TestAValueIsNotFoundInsideAPlaceholderAlreadyPutIn(t *testing.T) {
	got := Templatize(
		programOf(typeStep("hello world")),
		`type "hello" "world" 0 times`, map[string]string{"s0": "hello", "s1": "world", "n0": "0"})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if text := got.Program.Steps[0].Text; text != "{s0} {s1}" {
		t.Errorf("text = %q, want {s0} {s1}", text)
	}
	if len(got.Pinned) != 0 {
		t.Errorf("pinned = %v, want none — the 0 is part of {s0}, not something typed", got.Pinned)
	}
}

// Shape lifts URLs, quoted strings and numbers and nothing else, so the most
// ordinary drive instruction there is carries no literals at all — the words are
// part of the shape. Refusing to keep those was refusing to keep almost
// everything, and the cache would have looked broken rather than strict.
func TestWordsFromTheInstructionAreKeptEvenWithNoLiterals(t *testing.T) {
	got := Templatize(
		programOf(typeStep("hello")),
		"type hello into the document",
		map[string]string{})

	if !got.Keep() {
		t.Fatalf("Keep() = false, why = %q", got.Why)
	}
	if text := got.Program.Steps[0].Text; text != "hello" {
		t.Errorf("text = %q, want it kept as it is — it is part of the key", text)
	}
}

// The same rule that lets instruction words through has to keep window content
// out, because that is what will have changed by the next run.
func TestAWordFromNeitherTheInstructionNorItsValuesIsRefused(t *testing.T) {
	got := Templatize(
		programOf(typeStep("hello Untitled3.rtf")),
		"type hello into the document",
		map[string]string{})

	if got.Keep() {
		t.Fatal("Keep() = true for a word that came from the window")
	}
	if !strings.Contains(got.Why, "Untitled3.rtf") {
		t.Errorf("why = %q, want it to name the word that was not asked for", got.Why)
	}
}
