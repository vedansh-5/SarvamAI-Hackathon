// Package synth turns candidate page regions into an extraction program.
//
// This is the only component in the runtime that calls a language model. Every
// other package is deterministic, and that boundary is deliberate: synthesis
// happens once per page shape, and everything downstream replays its output
// without further inference.
package synth

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/vedansh/vans/internal/extract"
)

// Request is everything the model needs to write a program.
type Request struct {
	// Intent is the user's description of what to extract.
	Intent string

	// URL is the page the candidates came from, for context only.
	URL string

	// Candidates are the repeating regions found deterministically.
	Candidates []extract.Candidate

	// Failures, when non-empty, are the reasons a previous attempt was
	// rejected. Rung 2 of the failure ladder re-synthesizes with these in
	// hand rather than blindly retrying.
	Failures []string
}

// Synthesizer writes an extraction program for a request.
type Synthesizer interface {
	Synthesize(ctx context.Context, req Request) (extract.Program, error)
}

// Counter wraps a Synthesizer and counts calls.
//
// This exists so the runtime's central claim is testable: on a warm run the
// count must be exactly zero. That belongs in the test suite as an assertion,
// not in a README as a promise.
type Counter struct {
	Inner Synthesizer
	n     atomic.Int64
}

// NewCounter wraps s.
func NewCounter(s Synthesizer) *Counter { return &Counter{Inner: s} }

func (c *Counter) Synthesize(ctx context.Context, req Request) (extract.Program, error) {
	c.n.Add(1)
	return c.Inner.Synthesize(ctx, req)
}

// Calls reports how many times the model has been asked to synthesize.
func (c *Counter) Calls() int64 { return c.n.Load() }

// Reset zeroes the count.
func (c *Counter) Reset() { c.n.Store(0) }

// systemPrompt explains the contract. It is written plainly on purpose:
// emphasis markers and prohibition lists tend to make current models
// over-apply instructions rather than follow them more closely.
const systemPrompt = `You write extraction programs for a browser automation runtime.

A page has already been scanned for repeating structures, and you are given the candidate regions that were found. Your job is to pick the region that holds what the user asked for, and describe how to read each field out of it.

How a program is executed:
- "root" is a CSS selector run against the whole document. Every element it matches becomes one row.
- Each field's "selector" is run against a single row's element. An empty selector means the row element itself.
- "attr" is "text" for the element's visible text, or the name of an attribute to read.

The "expect" block is a contract. The runtime enforces it on every future run of this program, long after this conversation, and treats a violation as a signal that the page changed and the program needs rewriting. Set "min_rows" to the fewest rows a correct run should produce, and list under "required" only the fields that genuinely must be present in every row — a field that is legitimately blank for some records does not belong there.

Prefer selectors built from stable, meaningful class names and element structure. Class names containing what looks like a build hash are regenerated on every deploy; anchor on something else where you can.

If none of the candidates contain what the user asked for, pick the closest one and set "min_rows" to 1.`

// BuildPrompt renders the user-facing half of the request.
func BuildPrompt(req Request) string {
	var b strings.Builder

	fmt.Fprintf(&b, "The user wants: %s\n", strings.TrimSpace(req.Intent))
	if req.URL != "" {
		fmt.Fprintf(&b, "Page: %s\n", req.URL)
	}

	if len(req.Failures) > 0 {
		b.WriteString("\nA previous program was rejected for these reasons. Address them:\n")
		for _, f := range req.Failures {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}

	fmt.Fprintf(&b, "\n%d candidate regions were found:\n", len(req.Candidates))
	for i, c := range req.Candidates {
		fmt.Fprintf(&b, "\n--- candidate %d (%d records) ---\n", i+1, c.RecordCount)
		fmt.Fprintf(&b, "selector: %s\n", c.Selector)
		fmt.Fprintf(&b, "first record markup:\n%s\n", c.Skeleton)
		if len(c.Samples) > 0 {
			b.WriteString("sample record text:\n")
			for _, s := range c.Samples {
				fmt.Fprintf(&b, "  - %s\n", s)
			}
		}
	}

	return b.String()
}
