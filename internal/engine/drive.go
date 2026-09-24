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
	run, err := e.stepThroughTask(ctx, d.App, d.Task, MaxDriveSteps, act)
	if err != nil {
		return Receipt{}, err
	}

	// A run that performed nothing has nothing to verify, and every step having
	// landed is trivially true of no steps. Left alone it reported success for
	// having done nothing at all — the same hole D-095 closed on the coach side.
	if run.Steps == 0 {
		e.logf("nothing was done in %s: %s", d.App, reasonOr(run.Why, "the model gave no reason"))
		return Receipt{Title: d.App, Verified: false}, nil
	}
	return Receipt{Title: d.App, Verified: run.Landed}, nil
}

type stepHandler func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) error

// What a run through a task came to.
type taskRun struct {
	// Steps is how many were carried out or pointed at.
	Steps int

	// Landed reports that every step was followed by a change in the window.
	Landed bool

	// Why is the model's own sentence about stopping, which is the only thing
	// that can explain a run of no steps at all.
	Why string
}

func (e *Engine) stepThroughTask(ctx context.Context, app, task string, limit int, handle stepHandler) (taskRun, error) {
	var done []string
	landed := true

	for i := 1; i <= limit; i++ {
		tree, err := e.windowToWorkFrom(ctx, app)
		if err != nil {
			return taskRun{Steps: len(done), Landed: landed}, err
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
				// An answer with neither a step nor a reason is the model
				// dropping the contract for one turn, and it used to end the
				// whole run. It is worth exactly one more ask, which is what the
				// attempt loop is already for. A model that never answered is
				// not worth one: the re-ask waits out the whole timeout again
				// and has nothing new to offer.
				if attempt < 2 && !errors.Is(err, synth.ErrNoAnswer) {
					e.logf("step %d: nothing usable came back, asking again", i)
					continue
				}
				return taskRun{Steps: len(done), Landed: landed}, err
			}

			if answer.Done {
				e.logf("finished after %d step(s): %s", len(done), answer.Why)
				return taskRun{Steps: len(done), Landed: landed, Why: answer.Why}, nil
			}

			program := uiact.Program{App: tree.App, Steps: []uiact.Step{answer.Step}}
			if err := uiact.Validate(program, tree); err != nil {
				var invalid *uiact.InvalidError
				if !errors.As(err, &invalid) {
					return taskRun{Steps: len(done), Landed: landed}, err
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
				return taskRun{Steps: len(done), Landed: landed}, err
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
			return taskRun{Steps: len(done), Landed: landed}, stopped(app,
				"engine: nothing that fits %s's window survived being checked against it, after %d step(s)",
				app, len(done))
		}
	}

	return taskRun{Steps: len(done), Landed: landed}, fmt.Errorf(
		"engine: stopped after %d steps without finishing", limit)
}

// The window a step is planned against, brought back if something withdrew it.
//
// Reading is passive, so the one place that insists on a window is the one about
// to plan a step against it. Stage Manager is why it can be missing: whatever the
// person did between steps may have made another application the active one, and
// then the window publishes nothing at all (D-075).
func (e *Engine) windowToWorkFrom(ctx context.Context, app string) (uiact.Tree, error) {
	tree, err := e.UIScreen.Capture(ctx, app)
	if !errors.Is(err, uiact.ErrNoWindow) {
		return tree, err
	}

	e.logf("%s is not showing a window; bringing it forward", app)
	if err := e.UIScreen.BringToFront(ctx, app); err != nil {
		return uiact.Tree{}, err
	}
	return e.UIScreen.Capture(ctx, app)
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
