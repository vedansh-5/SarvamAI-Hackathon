package capability

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fakeRoot(t *testing.T, apps map[string]bool) string {
	t.Helper()
	root := t.TempDir()
	for name, scriptable := range apps {
		res := filepath.Join(root, name+".app", "Contents", "Resources")
		if err := os.MkdirAll(res, 0o755); err != nil {
			t.Fatal(err)
		}
		if !scriptable {
			continue
		}
		dict := filepath.Join(res, name+".sdef")
		if err := os.WriteFile(dict, []byte(`<dictionary title="`+name+`"/>`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// An application with no dictionary has no structured surface to drive, and
// listing it would offer the user something that cannot be carried out.
func TestOnlyScriptableAppsAreListed(t *testing.T) {
	root := fakeRoot(t, map[string]bool{"Notes": true, "Calculator": false, "Reminders": true})
	c := &Catalog{Roots: []string{root}}

	apps := c.Scriptable()
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2: %v", len(apps), apps)
	}
	if apps[0].Name != "Notes" || apps[1].Name != "Reminders" {
		t.Errorf("apps = %v, want them sorted by name", apps)
	}
}

func TestFindIgnoresCaseAndSpacing(t *testing.T) {
	root := fakeRoot(t, map[string]bool{"QuickTime Player": true})
	c := &Catalog{Roots: []string{root}}

	for _, name := range []string{"QuickTime Player", "quicktime player", "  QUICKTIME   PLAYER ", "QuickTime Player.app"} {
		got, err := c.Find(name)
		if err != nil {
			t.Errorf("Find(%q): %v", name, err)
			continue
		}
		if got.Name != "QuickTime Player" {
			t.Errorf("Find(%q) = %q", name, got.Name)
		}
	}
}

func TestFindResolvesAnUnambiguousPartialName(t *testing.T) {
	root := fakeRoot(t, map[string]bool{"Reminders": true, "Notes": true})
	c := &Catalog{Roots: []string{root}}

	got, err := c.Find("remind")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.Name != "Reminders" {
		t.Errorf("got %q", got.Name)
	}
}

// Two candidates is not a near miss to be broken by ordering: acting in the
// wrong application is exactly the failure a confirmation gate cannot catch,
// because the plan would name the app VANS chose rather than the one meant.
func TestAmbiguousNamesAreRefusedWithTheCandidates(t *testing.T) {
	root := fakeRoot(t, map[string]bool{"Mail": true, "Mailbox": true})
	c := &Catalog{Roots: []string{root}}

	_, err := c.Find("mail")
	if err != nil {
		t.Fatalf("an exact name should still win outright: %v", err)
	}

	_, err = c.Find("mai")
	var unknown *UnknownAppError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want UnknownAppError", err)
	}
	if len(unknown.Near) != 2 {
		t.Errorf("candidates = %v, want both", unknown.Near)
	}
}

func TestUnknownAppNamesNothing(t *testing.T) {
	root := fakeRoot(t, map[string]bool{"Notes": true})
	c := &Catalog{Roots: []string{root}}

	_, err := c.Find("Photoshop")
	var unknown *UnknownAppError
	if !errors.As(err, &unknown) {
		t.Fatalf("err = %v, want UnknownAppError", err)
	}
	if len(unknown.Near) != 0 {
		t.Errorf("candidates = %v, want none", unknown.Near)
	}
}

func TestTheSameAppInTwoRootsIsListedOnce(t *testing.T) {
	first := fakeRoot(t, map[string]bool{"Notes": true})
	second := fakeRoot(t, map[string]bool{"Notes": true, "Mail": true})
	c := &Catalog{Roots: []string{first, second}}

	if got := len(c.Scriptable()); got != 2 {
		t.Errorf("got %d apps, want 2", got)
	}
}

// Every dictionary on this machine has to parse. A single unreadable one is
// an application VANS silently cannot drive.
func TestEveryInstalledDictionaryParses(t *testing.T) {
	c := New()
	apps := c.Scriptable()
	if len(apps) == 0 {
		t.Skip("no scriptable applications installed")
	}

	for _, a := range apps {
		if _, err := c.Dictionary(a); err != nil {
			t.Errorf("%s: %v", a.Name, err)
		}
	}
}
