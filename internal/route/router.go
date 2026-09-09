package route

import (
	"context"
	"fmt"
	"strings"
)

// Request is what the model needs in order to route an instruction.
type Request struct {
	// Instruction is what the user typed, verbatim. The model reads this for
	// meaning.
	Instruction string

	// Shape is what it must write templates against. Slot values may only
	// reference placeholders that appear here.
	Shape string

	// Placeholders are the placeholder names present in Shape, so the model is
	// told plainly which ones exist rather than having to parse them out.
	Placeholders []string

	// Failures, when non-empty, are the reasons the previous decision was
	// rejected. The second attempt is informed rather than blind, the same way
	// rung 2 of the extraction ladder works (D-020).
	Failures []string
}

// Decider turns an instruction into a decision. The one model call in this
// package, and the only reason it is not deterministic.
type Decider interface {
	Decide(ctx context.Context, req Request) (Decision, error)
}

// Cache remembers decisions by shape.
//
// Declared here and implemented elsewhere so this package stays free of
// storage concerns — and so the store may depend on this package's types
// without the two importing each other.
type Cache interface {
	// Route returns the decision cached for a shape, if there is one.
	Route(ctx context.Context, shape string) (Decision, bool, error)

	// SaveRoute stores a decision against a shape.
	SaveRoute(ctx context.Context, shape string, d Decision, model string) error

	// ForgetRoute drops a shape's decision after it has proved wrong.
	ForgetRoute(ctx context.Context, shape string) error
}

// DefaultMaxDecisions bounds how many model calls one instruction may cost:
// the first attempt, plus one retry that knows why the first was rejected.
const DefaultMaxDecisions = 2

// Router decides what an instruction is asking for.
//
// The warm path makes no model call at all, which is the same claim the
// extraction path makes and the reason routing is done on shape rather than
// text (D-039).
type Router struct {
	Decider Decider
	Cache   Cache

	// Model labels which backend made a decision, stored as provenance.
	Model string

	// MaxDecisions bounds model calls per instruction. Defaults to
	// DefaultMaxDecisions.
	MaxDecisions int

	// Log receives progress lines. Optional.
	Log func(format string, args ...any)
}

func (r *Router) logf(format string, args ...any) {
	if r.Log != nil {
		r.Log(format, args...)
	}
}

func (r *Router) maxDecisions() int {
	if r.MaxDecisions > 0 {
		return r.MaxDecisions
	}
	return DefaultMaxDecisions
}

// Result is a routed instruction and what it cost.
type Result struct {
	Route Route

	// DecisionCalls is how many times the model was asked. Zero is the
	// expected value on a warm run and the number worth watching.
	DecisionCalls int
}

// UndecidedError is the router declining, with everything a person needs to
// either rephrase or recognise that VANS cannot do this yet.
type UndecidedError struct {
	Instruction string
	Shape       string
	Why         string
	Attempts    []string
}

func (e *UndecidedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "route: could not tell what %q is asking for", e.Instruction)
	if e.Why != "" {
		fmt.Fprintf(&b, ": %s", e.Why)
	}
	for i, a := range e.Attempts {
		fmt.Fprintf(&b, "\n  attempt %d: %s", i+1, a)
	}
	return b.String()
}

// Route decides what an instruction is asking for.
func (r *Router) Route(ctx context.Context, instruction string) (Result, error) {
	if strings.TrimSpace(instruction) == "" {
		return Result{}, fmt.Errorf("route: empty instruction")
	}

	shape, lits := Shape(instruction)

	// A shape of nothing but placeholders is a pasted value, not an
	// instruction: "https://news.ycombinator.com" shapes to "{u0}". There are
	// no words to decide from, so asking the model would spend a call to be
	// told what is already obvious here.
	if !hasWords(shape) {
		return Result{}, &UndecidedError{
			Instruction: instruction,
			Shape:       shape,
			Why:         "that is a value, not an instruction — say what to do with it",
		}
	}
	r.logf("instruction shape %q", shape)

	if r.Cache != nil {
		if d, ok, err := r.Cache.Route(ctx, shape); err != nil {
			return Result{}, err
		} else if ok {
			// A cached decision is re-validated on the way out, not trusted.
			// It may predate a change to a kind's required slots, which makes
			// it exactly as unusable as one the model just got wrong.
			if rt, err := d.Resolve(instruction, shape, lits); err == nil {
				r.logf("route cache hit: %s, no model call", rt.Kind)
				rt.FromCache = true
				return Result{Route: rt}, nil
			} else {
				r.logf("cached route no longer resolves (%v); deciding again", err)
				if err := r.Cache.ForgetRoute(ctx, shape); err != nil {
					return Result{}, err
				}
			}
		}
	}

	if r.Decider == nil {
		return Result{}, fmt.Errorf("route: no decider configured")
	}

	req := Request{
		Instruction:  instruction,
		Shape:        shape,
		Placeholders: PlaceholdersIn(shape),
	}

	var attempts []string
	for call := 1; call <= r.maxDecisions(); call++ {
		d, err := r.Decider.Decide(ctx, req)
		if err != nil {
			return Result{}, fmt.Errorf("route: decide: %w", err)
		}

		if d.Kind == KindUnknown {
			return Result{DecisionCalls: call}, &UndecidedError{
				Instruction: instruction,
				Shape:       shape,
				Why:         d.Why,
				Attempts:    attempts,
			}
		}

		// Resolved before it is cached, never after. A decision that cannot be
		// filled from the instruction that produced it will never be fillable,
		// and caching it would make every later instruction of this shape pay
		// for the lookup and then fail anyway.
		rt, err := d.Resolve(instruction, shape, lits)
		if err != nil {
			attempts = append(attempts, err.Error())
			req.Failures = append(req.Failures, err.Error())
			r.logf("decision rejected: %v", err)
			continue
		}

		if r.Cache != nil {
			if err := r.Cache.SaveRoute(ctx, shape, d, r.Model); err != nil {
				return Result{}, err
			}
		}
		r.logf("routed to %s after %d model call(s)", rt.Kind, call)
		return Result{Route: rt, DecisionCalls: call}, nil
	}

	return Result{DecisionCalls: r.maxDecisions()}, &UndecidedError{
		Instruction: instruction,
		Shape:       shape,
		Why:         "no usable decision after every attempt was spent",
		Attempts:    attempts,
	}
}

// hasWords reports whether anything survives a shape once its placeholders
// are taken out.
func hasWords(shape string) bool {
	return strings.TrimSpace(placeholder.ReplaceAllString(shape, " ")) != ""
}

// PlaceholdersIn lists the placeholder names in a shape, in order.
func PlaceholdersIn(shape string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range placeholder.FindAllStringSubmatch(shape, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}
