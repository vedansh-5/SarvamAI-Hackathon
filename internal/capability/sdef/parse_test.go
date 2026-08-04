package sdef

import "testing"

const dir = "testdata"

func load(t *testing.T, name string) *Dictionary {
	t.Helper()
	d, err := ParseFile(dir+"/"+name, TestdataResolver(dir))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", name, err)
	}
	return d
}

// The acceptance test. Notes declares only "show" and "open note location" of
// its own. "make" arrives solely through xi:include of CocoaStandard.sdef. If
// this fails, XInclude is not resolving and V1 cannot create a note at all.
func TestXIncludeBringsInMake(t *testing.T) {
	d := load(t, "Notes.sdef")

	mk, ok := d.FindCommand("make")
	if !ok {
		t.Fatal(`"make" not found — xi:include is not resolving`)
	}
	if got := mk.AccessKind(); got != Write {
		t.Errorf("make classified %v, want Write", got)
	}
}

func TestNotesOwnCommandsSurvive(t *testing.T) {
	d := load(t, "Notes.sdef")
	for _, name := range []string{"show", "open note location"} {
		if _, ok := d.FindCommand(name); !ok {
			t.Errorf("missing Notes command %q", name)
		}
	}
}

func TestNotesClasses(t *testing.T) {
	d := load(t, "Notes.sdef")
	want := map[string]bool{"account": false, "folder": false, "note": false, "attachment": false}
	for _, c := range d.Classes() {
		if _, ok := want[c.Name]; ok {
			want[c.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing class %q", name)
		}
	}
}

func TestPropertyWritableDefaults(t *testing.T) {
	cases := []struct {
		access string
		want   bool
	}{
		{"r", false},
		{"w", true},
		{"rw", true},
		{"", true}, // absent ⇒ rw — the majority case
	}
	for _, tc := range cases {
		if got := (Property{Access: tc.access}).Writable(); got != tc.want {
			t.Errorf("access=%q: Writable()=%v, want %v", tc.access, got, tc.want)
		}
	}
}

func TestHiddenTermsFiltered(t *testing.T) {
	d := load(t, "Mail.sdef")
	for _, c := range d.Commands() {
		if isYes(c.Hidden) {
			t.Errorf("hidden command %q leaked through", c.Name)
		}
	}
	for _, c := range d.Classes() {
		for _, p := range c.VisibleProperties() {
			if isYes(p.Hidden) {
				t.Errorf("hidden property %q.%q leaked through", c.Name, p.Name)
			}
		}
	}
}

// Mail is the richest dictionary on the machine — a smoke test that the parser
// survives real-world complexity, without brittle magic numbers that a macOS
// update would break.
func TestMailParses(t *testing.T) {
	d := load(t, "Mail.sdef")

	if got := len(d.Classes()); got < 20 {
		t.Errorf("got %d classes, want >= 20", got)
	}

	var absent, explicit int
	for _, c := range d.Classes() {
		for _, p := range c.VisibleProperties() {
			if p.Access == "" {
				absent++
			} else {
				explicit++
			}
		}
	}
	if absent <= explicit {
		t.Errorf("absent=%d explicit=%d — expected absent access to dominate", absent, explicit)
	}
}

func TestChromeParses(t *testing.T) {
	d := load(t, "Chrome.sdef")
	if len(d.Commands()) == 0 {
		t.Error("no commands parsed from Chrome.sdef")
	}
}
