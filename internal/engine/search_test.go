package engine

import (
	"strings"
	"testing"
)

func TestASearchGoesToTheNamedEngine(t *testing.T) {
	got, err := searchURL("duckduckgo", "vedansh saini")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://duckduckgo.com/") {
		t.Errorf("url = %q", got)
	}
	if !strings.Contains(got, "vedansh+saini") {
		t.Errorf("query not escaped into the url: %q", got)
	}
}

func TestAnUnnamedEngineFallsBackToTheDefault(t *testing.T) {
	got, err := searchURL("", "widgets")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "google.com") {
		t.Errorf("url = %q, want the default engine", got)
	}
}

// "on Google", "google.com" and "Google Search" all mean the same engine, and
// a model asked which engine was named will produce all three.
func TestEngineNamesAreRecognisedHoweverTheyArePhrased(t *testing.T) {
	for _, name := range []string{"Google", "google.com", "www.google.com", "Google Search"} {
		got, err := searchURL(name, "widgets")
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if !strings.Contains(got, "google.com/search") {
			t.Errorf("%q produced %q", name, got)
		}
	}
}

// Characters that would otherwise end the query string or start a new
// parameter. A search for `a&b=c` must search for that, not for `a`.
func TestAQueryCannotForgeURLParameters(t *testing.T) {
	got, err := searchURL("google", "a&b=c #tag")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "&b=") || strings.Contains(got, "#") {
		t.Errorf("query broke out of the parameter: %q", got)
	}
}

func TestAnUnknownEngineIsRejectedWithWhatIsAvailable(t *testing.T) {
	_, err := searchURL("altavista", "widgets")
	if err == nil {
		t.Fatal("an unknown engine was accepted")
	}
	if !strings.Contains(err.Error(), "google") {
		t.Errorf("the error should list what is available, got: %v", err)
	}
}

func TestAnEmptyQueryIsRejected(t *testing.T) {
	if _, err := searchURL("google", "   "); err == nil {
		t.Fatal("an empty search was accepted")
	}
}
