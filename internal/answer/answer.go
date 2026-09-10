// Package answer turns extracted rows into a reply to the question that was
// asked.
//
// Everything else in the runtime caches because it is about a *shape* — this
// kind of instruction, this kind of page. An answer is about this content, so
// it cannot be. That is the honest boundary: routing and extraction cost
// nothing on a warm run, and the answer costs one small call over rows that
// are already in hand.
//
// The rows are the only source. A model asked a question with search results
// in front of it will answer from memory when the results are thin, and be
// fluent and wrong. Every sentence has to point at a row, and an answer that
// points nowhere is refused.
package answer

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/vedansh/vans/internal/runner"
)

// Answer is a reply and where each part of it came from.
type Answer struct {
	// Text is the reply, in as few words as the question allows.
	Text string `json:"text"`

	// Cites are the rows the reply rests on, one-based as the model was shown
	// them. Empty is a refusal, not an answer.
	Cites []int `json:"cites"`
}

// UngroundedError is an answer that cannot be traced back to the rows.
type UngroundedError struct{ Reasons []string }

func (e *UngroundedError) Error() string {
	return "answer: not grounded in the results: " + strings.Join(e.Reasons, "; ")
}

var link = regexp.MustCompile(`https?://[^\s)\]",]+`)

// Validate refuses an answer the rows do not support.
//
// These are the floors. They do not ask the model whether it was careful; they
// check the answer against what it was given.
func Validate(a Answer, rows []runner.Row) error {
	var reasons []string

	if strings.TrimSpace(a.Text) == "" {
		reasons = append(reasons, "it says nothing")
	}

	// No citations at all means the answer came from somewhere other than the
	// page — which is the failure worth catching, because it reads exactly
	// like one that did not.
	if len(a.Cites) == 0 {
		reasons = append(reasons, "it points at none of the results")
	}
	for _, c := range a.Cites {
		if c < 1 || c > len(rows) {
			reasons = append(reasons, fmt.Sprintf("it cites result %d, and there are %d", c, len(rows)))
		}
	}

	// A link the rows do not contain was invented. Anything else in the text
	// is prose and can be judged by reading; a fabricated URL looks exactly
	// like a real one.
	haystack := strings.ToLower(strings.Join(values(rows), " "))
	for _, u := range link.FindAllString(a.Text, -1) {
		if !strings.Contains(haystack, strings.ToLower(strings.TrimRight(u, "./"))) {
			reasons = append(reasons, fmt.Sprintf("it contains a link that is not in the results: %s", u))
		}
	}

	if len(reasons) == 0 {
		return nil
	}
	sort.Strings(reasons)
	return &UngroundedError{Reasons: reasons}
}

func values(rows []runner.Row) []string {
	var out []string
	for _, r := range rows {
		for _, v := range r {
			out = append(out, v)
		}
	}
	return out
}

// Cited returns the rows an answer rests on, in the order it cited them.
func Cited(a Answer, rows []runner.Row) []runner.Row {
	var out []runner.Row
	seen := map[int]bool{}
	for _, c := range a.Cites {
		if c < 1 || c > len(rows) || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, rows[c-1])
	}
	return out
}
