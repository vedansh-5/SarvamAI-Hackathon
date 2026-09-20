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
				Can:         e.capabilities(),
			}
		}
		return nil, err
	}

	if e.History != nil {
		if err := e.History.RecordInstruction(ctx, in.Instruction, string(res.Route.Kind)); err != nil {
			e.logf("could not remember the instruction: %v", err)
		}
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

			// The question is the user's, not the runtime's rewriting of it.
			// searchIntent describes what a results page looks like; it is not
			// what anybody asked.
			e.answerFor(ctx, in.Instruction, plan)
		}
		return plan, err

	case route.KindPageHere:
		plan, err := e.readWhatIsInFront(ctx, res.Route.Slot("intent"), in.Context)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		plan.Destination = in.Destination
		e.answerFor(ctx, in.Instruction, plan)
		return plan, nil

	case route.KindAppGuide:
		// The user's own words, not the slot: routing reduces "where is the
		// search bar" to "the search bar", and a noun is not a task anyone can
		// work out the next step of.
		plan, err := e.planGuide(ctx, in.Instruction, in.Context)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		return plan, nil

	case route.KindPageStyle:
		plan, calls, err := e.stylePage(ctx, res.Route)
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		plan.SynthesisCalls = calls
		return plan, nil

	case route.KindPageAction:
		plan, calls, err := e.actOnPage(ctx, res.Route, res.Shape, "")
		if err != nil {
			return nil, err
		}
		r := res.Route
		plan.Route = &r
		plan.DecisionCalls = res.DecisionCalls
		plan.SynthesisCalls = calls
		return plan, nil

	case route.KindAppAction:
		r := res.Route
		if strings.TrimSpace(r.Slot("app")) == "" {
			if front, err := e.appInQuestion(ctx, in.Context); err == nil && front != "" {
				if r.Slots == nil {
					r.Slots = make(map[string]string)
				}
				r.Slots["app"] = front
			}
		}
		if e.canAct() {
			plan, calls, err := e.actInApplication(ctx, r, res.Shape)
			if err != nil {
				return nil, err
			}
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
			plan.SynthesisCalls = calls
			return plan, nil
		}
		if e.canActOnUI() {
			plan, calls, err := e.actOnUI(ctx, r, res.Shape, r.Slot("app"))
			if err != nil {
				return nil, err
			}
			r := res.Route
			plan.Route = &r
			plan.DecisionCalls = res.DecisionCalls
			plan.SynthesisCalls = calls
			return plan, nil
		}
	}

	// A kind the router can choose but this build cannot run. Said plainly
	// rather than failing somewhere downstream: the router is deliberately
	// ahead of the adapters, so this is an expected state and not a bug to be
	// discovered from a stack trace.
	return nil, &UnroutableError{
		Instruction: in.Instruction,
		Why: fmt.Sprintf(
			"that is a %s task, which VANS can recognise but cannot carry out yet",
			res.Route.Kind),
		Can: e.capabilities(),
	}
}

// capabilities describes what this build can actually run, which is not the
// same as what can be routed — and not the same across builds either. Asked of
// the engine rather than of a constant, so the list a user is shown after
// "I did not catch that" cannot offer something this particular runtime was
// not assembled to do.
func (e *Engine) capabilities() []string {
	var out []string
	for _, spec := range route.Specs {
		if !e.canRun(spec.Kind) {
			continue
		}
		out = append(out, spec.Describe)
	}
	return out
}

func (e *Engine) canRun(k route.Kind) bool {
	switch k {
	case route.KindWebExtract, route.KindWebSearch, route.KindPageHere:
		return true
	case route.KindPageAction:
		_, ok := e.pageDriver()
		return ok && e.WebWriter != nil
	case route.KindPageStyle:
		return e.canStyle()
	case route.KindAppGuide:
		return e.canGuide()
	case route.KindAppAction:
		return e.canAct() || e.canActOnUI()
	default:
		return false
	}
}

// canAct reports whether every piece acting in an application needs is here.
// All four together or none: a build with some of them would decline halfway
// through a request rather than before starting one.
func (e *Engine) canAct() bool {
	return e.Apps != nil && e.Writer != nil && e.Actions != nil && e.Runner != nil
}
