package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/screen"
)

// Ask is a free-text request: what the user typed, and where they want it.
type Ask struct {
	// Instruction is what the user asked for, in their own words.
	Instruction string

	// Destination names where the answer goes. Empty means the panel.
	Destination string

	// Context is what the user was looking at when they asked. Optional, and
	// deliberately not required: the runtime must work when the screen cannot
	// be read, which is the ordinary case before Screen Recording is granted.
	Context *screen.Context
}

// UnroutableError is the router declining, carried across the engine boundary
// so the caller can tell "I did not understand" from "I understood and it
// failed". They need entirely different things said to the user.
type UnroutableError struct {
	Instruction string
	Why         string

	// Can lists what VANS is able to do, so the answer to "I can't do that"
	// is followed by what it can. A refusal with no alternative is a dead end.
	Can []string
}

func (e *UnroutableError) Error() string {
	msg := fmt.Sprintf("engine: could not tell what %q is asking for", e.Instruction)
	if e.Why != "" {
		msg += ": " + e.Why
	}
	return msg
}

// Ask routes an instruction and prepares whatever it turns out to be.
//
// Nothing here touches the world. Routing reads no state and extraction reads
// a page; the side effect, if the destination has one, waits for Commit.
func (e *Engine) Ask(ctx context.Context, in Ask) (*Plan, error) {
	if strings.TrimSpace(in.Instruction) == "" {
		return nil, fmt.Errorf("engine: no instruction")
	}
	if e.Router == nil {
		return nil, fmt.Errorf("engine: no router configured")
	}

	res, err := e.Router.Route(ctx, in.Instruction)
	if err != nil {
		var undecided *route.UndecidedError
		if errors.As(err, &undecided) {
			return nil, &UnroutableError{
				Instruction: in.Instruction,
				Why:         undecided.Why,
				Can:         capabilities(),
			}
		}
		return nil, err
	}

	switch res.Route.Kind {
	case route.KindWebExtract:
		plan, err := e.Prepare(ctx, Task{
			URL:         res.Route.Slot("url"),
			Intent:      res.Route.Slot("intent"),
			Title:       res.Route.Slot("title"),
			Destination: in.Destination,
		})
		if plan != nil {
			r := res.Route
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
		}
		return plan, err

	case route.KindWebSearch:
		query := res.Route.Slot("query")
		target, err := searchURL(res.Route.Slot("engine"), query)
		if err != nil {
			return nil, err
		}

		// The intent is the runtime's, not the user's: they asked a question
		// and said nothing about what a result looks like. From here it is an
		// ordinary extraction, so the results page is signed and cached like
		// any other — and a search engine's results page is about as stable a
		// shape as the web has.
		plan, err := e.Prepare(ctx, Task{
			URL:         target,
			Intent:      searchIntent,
			Title:       searchTitle(query),
			Destination: in.Destination,
		})
		if plan != nil {
			r := res.Route
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
		}
		return plan, err

	default:
		// A kind the router can choose but the engine cannot yet run. Said
		// plainly rather than failing somewhere downstream: the router is
		// deliberately ahead of the adapters, so this is an expected state
		// and not a bug to be discovered from a stack trace.
		return nil, &UnroutableError{
			Instruction: in.Instruction,
			Why: fmt.Sprintf(
				"that is a %s task, which VANS can recognise but cannot carry out yet",
				res.Route.Kind),
			Can: capabilities(),
		}
	}
}

// capabilities describes what can actually be run, which is not the same as
// what can be routed.
func capabilities() []string {
	var out []string
	for _, spec := range route.Specs {
		if !runnable[spec.Kind] {
			continue
		}
		out = append(out, spec.Describe)
	}
	return out
}

// runnable marks the kinds that have an adapter behind them. Anything routed
// to a kind not listed here is recognised and declined, which is a better
// answer than attempting it.
var runnable = map[route.Kind]bool{
	route.KindWebExtract: true,
	route.KindWebSearch:  true,
}
