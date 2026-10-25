package sdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const notesPath = "/System/Applications/Notes.app"

func loadFile(path string) (*Dictionary, error) {
	return ParseFile(path, SystemResolver)
}

func bundle(t *testing.T, dictName, dict string) string {
	t.Helper()
	root := t.TempDir()
	app := filepath.Join(root, "Thing.app")
	res := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(res, dictName), []byte(dict), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

func TestLocateFindsTheDictionaryWhateverItIsCalled(t *testing.T) {
	app := bundle(t, "iCal.sdef", `<dictionary title="x"/>`)

	got, err := Locate(app)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	if filepath.Base(got) != "iCal.sdef" {
		t.Errorf("found %q", got)
	}
}

func TestLocateReportsAnUnscriptableApp(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Quiet.app")
	if err := os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := Locate(app); err != ErrNoDictionary {
		t.Errorf("err = %v, want ErrNoDictionary", err)
	}
}

func TestIncludedSuitesAreSpliced(t *testing.T) {
	root := t.TempDir()
	standard := filepath.Join(root, "Standard.sdef")
	if err := os.WriteFile(standard, []byte(`<dictionary title="Standard">
	  <suite name="Standard Suite" code="core">
	    <command name="get" code="coregetd"/>
	    <command name="delete" code="coredelo"/>
	  </suite>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	own := filepath.Join(root, "Thing.sdef")
	if err := os.WriteFile(own, []byte(`<dictionary title="Thing"
	    xmlns:xi="http://www.w3.org/2001/XInclude">
	  <xi:include href="file://`+standard+`" xpointer="xpointer(/dictionary/suite)"/>
	  <suite name="Thing Suite" code="thng">
	    <command name="ponder" code="thngpndr"/>
	  </suite>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := loadFile(own)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := len(d.Suites); got != 2 {
		t.Fatalf("got %d suites, want 2", got)
	}
	if len(d.Includes) != 0 {
		t.Error("the include was left unresolved")
	}
	if !hasCommand(d, "get") || !hasCommand(d, "ponder") {
		t.Error("a command went missing across the splice")
	}
}

// An app whose xpointer removes a command from the standard suite does not
// respond to it, and a dictionary that claims otherwise produces a program
// that fails against the real application.
func TestExcludedCommandsDoNotSurviveTheSplice(t *testing.T) {
	root := t.TempDir()
	standard := filepath.Join(root, "Standard.sdef")
	if err := os.WriteFile(standard, []byte(`<dictionary title="Standard">
	  <suite name="Standard Suite" code="core">
	    <command name="get" code="coregetd"/>
	    <command name="delete" code="coredelo"/>
	    <command name="move" code="coremove"/>
	  </suite>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	own := filepath.Join(root, "Thing.sdef")
	pointer := `xpointer(/dictionary/suite/node()[not(self::command and ((@name = 'delete') or (@name = 'move')))])`
	if err := os.WriteFile(own, []byte(`<dictionary title="Thing"
	    xmlns:xi="http://www.w3.org/2001/XInclude">
	  <xi:include href="file://`+standard+`" xpointer="`+pointer+`"/>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := loadFile(own)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if !hasCommand(d, "get") {
		t.Error("get was dropped along with the excluded commands")
	}
	if hasCommand(d, "delete") || hasCommand(d, "move") {
		t.Error("an excluded command survived, so VANS would offer one the app rejects")
	}
}

func TestNetworkIncludesAreRefused(t *testing.T) {
	root := t.TempDir()
	own := filepath.Join(root, "Thing.sdef")
	if err := os.WriteFile(own, []byte(`<dictionary title="Thing"
	    xmlns:xi="http://www.w3.org/2001/XInclude">
	  <xi:include href="https://example.com/Standard.sdef" xpointer="xpointer(/dictionary/suite)"/>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadFile(own)
	if err == nil || !strings.Contains(err.Error(), "network") {
		t.Errorf("err = %v, want a refusal to fetch over the network", err)
	}
}

func TestUnrecognisedXpointerIsAnError(t *testing.T) {
	root := t.TempDir()
	standard := filepath.Join(root, "Standard.sdef")
	if err := os.WriteFile(standard, []byte(`<dictionary title="Standard"/>`), 0o644); err != nil {
		t.Fatal(err)
	}
	own := filepath.Join(root, "Thing.sdef")
	if err := os.WriteFile(own, []byte(`<dictionary title="Thing"
	    xmlns:xi="http://www.w3.org/2001/XInclude">
	  <xi:include href="file://`+standard+`" xpointer="xpointer(/dictionary/suite/class)"/>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadFile(own); err == nil {
		t.Error("an xpointer VANS cannot honour was accepted, so the surface it reports is a guess")
	}
}

func TestLoadsARealDictionary(t *testing.T) {
	if _, err := os.Stat(notesPath); err != nil {
		t.Skip("Notes is not installed")
	}

	d, err := Load(notesPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !hasCommand(d, "make") {
		t.Error("the standard suite did not come through the include")
	}

	var found bool
	for _, s := range d.Suites {
		for _, c := range s.Classes {
			if c.Name == "note" {
				found = true
			}
		}
	}
	if !found {
		t.Error("Notes' own suite is missing")
	}
}

func hasCommand(d *Dictionary, name string) bool {
	for _, s := range d.Suites {
		for _, c := range s.Commands {
			if strings.EqualFold(c.Name, name) {
				return true
			}
		}
	}
	return false
}

// The including dictionary's own definitions have to win. Mail redefines
// delete, move and duplicate, and resolving an include the other way round
// silently hands back the standard suite's version of a command the app
// deliberately replaced.
func TestTheAppsOwnDefinitionWins(t *testing.T) {
	root := t.TempDir()
	standard := filepath.Join(root, "Standard.sdef")
	if err := os.WriteFile(standard, []byte(`<dictionary title="Standard">
	  <suite name="Standard Suite" code="core">
	    <command name="delete" code="coredelo" description="standard"/>
	  </suite>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	own := filepath.Join(root, "Thing.sdef")
	if err := os.WriteFile(own, []byte(`<dictionary title="Thing"
	    xmlns:xi="http://www.w3.org/2001/XInclude">
	  <xi:include href="file://`+standard+`" xpointer="xpointer(/dictionary/suite)"/>
	  <suite name="Thing Suite" code="thng">
	    <command name="delete" code="thngdelo" description="the app's own"/>
	  </suite>
	</dictionary>`), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := loadFile(own)
	if err != nil {
		t.Fatalf("loadFile: %v", err)
	}

	got, ok := d.FindCommand("delete")
	if !ok {
		t.Fatal("delete went missing")
	}
	if got.Description != "the app's own" {
		t.Errorf("delete resolved to the %q definition", got.Description)
	}
}
