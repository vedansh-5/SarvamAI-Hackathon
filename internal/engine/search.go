package engine

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// A search is an extraction whose page has to be found first.
//
// Everything downstream is unchanged: the results page is signed, a program is
// written for it once, and every later search of the same shape replays that
// program. A search engine's results page is about as stable a shape as exists
// on the web, so this is the case the cache was made for.

// searchEngines are the ones reachable by name.
//
// Held as templates rather than built with url.Values because each engine
// spells its query parameter differently and there is no gain in pretending
// otherwise.
var searchEngines = map[string]string{
	"google":     "https://www.google.com/search?q=%s",
	"duckduckgo": "https://duckduckgo.com/?q=%s",
	"ddg":        "https://duckduckgo.com/?q=%s",
	"bing":       "https://www.bing.com/search?q=%s",
	"brave":      "https://search.brave.com/search?q=%s",
}

// DefaultSearchEngine is used when the instruction does not name one.
//
// Not Google. Google randomises the class names on its results page, so its
// signature never repeats: every search would be a cold run, two model calls
// and seven seconds, for the same question asked twice. DuckDuckGo renders
// the same shape every time, which is what makes a search cost nothing the
// second time anyone asks it. Naming another engine in the instruction still
// works, Google included.
const DefaultSearchEngine = "duckduckgo"

// searchIntent is what the extractor is asked for on a results page.
//
// Fixed rather than taken from the user, because the user asked a question and
// said nothing about what a result looks like. Naming the three parts every
// engine renders gives the verifier something to check: a consent wall or a
// captcha has none of them, so it fails loudly instead of returning a page of
// confident nonsense.
const searchIntent = "each search result: its title, the link it points to, and the description under it"

// searchURL builds the address for a query.
func searchURL(engine, query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("engine: search has no query")
	}

	name := strings.ToLower(strings.TrimSpace(engine))
	if name == "" {
		name = DefaultSearchEngine
	}
	// "on Google", "google.com", "Google Search" all mean the same engine, and
	// the model will produce all three.
	name = strings.TrimPrefix(name, "www.")
	name = strings.TrimSuffix(name, ".com")
	name = strings.TrimSuffix(name, " search")

	tmpl, ok := searchEngines[name]
	if !ok {
		var known []string
		for k := range searchEngines {
			known = append(known, k)
		}
		sort.Strings(known)
		return "", fmt.Errorf("engine: no search engine called %q (have %s)",
			engine, strings.Join(known, ", "))
	}
	return fmt.Sprintf(tmpl, url.QueryEscape(query)), nil
}

// searchTitle names the document a search would be written to.
func searchTitle(query string) string {
	return fmt.Sprintf("Search: %s", strings.TrimSpace(query))
}
