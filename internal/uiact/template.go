package uiact

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// MinSubstringLiteral is the shortest value that may be swapped out from inside
// a longer string.
//
// Below it, substitution does more harm than good: replacing every "a" or "10"
// found inside other words corrupts the step rather than generalising it. A whole
// field that *equals* a value has no such problem and is swapped at any length.
const MinSubstringLiteral = 3

var placeholderToken = regexp.MustCompile(`\{[a-z]\d+\}`)

// Templated is a program with one instruction's values lifted back out of it, and
// the verdict on whether what came out is safe to replay.
type Templated struct {
	Program Program

	// Pinned names the values the program must be replayed with, and is set only
	// where one of them could not be lifted out. Empty is the good case: the
	// program is general and answers for any values of its shape.
	//
	// In practice this comes from control names rather than typed text — "Tab 2"
	// for "close tab 2". Typed text with a value too short to lift is refused
	// outright instead, because whatever surrounds it is then unaccounted for.
	Pinned map[string]string

	// Why names what makes the program unsafe to keep at all, empty when it is
	// safe. It is written to be logged: a run that is not cached should say so in
	// words, or the cache silently never fills and nobody knows why.
	Why string
}

// Keep reports that this program is worth storing.
func (t Templated) Keep() bool { return t.Why == "" }

// Steps is how many steps came out.
func (t Templated) Steps() int { return len(t.Program.Steps) }

// Templatize turns a run that happened into a program that can be replayed.
//
// It is the inverse of Program.Fill, and the mirror of Baked: where Baked
// catches a model writing a value in where a placeholder belongs, this puts the
// placeholder back once the run is over. Doing it afterwards rather than asking
// the model for placeholders keeps the prompt as it is — the model sees the
// concrete task, which is what it reasons about best.
//
// Text is judged much more harshly than a selector, and the asymmetry is the
// point. A selector that has gone stale fails validation on replay and the run
// resumes live, so a wrong one is caught. Text is typed in; nothing downstream
// can tell that the words are last week's.
//
// So every word a step types has to be traceable to the request: a placeholder, a
// value pinned in place, or a word of `asked` that was never a value at all. That
// last one is the common case and it is safe for a reason worth stating — two
// instructions with the same shape differ *only* in their literal values, so
// anything in the instruction that is not a literal is identical for every request
// that will ever hit this key. "type hello into the document" lifts no literals at
// all, because Shape lifts URLs, quoted strings and numbers and "hello" is none of
// them; the word is part of the key, so baking it in is exact rather than risky.
//
// What is left over after that is a word the model supplied itself — composed, or
// read off the window — and that is what makes a run unkeepable.
func Templatize(p Program, asked string, literals map[string]string) Templated {
	out := Program{App: p.App, Steps: make([]Step, len(p.Steps))}
	pinned := map[string]string{}
	values := byLongestFirst(literals)

	for i, step := range p.Steps {
		step.Text = lift(step.Text, values, pinned)
		step.Target.Title = lift(step.Target.Title, values, pinned)

		// Describe is shown to a person, never acted on. A stale one would make
		// the confirmation describe the wrong thing, so it is dropped rather than
		// kept wrong — Preview writes a fresh description from the step itself.
		step.Describe = lift(step.Describe, values, pinned)
		if leftOver(step.Describe, literals) {
			step.Describe = ""
		}

		out.Steps[i] = step
	}

	for i, step := range out.Steps {
		if word := contributed(step.Text, asked, literals); word != "" {
			return Templated{Why: fmt.Sprintf(
				"step %d types %q, which is not a value from the request and not in what "+
					"was asked, so replaying it would type this run's words into a different one",
				i+1, truncate(word, 40))}
		}
	}

	if len(pinned) == 0 {
		return Templated{Program: out}
	}
	return Templated{Program: out, Pinned: pinned}
}

// Longest first, so a value that contains another is swapped before the one it
// contains. Substituting "hello" ahead of "hello world" would leave "{s0} world"
// standing for the wrong thing.
func byLongestFirst(literals map[string]string) [][2]string {
	out := make([][2]string, 0, len(literals))
	for key, value := range literals {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out = append(out, [2]string{key, value})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i][1]) != len(out[j][1]) {
			return len(out[i][1]) > len(out[j][1])
		}
		return out[i][0] < out[j][0]
	})
	return out
}

// lift swaps values for their placeholders, recording any it had to leave.
func lift(field string, values [][2]string, pinned map[string]string) string {
	if strings.TrimSpace(field) == "" {
		return field
	}

	for _, pair := range values {
		key, value := pair[0], pair[1]

		// A whole field that is the value can be swapped however short it is:
		// there is no surrounding text to damage. This is the common case — a
		// typing step usually types exactly what was asked for.
		if strings.EqualFold(strings.TrimSpace(field), value) {
			return "{" + key + "}"
		}
		// Looked for in the text that is still text, not in the placeholders
		// already put in. A token carries its value's index as a digit, so "{s0}"
		// contains "0" — and a request whose own value is "0" would otherwise be
		// found inside the placeholder standing for something else.
		if !strings.Contains(placeholderToken.ReplaceAllString(field, " "), value) {
			continue
		}
		if len(value) < MinSubstringLiteral {
			// Present, and too short to swap out of the middle of something.
			// Kept as it is, and the trace is pinned to it instead.
			pinned[key] = value
			continue
		}
		field = strings.ReplaceAll(field, value, "{"+key+"}")
	}
	return field
}

// contributed names the first word of a field that the model supplied itself, or
// empty when every word of it came from the request.
//
// Word by word rather than as a whole string, because taking the placeholders out
// leaves gaps: "type {s0} now" becomes "type  now", which is in no instruction
// anywhere. The words are what can be checked.
func contributed(field, asked string, literals map[string]string) string {
	if strings.TrimSpace(field) == "" {
		return ""
	}

	rest := placeholderToken.ReplaceAllString(field, " ")
	for _, value := range literals {
		if strings.TrimSpace(value) == "" {
			continue
		}
		rest = strings.ReplaceAll(rest, value, " ")
	}

	for _, word := range strings.Fields(rest) {
		if !strings.Contains(asked, word) {
			return word
		}
	}
	return ""
}

func leftOver(field string, literals map[string]string) bool {
	for _, value := range literals {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(field, value) {
			return true
		}
	}
	return false
}
