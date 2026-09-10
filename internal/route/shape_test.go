package route

import "testing"

// The property the cache depends on: two instructions asking the same thing
// about different particulars must land on the same key.
func TestTheSameQuestionAboutDifferentThingsSharesAShape(t *testing.T) {
	a, litsA := Shape("the top 10 stories from https://news.ycombinator.com")
	b, litsB := Shape("The top 30 stories from https://lobste.rs")

	if a != b {
		t.Fatalf("shapes differ, so the second would cost a model call:\n  %q\n  %q", a, b)
	}
	if litsA["u0"] != "https://news.ycombinator.com" {
		t.Errorf("url literal = %q", litsA["u0"])
	}
	if litsB["n0"] != "30" {
		t.Errorf("number literal = %q", litsB["n0"])
	}
}

// The other half: shapes must not collapse so far that different requests
// share a decision. A cache that hits on everything is worse than no cache.
func TestDifferentQuestionsDoNotShareAShape(t *testing.T) {
	extract, _ := Shape("the top 10 stories from https://news.ycombinator.com")
	act, _ := Shape("reply to the last message in Telegram")

	if extract == act {
		t.Fatalf("unrelated instructions collapsed to the same shape: %q", extract)
	}
}

func TestBareHostsCountAsURLs(t *testing.T) {
	_, lits := Shape("stories from news.ycombinator.com")
	if lits["u0"] != "news.ycombinator.com" {
		t.Fatalf("bare host not lifted out: %#v", lits)
	}
}

// A quoted run is the user naming something exactly, so it is a value. Two
// notes with different names are the same instruction.
func TestQuotedTextIsAValueNotPartOfTheShape(t *testing.T) {
	a, litsA := Shape(`save them to a note called "Hacker News"`)
	b, _ := Shape(`save them to a note called "Reading list"`)

	if a != b {
		t.Fatalf("quoted text leaked into the shape:\n  %q\n  %q", a, b)
	}
	if litsA["s0"] != "Hacker News" {
		t.Errorf("quote literal = %q, want it unquoted", litsA["s0"])
	}
}

func TestPunctuationDoesNotChangeTheShape(t *testing.T) {
	a, _ := Shape("the top stories from news.ycombinator.com")
	b, _ := Shape("The top stories from news.ycombinator.com!!")
	if a != b {
		t.Fatalf("punctuation changed the shape:\n  %q\n  %q", a, b)
	}
}

// Fill is how a cached decision is replayed: the model's template carries
// placeholders, and this run's literals go into them.
func TestFillPutsThisRunsValuesIntoACachedTemplate(t *testing.T) {
	_, lits := Shape("the top 30 stories from https://lobste.rs")

	if got := Fill("{u0}", lits); got != "https://lobste.rs" {
		t.Errorf("url = %q", got)
	}
	if got := Fill("top {n0} stories", lits); got != "top 30 stories" {
		t.Errorf("intent = %q", got)
	}
}

// A template may reference a slot this instruction did not supply — the model
// wrote it against a sibling phrasing that had a title and this one does not.
// Empty is recoverable; a literal "{s0}" reaching the engine as a URL is not.
func TestAnUnfilledPlaceholderResolvesToEmpty(t *testing.T) {
	_, lits := Shape("the top stories from lobste.rs")
	if got := Fill("{s0}", lits); got != "" {
		t.Fatalf("unfilled placeholder survived as %q", got)
	}
}

// Literals are used verbatim — as a note's name, as a URL to navigate to — so
// they must come back exactly as typed. Only the shape is lowered.
func TestLiteralsKeepTheirCase(t *testing.T) {
	_, lits := Shape(`save https://Example.com/Path to a note called "Hacker News"`)

	if lits["s0"] != "Hacker News" {
		t.Errorf("title = %q, want the case the user typed", lits["s0"])
	}
	if lits["u0"] != "https://Example.com/Path" {
		t.Errorf("url = %q, want the path's case preserved", lits["u0"])
	}
}

// A missing space after a full stop looks exactly like a hostname. Lifting it
// out would change the shape of an ordinary sentence and route it as if it
// carried an address.
func TestProseIsNotMistakenForAHostname(t *testing.T) {
	_, lits := Shape("get the stories.Then save them")
	if u, ok := lits["u0"]; ok {
		t.Fatalf("prose lifted out as a URL: %q", u)
	}
}

func TestTwoPartHostsAreRecognised(t *testing.T) {
	for _, host := range []string{"example.com", "lobste.rs", "news.ycombinator.com"} {
		_, lits := Shape("stories from " + host)
		if lits["u0"] != host {
			t.Errorf("%q not recognised as a host: %#v", host, lits)
		}
	}
}
