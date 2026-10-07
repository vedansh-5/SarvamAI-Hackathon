// Package route turns a plain-language instruction into a task the engine can
// run.
//
// It is the selector-program idea (D-002, D-006) applied one level up. There,
// the model writes a program against a page's *structure* so the program
// survives the page's *content* changing. Here, the model decides what a
// sentence is asking for against the sentence's *shape*, so the decision
// survives the sentence's *particulars* changing.
//
//	"the top 10 stories from https://news.ycombinator.com"
//	"the top 30 stories from https://lobste.rs"
//
// Those are the same instruction. A cache keyed on the text would miss; a
// cache keyed on the shape hits, and the second one costs no model call. That
// is the whole reason this package exists rather than routing on raw text.
package route

import (
	"fmt"
	"regexp"
	"strings"
)

// Literals are the concrete values lifted out of an instruction, keyed by the
// placeholder that replaced them.
type Literals map[string]string

// Two-letter TLDs are all country codes, so they are taken on length. Anything
// longer has to be recognised, which is what stops a missing space after a full
// stop — "stories.Then" — from being lifted out of the sentence as a hostname.
var commonTLDs = map[string]bool{
	"com": true, "org": true, "net": true, "edu": true, "gov": true,
	"mil": true, "int": true, "info": true, "biz": true, "xyz": true,
	"io": true, "dev": true, "app": true, "ai": true, "sh": true,
	"tech": true, "news": true, "blog": true, "wiki": true, "site": true,
	"page": true, "cloud": true, "store": true, "online": true, "space": true,
}

// hostLike reports whether a match is addressable enough to treat as a URL.
func hostLike(match string) bool {
	// An explicit scheme is the user being unambiguous. Trust it — this is
	// also what lets localhost and bare IPs through.
	if strings.HasPrefix(strings.ToLower(match), "http") {
		return true
	}
	host, _, _ := strings.Cut(match, "/")
	dot := strings.LastIndex(host, ".")
	if dot < 0 {
		return false
	}
	tld := strings.ToLower(host[dot+1:])
	return len(tld) == 2 || commonTLDs[tld]
}

// Ordered so the greedy patterns run first. A URL contains dots, digits and
// slashes, so leaving it until after the number pass would shred it into
// fragments that no longer look like an address.
var shapePatterns = []struct {
	prefix string
	re     *regexp.Regexp
	accept func(string) bool
}{
	{"p", regexp.MustCompile(`(?:^|\s)(?:~/|/)[^\s"'<>]+`), pathLike},
	// Explicit scheme, or a bare host with a known-ish TLD. Bare hosts are
	// worth catching because people type "news.ycombinator.com" far more often
	// than they type the scheme.
	{"u", regexp.MustCompile(`(?i)https?://[^\s"'<>]+|\b[a-z0-9][a-z0-9-]*(?:\.[a-z0-9-]+)*\.[a-z]{2,}(?:/[^\s"'<>]*)?`), hostLike},
	// Quoted runs are the user naming something exactly — a note title, a
	// button label. They are values, never part of the shape.
	{"s", regexp.MustCompile(`"[^"]*"|'[^']*'|\x{201c}[^\x{201d}]*\x{201d}`), nil},
	{"n", regexp.MustCompile(`\b\d+(?:\.\d+)?\b`), nil},
}

func aroundThePath(match string) (lead, value, tail string) {
	value = strings.TrimLeft(match, " \t\n")
	lead = match[:len(match)-len(value)]
	trimmed := strings.TrimRight(value, ".,;:!?)")
	return lead, trimmed, value[len(trimmed):]
}

func pathLike(match string) bool {
	_, value, _ := aroundThePath(match)
	if strings.HasPrefix(value, "~/") {
		return len(value) > 2
	}
	return strings.Count(value, "/") >= 2
}

var whitespace = regexp.MustCompile(`\s+`)

// Shape reduces an instruction to its reusable form and hands back the values
// it removed.
//
// The shape is what gets cached and what the model is asked about. The
// literals are what the cached answer is replayed against, so a second
// instruction of the same shape reuses the decision with its own values.
func Shape(instruction string) (string, Literals) {
	// Matching happens against the original text, not a lowered copy, because
	// the literals are values that get used verbatim: a note named
	// "Hacker News" must not be written as "hacker news", and a URL path is
	// case-sensitive. Only what survives as shape is lowered, below.
	s := strings.TrimSpace(instruction)
	lits := Literals{}

	for _, p := range shapePatterns {
		n := 0
		s = p.re.ReplaceAllStringFunc(s, func(match string) string {
			// Left in place, and so left as part of the shape. A rejected
			// match is an ordinary word that merely looked like a value.
			if p.accept != nil && !p.accept(match) {
				return match
			}
			key := fmt.Sprintf("%s%d", p.prefix, n)
			n++
			if p.prefix == "p" {
				lead, value, tail := aroundThePath(match)
				lits[key] = value
				return lead + "{" + key + "}" + tail
			}
			lits[key] = trimQuotes(match)
			return "{" + key + "}"
		})
	}

	// Safe to lower now: every value is out of the string, and the
	// placeholders standing in for them are lowercase already.
	s = strings.ToLower(s)

	// Punctuation carries no routing signal, but braces do now, so they are
	// the one thing kept. Doing this after substitution rather than before is
	// deliberate: a URL's own punctuation is already safely inside a literal.
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune("{}", r) || isWordish(r) || r == ' ' {
			return r
		}
		return ' '
	}, s)

	return strings.TrimSpace(whitespace.ReplaceAllString(s, " ")), lits
}

func isWordish(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		r > 127
}

func trimQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return strings.Trim(s, "“”")
}

var placeholder = regexp.MustCompile(`\{([a-z]\d+)\}`)

// Fill substitutes literals back into a template the model wrote.
//
// A placeholder with no literal resolves to empty rather than being left as
// "{u0}", which would otherwise reach the engine as a URL to navigate to and
// fail somewhere far from the cause.
var plainWord = regexp.MustCompile(`[\p{L}\p{N}][\p{L}\p{N}'’_-]*`)

func InTheirCase(template, instruction string) string {
	theirs := plainWord.FindAllString(instruction, -1)
	if len(theirs) == 0 {
		return template
	}
	next := 0
	asWritten := func(word string) string {
		for i := next; i < len(theirs); i++ {
			if strings.EqualFold(theirs[i], word) {
				next = i + 1
				return theirs[i]
			}
		}
		for i := 0; i < next; i++ {
			if strings.EqualFold(theirs[i], word) {
				return theirs[i]
			}
		}
		return word
	}

	var out strings.Builder
	at := 0
	for _, held := range placeholder.FindAllStringIndex(template, -1) {
		out.WriteString(plainWord.ReplaceAllStringFunc(template[at:held[0]], asWritten))
		out.WriteString(template[held[0]:held[1]])
		at = held[1]
	}
	out.WriteString(plainWord.ReplaceAllStringFunc(template[at:], asWritten))
	return out.String()
}

func Fill(template string, lits Literals) string {
	out := placeholder.ReplaceAllStringFunc(template, func(m string) string {
		return lits[placeholder.FindStringSubmatch(m)[1]]
	})
	return strings.TrimSpace(whitespace.ReplaceAllString(out, " "))
}
