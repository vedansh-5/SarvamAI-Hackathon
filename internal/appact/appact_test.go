package appact

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/vedansh/vans/internal/capability"
)

func notes(t *testing.T) capability.Surface {
	t.Helper()
	c := capability.New()
	app, err := c.Find("Notes")
	if err != nil {
		t.Skipf("Notes is not installed: %v", err)
	}
	d, err := c.Dictionary(app)
	if err != nil {
		t.Fatalf("dictionary: %v", err)
	}
	return capability.Describe(app, d)
}

func reasons(t *testing.T, err error) []string {
	t.Helper()
	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("err = %v, want InvalidError", err)
	}
	return invalid.Reasons
}

func mentions(reasons []string, want string) bool {
	for _, r := range reasons {
		if strings.Contains(r, want) {
			return true
		}
	}
	return false
}

func writeNote() Program {
	return Program{
		App: "Notes",
		Steps: []Step{{
			Command: "make",
			Make:    "note",
			With:    map[string]string{"body": "<div>hello</div>"},
			Bind:    "note",
		}},
	}
}

func TestAValidProgramRenders(t *testing.T) {
	s := notes(t)

	script, err := Render(writeNote(), s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, `Application("Notes")`) {
		t.Error("the script does not address Notes")
	}
	if !strings.Contains(script, `new: "note"`) {
		t.Error("the script does not make a note")
	}
	if !strings.Contains(script, `body: "<div>hello</div>"`) {
		t.Errorf("the body did not survive encoding:\n%s", script)
	}
}

// The one that matters. Whether a confirmation appears is decided from the
// dictionary, so a program cannot present itself as harmless.
func TestWritingIsDecidedByTheDictionary(t *testing.T) {
	s := notes(t)

	if !Writes(writeNote(), s) {
		t.Error("making a note is not reported as a write")
	}

	read := Program{
		App: "Notes",
		Steps: []Step{{
			Command: "get",
			Target:  &Selector{Class: "note", Index: 1},
		}},
	}
	if Writes(read, s) {
		t.Error("reading a note is reported as a write")
	}
}

func TestAProgramForAnotherApplicationIsRefused(t *testing.T) {
	s := notes(t)
	p := writeNote()
	p.App = "Messages"

	if !mentions(reasons(t, Validate(p, s)), "addresses") {
		t.Error("a program aimed at a different application was accepted")
	}
}

func TestInventedCommandsAndObjectsAreRefused(t *testing.T) {
	s := notes(t)
	p := Program{
		App: "Notes",
		Steps: []Step{{
			Command: "sendTelepathically",
			Target:  &Selector{Class: "telegram"},
		}},
	}

	got := reasons(t, Validate(p, s))
	if !mentions(got, `no "sendTelepathically" command`) {
		t.Errorf("the invented command was accepted: %v", got)
	}
	if !mentions(got, `no "telegram" object`) {
		t.Errorf("the invented object was accepted: %v", got)
	}
}

// A property that does not exist, or one that cannot be changed, fails at the
// Apple event — which is after the user has confirmed a plan that said it
// would work.
func TestPropertiesAreCheckedAgainstTheClass(t *testing.T) {
	s := notes(t)

	missing := writeNote()
	missing.Steps[0].With = map[string]string{"urgency": "high"}
	if !mentions(reasons(t, Validate(missing, s)), `no "urgency"`) {
		t.Error("a property the class does not have was accepted")
	}

	readOnly := writeNote()
	readOnly.Steps[0].With = map[string]string{"id": "x-1"}
	if !mentions(reasons(t, Validate(readOnly, s)), "cannot be changed") {
		t.Error("a read-only property was accepted as something to set")
	}
}

func TestEveryReasonIsReportedAtOnce(t *testing.T) {
	s := notes(t)
	p := Program{
		App: "Messages",
		Steps: []Step{{
			Command: "levitate",
			Make:    "unicorn",
		}},
	}

	if got := reasons(t, Validate(p, s)); len(got) < 3 {
		t.Errorf("got %d reasons, want every problem at once: %v", len(got), got)
	}
}

func TestABindingMustExistBeforeItIsUsed(t *testing.T) {
	s := notes(t)
	p := Program{
		App: "Notes",
		Steps: []Step{{
			Command: "get",
			Target:  &Selector{Class: "note", In: "folder"},
		}},
	}

	if !mentions(reasons(t, Validate(p, s)), "no earlier step produced") {
		t.Error("a program read a binding nothing had produced")
	}
}

// Values from the model reach the script through encoding/json, so a value
// cannot close its own literal and continue as code.
//
// Checked by decoding the literal back rather than by looking for the payload
// in the script: correctly escaped, the payload *is* still in there, sitting
// harmlessly inside a string. What matters is that it decodes to exactly what
// went in, which is only true if nothing broke out.
func TestAValueCannotBecomeCode(t *testing.T) {
	hostile := []string{
		`"); app.quit(); ("`,
		"line\nbreak",
		`back\slash`,
		"\u2028paragraph",
		"tab\there",
		`{ "not": "an object" }`,
	}

	for _, value := range hostile {
		got, err := literal(value)
		if err != nil {
			t.Fatalf("literal(%q): %v", value, err)
		}
		if strings.ContainsAny(got[1:len(got)-1], "\n\r") {
			t.Errorf("literal(%q) = %s, which spans lines", value, got)
		}

		var back string
		if err := json.Unmarshal([]byte(got), &back); err != nil {
			t.Errorf("literal(%q) = %s, which is not a closed literal: %v", value, got, err)
			continue
		}
		if back != value {
			t.Errorf("literal(%q) decoded to %q", value, back)
		}
	}
}

func TestSelectorsUseTheCollectionNameJXAExpects(t *testing.T) {
	s := notes(t)
	p := Program{
		App: "Notes",
		Steps: []Step{{
			Command: "get",
			Target:  &Selector{Class: "note", Name: "Groceries"},
		}},
	}

	script, err := Render(p, s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(script, `app.notes.byName("Groceries")`) {
		t.Errorf("the selector did not render as a named lookup:\n%s", script)
	}
}

func TestDeclaredPluralsWinOverDerivedOnes(t *testing.T) {
	c := capability.New()
	app, err := c.Find("Messages")
	if err != nil {
		t.Skip("Messages is not installed")
	}
	d, err := c.Dictionary(app)
	if err != nil {
		t.Fatal(err)
	}
	s := capability.Describe(app, d)

	var found bool
	for _, n := range s.Nouns {
		if n.Name == "file transfer" {
			found = true
			if n.Plural != "file transfers" {
				t.Errorf("plural = %q, want the declared one", n.Plural)
			}
		}
	}
	if !found {
		t.Skip("Messages no longer declares file transfers")
	}
}

func TestPluralRules(t *testing.T) {
	for in, want := range map[string]string{
		"note":          "notes",
		"folder":        "folders",
		"account":       "accounts",
		"attachment":    "attachments",
		"category":      "categories",
		"box":           "boxes",
		"address":       "addresses",
		"day":           "days",
		"file transfer": "file transfers",
	} {
		if got := Plural(in); got != want {
			t.Errorf("Plural(%q) = %q, want %q", in, got, want)
		}
	}
}

// A bound get has to stay a specifier. Calling it turns a live reference into
// a dead copy, and the next step addresses something the application no
// longer recognises — which fails at the Apple event, after the user has
// confirmed a plan that said it would work.
func TestABoundLookupStaysAddressable(t *testing.T) {
	c := capability.New()
	app, err := c.Find("Reminders")
	if err != nil {
		t.Skip("Reminders is not installed")
	}
	d, err := c.Dictionary(app)
	if err != nil {
		t.Fatal(err)
	}
	s := capability.Describe(app, d)

	p := Program{
		App: "Reminders",
		Steps: []Step{
			{Command: "get", Target: &Selector{Class: "list", Name: "Groceries"}, Bind: "list"},
			{
				Command: "make",
				Make:    "reminder",
				Target:  &Selector{Class: "reminder", In: "list"},
				With:    map[string]string{"name": "milk"},
			},
		},
	}

	script, err := Render(p, s)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(script, `byName("Groceries")()`) {
		t.Errorf("the bound lookup was evaluated:\n%s", script)
	}
	if !strings.Contains(script, "at: list.reminders") {
		t.Errorf("the reminder is not placed in the bound list:\n%s", script)
	}
}

// The failure this guards against is silent and permanent. A program cached
// with a value written into it creates the first note's text for every later
// note of that shape — nothing errors, and the wrong content is written
// forever.
func TestValuesWrittenIntoTheProgramAreCaught(t *testing.T) {
	literals := map[string]string{"s0": "pick up the dry cleaning"}

	baked := Program{App: "Notes", Steps: []Step{{
		Command: "make", Make: "note",
		With: map[string]string{"body": "pick up the dry cleaning"},
	}}}
	if got := Baked(baked, literals); len(got) != 1 {
		t.Errorf("Baked = %v, want the hardcoded value", got)
	}

	templated := Program{App: "Notes", Steps: []Step{{
		Command: "make", Make: "note",
		With: map[string]string{"body": "{s0}"},
	}}}
	if got := Baked(templated, literals); len(got) != 0 {
		t.Errorf("Baked = %v, want nothing — it references the placeholder", got)
	}
}

func TestFillingReplacesValuesAndLeavesStructureAlone(t *testing.T) {
	p := Program{App: "Notes", Steps: []Step{{
		Command: "make",
		Make:    "note",
		Bind:    "note",
		Target:  &Selector{Class: "folder", Name: "{s0}"},
		With:    map[string]string{"body": "{s1}"},
	}}}

	got := p.Fill(func(v string) string {
		switch v {
		case "{s0}":
			return "Work"
		case "{s1}":
			return "the thing"
		}
		return v
	})

	if got.Steps[0].Target.Name != "Work" {
		t.Errorf("target name = %q", got.Steps[0].Target.Name)
	}
	if got.Steps[0].With["body"] != "the thing" {
		t.Errorf("body = %q", got.Steps[0].With["body"])
	}
	if got.Steps[0].Command != "make" || got.Steps[0].Make != "note" || got.Steps[0].Bind != "note" {
		t.Error("filling changed the program's structure, not just its values")
	}
}
