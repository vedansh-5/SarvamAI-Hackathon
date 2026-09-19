package engine

import (
	"context"
	"fmt"
	"time"

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

func (e *Engine) planGuide(ctx context.Context, task string) (*Plan, error) {
	if !e.canGuide() {
		return nil, fmt.Errorf("engine: this build cannot show you what to do on screen")
	}

	app, err := e.UIScreen.Frontmost(ctx)
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

func (e *Engine) RunGuide(ctx context.Context, plan *Plan, show func(GuideStep)) (int, error) {
	if !e.canGuide() {
		return 0, fmt.Errorf("engine: this build cannot show you what to do on screen")
	}

	g := plan.Guide
	point := func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) error {
		x, y, w, h, why := tree.RectFor(program.Steps[0].Target)
		if why != "" {
			return fmt.Errorf("engine: %s", why)
		}
		show(GuideStep{Number: number, Label: describe, X: x, Y: y, W: w, H: h})

		// The hand is theirs. Nothing is clicked here — the next step is only
		// worked out once the window shows they have done this one.
		return e.waitForTheUserToAct(ctx, g.App, tree)
	}
	return e.stepThroughTask(ctx, g.App, g.Task, MaxGuideSteps, point)
}

func (e *Engine) waitForTheUserToAct(ctx context.Context, app string, before uiact.Tree) error {
	deadline := time.Now().Add(guideStepTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(guidePoll):
		}

		if after, err := e.UIScreen.Capture(ctx, app); err == nil {
			if len(uiact.Changes(before, after)) > 0 {
				return nil
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("engine: nothing happened in %s, so the steps stopped", app)
		}
	}
}
