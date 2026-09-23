package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/webact"
	"github.com/vedansh/vans/internal/webstyle"
)

// StyleWriter writes a restyle for a page.
type StyleWriter interface {
	WriteStyle(ctx context.Context, req synth.StyleRequest) (synth.StyleAnswer, error)
}

// StylePlan is a restyle waiting to be confirmed.
type StylePlan struct {
	URL     string
	Preview string

	// Clears reports that this plan takes a restyle off rather than putting
	// one on. Undoing needs no confirmation: it puts the page back.
	Clears bool

	program webstyle.Program
	script  string
}

// Asking for the highlighting to go away is recognised here rather than by the
// model. It costs no call, it cannot be got wrong, and "clear" is not an
// instruction anyone wants to wait on a round trip for.
var clearing = regexp.MustCompile(`(?i)\b(clear|remove|undo|reset|revert|unhighlight|un-highlight)\b`)

var styling = regexp.MustCompile(`(?i)\b(highlight|highlighting|restyle|styling|style|mark|marking|colou?r|colou?ring|tint|emphasis)\b`)

func asksToClear(intent string) bool {
	if strings.TrimSpace(intent) == "" {
		return false
	}
	return clearing.MatchString(intent) && styling.MatchString(intent)
}

const maxStyleAttempts = 2

var errNoStyle = errors.New("engine: no restyle survived being checked against the page")

func (e *Engine) canStyle() bool {
	_, ok := e.pageDriver()
	return ok && e.Styler != nil
}

// stylePage prepares a restyle of the page the user is looking at.
//
// Nothing is written and nothing is navigated: the page is read, a restyle is
// written against what is actually in it, and the result waits for Commit like
// every other thing that changes something the user can see.
func (e *Engine) stylePage(ctx context.Context, r route.Route) (*Plan, int, error) {
	if !e.canStyle() {
		return nil, 0, fmt.Errorf("engine: this build cannot restyle a page")
	}
	driver, _ := e.pageDriver()

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	pageURL, src, err := driver.Front(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("engine: read the page in front of you: %w", err)
	}

	doc, err := webact.Parse(src)
	if err != nil {
		return nil, 0, err
	}

	intent := r.Slot("intent")

	if asksToClear(intent) {
		return &Plan{
			Task:        Task{Intent: intent, Title: pageURL},
			PageURL:     pageURL,
			Destination: hostOf(pageURL),
			Style: &StylePlan{
				URL:     pageURL,
				Preview: "Take the highlighting off every page VANS put it on",
				Clears:  true,
				script:  webstyle.Clear(),
			},
		}, 0, nil
	}

	structure := webstyle.StructureText(webstyle.Structure(doc), webstyle.MaxStructureBytes)

	var failures []string
	calls := 0

	for attempt := 1; attempt <= maxStyleAttempts; attempt++ {
		answer, err := e.Styler.WriteStyle(ctx, synth.StyleRequest{
			Intent:    intent,
			URL:       pageURL,
			Structure: structure,
			Failures:  failures,
		})
		calls++
		if err != nil {
			return nil, calls, err
		}

		if len(answer.Rules) == 0 {
			return nil, calls, &UnroutableError{
				Instruction: intent,
				Why:         reasonOr(answer.Why, "nothing on the page matches what you asked to see"),
				Can:         e.capabilities(),
			}
		}

		program := webstyle.Program{Rules: answer.Rules}
		script, err := webstyle.Render(program, doc)
		if err != nil {
			var invalid *webstyle.InvalidError
			if !errors.As(err, &invalid) {
				return nil, calls, err
			}
			failures = invalid.Reasons
			e.logf("restyle refused: %v", invalid.Reasons)
			continue
		}

		return &Plan{
			Task:           Task{Intent: intent, Title: pageURL},
			PageURL:        pageURL,
			Destination:    hostOf(pageURL),
			SynthesisCalls: calls,
			Style: &StylePlan{
				URL:     pageURL,
				Preview: webstyle.Preview(program),
				program: program,
				script:  script,
			},
		}, calls, nil
	}

	return nil, calls, errNoStyle
}

func reasonOr(why, fallback string) string {
	if why != "" {
		return why
	}
	return fallback
}

// runStyle is the side effect: the page in front changes how it looks.
func (e *Engine) runStyle(ctx context.Context, plan *Plan) (Receipt, error) {
	driver, ok := e.pageDriver()
	if !ok {
		return Receipt{}, fmt.Errorf("engine: this build cannot restyle a page")
	}

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	if plan.Style.Clears {
		cleared, err := e.takeTheStyleOff(ctx, driver)
		if err != nil {
			return Receipt{}, err
		}
		e.logf("took the highlighting off %d page(s)", cleared)
		return Receipt{Title: hostOf(plan.Style.URL), Verified: true}, nil
	}

	if err := driver.Do(ctx, plan.Style.script); err != nil {
		return Receipt{}, err
	}
	e.logf("restyled %s", plan.Style.URL)
	return Receipt{Title: hostOf(plan.Style.URL), Verified: true}, nil
}

// ClearStyle takes every restyle off, with no plan and no gate.
//
// Undoing is not a side effect anyone needs protecting from, and a button that
// puts the page back should put the page back rather than ask twice.
func (e *Engine) ClearStyle(ctx context.Context) error {
	driver, ok := e.pageDriver()
	if !ok {
		return fmt.Errorf("engine: this build cannot restyle a page")
	}

	if l, isLock := e.Browser.(sync.Locker); isLock {
		l.Lock()
		defer l.Unlock()
	}

	cleared, err := e.takeTheStyleOff(ctx, driver)
	if err != nil {
		return err
	}
	e.logf("took the highlighting off %d page(s)", cleared)
	return nil
}

// EveryTabDriver can reach every tab, not only the one in front. Implemented by
// the live browser adapter; the debugged one drives a single tab and does not.
type EveryTabDriver interface {
	DoOnEveryTab(ctx context.Context, script string) (int, error)
}

// takeTheStyleOff clears every tab that carries a restyle, and reports how many
// did.
//
// Every tab rather than the one in front, because a restyle outlives being
// looked at: highlight one page, move to another, ask for the highlighting to
// go, and the first page stays marked with nothing left pointing at it. An
// adapter that can only reach the front tab still clears that one — a partial
// undo beats none — and the log says which happened.
func (e *Engine) takeTheStyleOff(ctx context.Context, driver PageDriver) (int, error) {
	if everywhere, canReachThemAll := driver.(EveryTabDriver); canReachThemAll {
		cleared, err := everywhere.DoOnEveryTab(ctx, webstyle.Clear())
		if err == nil {
			return cleared, nil
		}
		e.logf("could not reach every tab (%v); clearing the one in front", err)
	}

	if err := driver.Do(ctx, webstyle.Clear()); err != nil {
		return 0, err
	}
	return 1, nil
}
