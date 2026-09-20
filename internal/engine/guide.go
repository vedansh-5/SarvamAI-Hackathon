package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/uiact"
)

const MaxGuideSteps = 12

// Long enough to find a control and read a line about it, short enough that a
// person who walked away is not left with a dimmed screen. Polling is how we
// notice they acted, since nothing tells us.
var (
	guidePoll        = 500 * time.Millisecond
	guideStepTimeout = 45 * time.Second
)

// How many quiet spells one step is pointed at through before the coach accepts
// that nobody is coming back. Three is a couple of minutes: long enough to find
// a control, short enough not to leave a dimmed screen up for someone who walked
// away.
const timesToRepeatAStep = 3

type GuidePlan struct {
	App     string
	Task    string
	Preview string
}

type GuideStep struct {
	Number int
	Label  string
	X, Y   int
	W, H   int
}

func (e *Engine) canGuide() bool {
	return e.Steps != nil && e.UIScreen != nil
}

// The application a question about "this window" is about.
//
// Not simply whichever application is in front when the question arrives: the
// panel is summoned over the work, and on a Mac with Stage Manager on that is
// enough to withdraw the window the user was looking at. The shell records who
// was in front at the moment the hotkey was pressed, and that answer outlives
// the panel appearing.
func (e *Engine) appInQuestion(ctx context.Context, lookingAt *screen.Context) (string, error) {
	if lookingAt != nil {
		if name := strings.TrimSpace(lookingAt.AppName); name != "" && !strings.EqualFold(name, "vans") {
			return name, nil
		}
	}
	return e.UIScreen.Frontmost(ctx)
}

func (e *Engine) planGuide(ctx context.Context, task string, lookingAt *screen.Context) (*Plan, error) {
	if !e.canGuide() {
		return nil, fmt.Errorf("engine: this build cannot show you what to do on screen")
	}

	app, err := e.appInQuestion(ctx, lookingAt)
	if err != nil {
		return nil, err
	}

	tree, err := e.UIScreen.Capture(ctx, app)
	if err != nil {
		return nil, err
	}
	if len(tree.Elements) == 0 {
		return nil, fmt.Errorf("engine: %s has nothing on screen to point at", app)
	}

	return &Plan{
		Task:        Task{Intent: task, Title: tree.App},
		Destination: tree.App,
		Guide: &GuidePlan{
			App:  tree.App,
			Task: task,
			Preview: fmt.Sprintf("Show you how to do this in %s, a step at a time:\n\n  %s",
				tree.App, task),
		},
	}, nil
}

func (e *Engine) RunGuide(ctx context.Context, plan *Plan, show func(GuideStep), advance <-chan struct{}) (int, error) {
	if !e.canGuide() {
		return 0, fmt.Errorf("engine: this build cannot show you what to do on screen")
	}

	g := plan.Guide

	// The panel held the keyboard while the question was typed, and the person is
	// about to act in their own application rather than in VANS. Handing it back
	// is also what keeps the window readable: macOS withdraws the windows of an
	// application that is not the active one (D-075).
	if err := e.UIScreen.BringToFront(ctx, g.App); err != nil {
		return 0, err
	}

	point := func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) error {
		x, y, w, h, why := tree.RectFor(program.Steps[0].Target)
		if why != "" {
			return fmt.Errorf("engine: %s", why)
		}

		// A step nobody has done yet is still the step. Pointing at it again
		// costs nothing and asks the model for nothing; starting over from the
		// first step because someone was slow is the behaviour being avoided.
		for attempt := 1; ; attempt++ {
			show(GuideStep{Number: number, Label: describe, X: x, Y: y, W: w, H: h})

			// The hand is theirs. Nothing is clicked here — the next step is only
			// worked out once the window shows they have done this one.
			err := e.waitForTheUserToAct(ctx, g.App, tree, advance)
			if !errors.Is(err, errStillWaiting) {
				return err
			}
			if attempt >= timesToRepeatAStep {
				return fmt.Errorf("engine: nothing happened in %s, so the steps stopped", g.App)
			}
			e.logf("still on step %d: %s", number, describe)
		}
	}
	steps, _, err := e.stepThroughTask(ctx, g.App, asTask(g.Task), MaxGuideSteps, point)
	return steps, err
}

// The step writer works out the next thing to do; a question is not a thing to
// do, and left as one it answers "nothing to do" and stops. Asking which
// control answers the question turns it back into a step, and the step's target
// is what gets pointed at.
func asTask(question string) string {
	return fmt.Sprintf(
		"The person is looking at this window and asked: %q\n\n"+
			"Work out which control on screen answers that, or the next step in completing it, and give it as the step to take. "+
			"Describe it as you would to someone who cannot find it. "+
			"They are performing the actions themselves. "+
			"Pointing at a control is the only way you can answer: describing where something is in words reaches nobody. "+
			"Never mark done on the first turn — the control has not been pointed at yet. "+
			"If they asked where something is, point at it, and mark done on the next turn, once it appears in what has been done so far. "+
			"If they asked how to accomplish something, mark done only when it is fully accomplished or nothing further is needed.",
		question)
}

// Nothing has happened yet, which is not a failure until it has gone on long
// enough that nobody is there.
var errStillWaiting = errors.New("engine: the window has not changed yet")

func (e *Engine) waitForTheUserToAct(ctx context.Context, app string, before uiact.Tree, advance <-chan struct{}) error {
	deadline := time.Now().Add(guideStepTimeout)
	consecutiveFailures := 0

	if advance != nil {
		select {
		case <-advance:
		default:
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-advance:
			return nil
		case <-time.After(guidePoll):
		}

		after, err := e.UIScreen.Capture(ctx, app)
		if err != nil || len(after.Elements) == 0 {
			consecutiveFailures++
			if consecutiveFailures >= 3 {
				return fmt.Errorf("engine: %s is no longer open on screen", app)
			}
			e.logf("waiting for %s to reappear...", app)
			continue
		}
		consecutiveFailures = 0

		if len(uiact.Changes(before, after)) > 0 {
			return nil
		}

		if time.Now().After(deadline) {
			return errStillWaiting
		}
	}
}
