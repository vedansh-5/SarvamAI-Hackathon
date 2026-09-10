package capability

import (
	"strings"
	"testing"
)

func surfaceOf(t *testing.T, name string) Surface {
	t.Helper()
	c := New()
	app, err := c.Find(name)
	if err != nil {
		t.Skipf("%s is not installed: %v", name, err)
	}
	d, err := c.Dictionary(app)
	if err != nil {
		t.Fatalf("dictionary: %v", err)
	}
	return Describe(app, d)
}

// Notes hangs its own objects off the application class with a
// class-extension rather than declaring them, which is the ordinary Cocoa
// shape. A surface built only from <class> elements misses them entirely and
// reports an application that can do nothing with notes.
func TestClassExtensionsReachTheSurface(t *testing.T) {
	s := surfaceOf(t, "Notes")

	app, ok := nounNamed(s, "application")
	if !ok {
		t.Fatal("no application object")
	}
	if !contains(app.Contains, "note") {
		t.Errorf("the application does not contain notes: %v", app.Contains)
	}

	note, ok := nounNamed(s, "note")
	if !ok {
		t.Fatal("no note object")
	}
	if !hasProp(note, "body") || !hasProp(note, "name") {
		t.Error("a note has neither a body nor a name")
	}
}

// The read/write split has to come from the dictionary. It decides whether a
// plan needs confirming, and a model that could relabel its own write as a
// read would be deciding that for itself.
func TestWritingCommandsAreMarkedFromTheDictionary(t *testing.T) {
	s := surfaceOf(t, "Notes")

	make_, ok := s.Verb("make")
	if !ok {
		t.Fatal("no make command")
	}
	if !make_.Writes {
		t.Error("make is not marked as changing state")
	}

	get, ok := s.Verb("get")
	if !ok {
		t.Fatal("no get command")
	}
	if get.Writes {
		t.Error("get is marked as changing state")
	}

	if !s.Writes([]string{"get", "count", "make"}) {
		t.Error("a program that makes something is not reported as writing")
	}
	if s.Writes([]string{"get", "count"}) {
		t.Error("a program that only reads is reported as writing")
	}
}

// A command the application does not have is the failure that survives the
// gate: the plan names it, the user confirms it, and the Apple event is
// rejected afterwards.
func TestUnknownCommandsAreNamed(t *testing.T) {
	s := surfaceOf(t, "Notes")

	got := s.Unknown([]string{"make", "sendTelepathically", "get", "sendTelepathically"})
	if len(got) != 1 || got[0] != "sendTelepathically" {
		t.Errorf("unknown = %v, want the one command that does not exist, listed once", got)
	}
}

func TestEveryInstalledSurfaceFitsTheBudget(t *testing.T) {
	c := New()
	apps := c.Scriptable()
	if len(apps) == 0 {
		t.Skip("no scriptable applications installed")
	}

	for _, a := range apps {
		d, err := c.Dictionary(a)
		if err != nil {
			t.Errorf("%s: %v", a.Name, err)
			continue
		}
		s := Describe(a, d)

		if len(s.Verbs) == 0 && len(s.Nouns) == 0 {
			t.Errorf("%s: the surface is empty", a.Name)
		}
		if n := len(s.Text()); n > MaxSurfaceBytes {
			t.Errorf("%s: renders %d bytes, over the %d budget", a.Name, n, MaxSurfaceBytes)
		}
	}
}

// Trimming to fit must never drop a command, because the validator checks
// against what the model was shown.
func TestTrimmingKeepsEveryCommandAndSaysWhatItDropped(t *testing.T) {
	s := surfaceOf(t, "Notes")
	if len(s.Nouns) < 2 {
		t.Skip("nothing to trim")
	}

	full := s.Text()
	tight := s.TextWithin(len(full) / 2)

	if len(tight) >= len(full) {
		t.Fatal("nothing was trimmed")
	}
	for _, v := range s.Verbs {
		if v.Hidden {
			continue
		}
		if !strings.Contains(tight, "  "+v.Name) {
			t.Errorf("command %q was dropped by trimming", v.Name)
		}
	}
	if !strings.Contains(tight, "not listed here") {
		t.Error("the trimmed rendering does not say that anything is missing")
	}
}

func nounNamed(s Surface, name string) (Noun, bool) {
	for _, n := range s.Nouns {
		if n.Name == name {
			return n, true
		}
	}
	return Noun{}, false
}

func hasProp(n Noun, name string) bool {
	for _, p := range n.Properties {
		if p.Name == name {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
