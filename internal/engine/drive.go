package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

const MaxDriveSteps = 12

type StepWriter interface {
	WriteStep(ctx context.Context, req synth.StepRequest) (synth.StepAnswer, error)
}

type DrivePlan struct {
	App     string
	Task    string
	Preview string
}

func (e *Engine) canDrive() bool {
	return e.Steps != nil && e.UIScreen != nil
}

func (e *Engine) driveOn(ctx context.Context, app, task string) (*Plan, error) {
	if !e.canDrive() {
		return nil, fmt.Errorf("engine: this build cannot work through a task on screen")
	}

	tree, err := e.UIScreen.Capture(ctx, app)
	if err != nil {
		return nil, err
	}
	if len(tree.Elements) == 0 {
		return nil, fmt.Errorf("engine: %s has nothing on screen to work with", app)
	}

	preview := fmt.Sprintf(
		"Work through this in %s, on screen, one step at a time:\n\n  %s\n\n"+
			"Up to %d steps. Every click and every keystroke happens where you can watch it.",
		tree.App, task, MaxDriveSteps)

	return &Plan{
		Task:        Task{Intent: task, Title: tree.App},
		Destination: tree.App,
		Drive:       &DrivePlan{App: tree.App, Task: task, Preview: preview},
	}, nil
}

func (e *Engine) runDrive(ctx context.Context, plan *Plan) (Receipt, error) {
	if !e.canDrive() {
		return Receipt{}, fmt.Errorf("engine: this build cannot work through a task on screen")
	}

	d := plan.Drive
	act := func(ctx context.Context, tree uiact.Tree, program uiact.Program, _ int, _ string) error {
		return e.UIScreen.Act(ctx, program, tree)
	}

	// Verified is evidence, not a claim. The model saying the task is done is
	// the thing being checked, so it cannot also be the check (D-004): what
	// counts is that the window moved for every step VANS performed.
	_, landed, err := e.stepThroughTask(ctx, d.App, d.Task, MaxDriveSteps, act)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{Title: d.App, Verified: landed}, nil
}

type stepHandler func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) error

func (e *Engine) stepThroughTask(ctx context.Context, app, task string, limit int, handle stepHandler) (steps int, landed bool, err error) {
	var done []string
	landed = true

	for i := 1; i <= limit; i++ {
		tree, err := e.UIScreen.Capture(ctx, app)
		if err != nil {
			return len(done), landed, err
		}

		var failures []string
		acted := false

		// Twice, not once: a step refused by the window is worth re-asking with
		// the reasons in hand, and a third try has never turned one around.
		for attempt := 1; attempt <= 2; attempt++ {
			if attempt == 1 {
				e.logf("working out step %d", i)
			}
			answer, err := e.Steps.WriteStep(ctx, synth.StepRequest{
				Task:     task,
				Tree:     tree,
				Done:     done,
				Failures: failures,
			})
			if err != nil {
				return len(done), landed, err
			}

			if answer.Done {
				e.logf("finished after %d step(s): %s", len(done), answer.Why)
				return len(done), landed, nil
			}

			program := uiact.Program{App: tree.App, Steps: []uiact.Step{answer.Step}}
			if err := uiact.Validate(program, tree); err != nil {
				var invalid *uiact.InvalidError
				if !errors.As(err, &invalid) {
					return len(done), landed, err
				}
				failures = invalid.Reasons
				e.logf("step %d rejected: %s", i, strings.Join(invalid.Reasons, "; "))
				continue
			}

			describe := strings.TrimSpace(answer.Step.Describe)
			if describe == "" {
				describe = uiact.Preview(program)
			}
			e.logf("step %d: %s", i, describe)

			if err := handle(ctx, tree, program, i, describe); err != nil {
				var invalid *uiact.InvalidError
				if errors.As(err, &invalid) && attempt < 2 {
					failures = invalid.Reasons
					e.logf("step %d rejected: %s", i, strings.Join(invalid.Reasons, "; "))
					continue
				}
				return len(done), landed, err
			}

			effect, changed := describeEffect(ctx, e, app, tree)
			if !changed {
				landed = false
			}
			done = append(done, describe+" — "+effect)
			acted = true
			break
		}

		if !acted {
			return len(done), landed, fmt.Errorf(
				"engine: nothing that fits %s's window survived being checked against it, after %d step(s)",
				app, len(done))
		}
	}

	return len(done), landed, fmt.Errorf("engine: stopped after %d steps without finishing", limit)
}

func describeEffect(ctx context.Context, e *Engine, app string, before uiact.Tree) (string, bool) {
	after, err := e.UIScreen.Capture(ctx, app)
	if err != nil {
		return "could not read the window back", false
	}
	changes := uiact.Changes(before, after)
	if len(changes) == 0 {
		return "nothing in the window changed", false
	}
	return strings.Join(changes, "; "), true
}
