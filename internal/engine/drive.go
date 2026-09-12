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
	return e.Steps != nil && e.UIRunner != nil
}

func (e *Engine) driveOn(ctx context.Context, app, task string) (*Plan, error) {
	if !e.canDrive() {
		return nil, fmt.Errorf("engine: this build cannot work through a task on screen")
	}

	tree, err := uiact.Capture(ctx, e.UIRunner, app)
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
	var done []string

	for i := 1; i <= MaxDriveSteps; i++ {
		tree, err := uiact.Capture(ctx, e.UIRunner, d.App)
		if err != nil {
			return Receipt{}, err
		}

		var failures []string
		acted := false

		for attempt := 1; attempt <= 2; attempt++ {
			answer, err := e.Steps.WriteStep(ctx, synth.StepRequest{
				Task:     d.Task,
				Tree:     tree,
				Done:     done,
				Failures: failures,
			})
			if err != nil {
				return Receipt{}, err
			}

			if answer.Done {
				e.logf("finished after %d step(s): %s", len(done), answer.Why)
				return Receipt{Title: d.App, Verified: false}, nil
			}

			program := uiact.Program{App: tree.App, Steps: []uiact.Step{answer.Step}}
			if err := uiact.Validate(program, tree); err != nil {
				var invalid *uiact.InvalidError
				if !errors.As(err, &invalid) {
					return Receipt{}, err
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

			if err := uiact.RunScreen(ctx, e.UIRunner, program, tree); err != nil {
				return Receipt{}, err
			}

			done = append(done, describe)
			acted = true
			break
		}

		if !acted {
			return Receipt{}, fmt.Errorf(
				"engine: nothing that fits %s's window survived being checked against it, after %d step(s)",
				d.App, len(done))
		}
	}

	return Receipt{}, fmt.Errorf("engine: stopped after %d steps without finishing", MaxDriveSteps)
}
