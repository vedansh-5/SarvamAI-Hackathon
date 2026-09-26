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

	// Shape and Literals are what the instruction generalises to and the values
	// taken out of it. Carried so a run that works can be kept under the same
	// key every other tier caches by, with this request's values lifted back out
	// of it (§9.8).
	Shape    string
	Literals map[string]string

	// Replay is the steps this task took last time, already filled with this
	// request's values. Empty means the model works them out as it goes.
	Replay []uiact.Step
}

func (e *Engine) canDrive() bool {
	return e.Steps != nil && e.UIScreen != nil
}

func (e *Engine) driveOn(ctx context.Context, app, task, shape string, literals map[string]string) (*Plan, error) {
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

	replay := e.stepsFromLastTime(ctx, tree.App, shape, literals)

	// A known sequence gets a confirmation that names the steps, because it can.
	// "Up to 12 steps" is the honest thing to say about a run nobody has planned
	// yet; saying it about a run whose steps are in hand would be withholding what
	// somebody is being asked to agree to.
	preview := fmt.Sprintf(
		"Work through this in %s, on screen, one step at a time:\n\n  %s\n\n"+
			"Up to %d steps. Every click and every keystroke happens where you can watch it.",
		tree.App, task, MaxDriveSteps)
	if len(replay) > 0 {
		preview = fmt.Sprintf(
			"Do this in %s, on screen, the way it worked last time:\n\n%s\n\n"+
				"Every step is checked against the window before it runs, and anything that "+
				"no longer fits is worked out again.",
			tree.App, uiact.Preview(uiact.Program{App: tree.App, Steps: replay}))
	}

	return &Plan{
		Task:        Task{Intent: task, Title: tree.App},
		Destination: tree.App,
		Drive: &DrivePlan{
			App:      tree.App,
			Task:     task,
			Preview:  preview,
			Shape:    shape,
			Literals: literals,
			Replay:   replay,
		},
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

	if len(d.Replay) > 0 {
		e.logf("%d step(s) kept from last time; the model is only asked if one stops fitting", len(d.Replay))
	}

	// Verified is evidence, not a claim. The model saying the task is done is
	// the thing being checked, so it cannot also be the check (D-004): what
	// counts is that the window moved for every step VANS performed. A replayed
	// step is held to exactly the same standard, so where the steps came from
	// never enters into whether the run is verified.
	run, err := e.stepThroughTask(ctx, d.App, d.Task, MaxDriveSteps, act, d.Replay)
	if err != nil {
		e.settleTheTrace(ctx, d, run)
		return Receipt{}, err
	}

	if run.Replayed > 0 {
		e.logf("%d of %d step(s) came from last time, and the model was asked %d time(s)",
			run.Replayed, run.Steps(), run.Calls)
	}

	// Before the empty-run check below, not after it. A trace that goes off on its
	// very first step leaves a run that did nothing, and that is precisely the
	// run whose trace most needs marking as having gone off — skipping it meant a
	// sequence that reliably breaks was never counted and so never forgotten.
	e.settleTheTrace(ctx, d, run)

	// A run that performed nothing has nothing to verify, and every step having
	// landed is trivially true of no steps. Left alone it reported success for
	// having done nothing at all — the same hole D-095 closed on the coach side.
	if run.Steps() == 0 {
		e.logf("nothing was done in %s: %s", d.App, reasonOr(run.Why, "the model gave no reason"))
		return Receipt{Title: d.App, Verified: false}, nil
	}

	return Receipt{Title: d.App, Verified: run.Landed}, nil
}

type stepHandler func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) error

// What a run through a task came to.
type taskRun struct {
	// Did is the steps carried out or pointed at, in order. Kept as the steps
	// themselves and not just a count because a run that worked is the best
	// description of how to do that task again (§9.8).
	Did []uiact.Step

	// Landed reports that every step was followed by a change in the window.
	Landed bool

	// Finished reports that the model said the task was done, as against the run
	// reaching the step limit. Only the first is evidence that the sequence is
	// the whole of the task.
	Finished bool

	// Why is the model's own sentence about stopping, which is the only thing
	// that can explain a run of no steps at all.
	Why string

	// Replayed is how many steps came off a stored trace, and Calls how many
	// times the model was asked. Together they are the whole claim of the cache,
	// counted rather than asserted.
	Replayed int
	Calls    int

	// Diverged reports that a replayed step stopped fitting the window and the
	// model had to take over.
	Diverged bool
}

// Steps is how many were carried out.
func (r taskRun) Steps() int { return len(r.Did) }

// modelAttempts is how many times the model is asked about one step.
//
// Twice, not once: a step refused by the window is worth re-asking with the
// reasons in hand, and a third try has never turned one around.
const modelAttempts = 2

// stepThroughTask works through a task one step at a time, taking each step from
// the trace where there is one and from the model where there is not.
//
// Divergence is handled inside this loop rather than by starting a second one.
// That is the point: a replayed step goes through the same validation, the same
// covered-window raise and the same read-back as a live one, and it cannot drift
// from them because there is only one of each.
func (e *Engine) stepThroughTask(ctx context.Context, app, task string, limit int, handle stepHandler, trace []uiact.Step) (taskRun, error) {
	var done []string
	var did []uiact.Step
	landed := true
	replayed, calls := 0, 0
	diverged := false

	// A trace is a suggestion. It is dropped the moment one of its steps stops
	// fitting the window, and the model takes over from that step onwards (§9.8).
	replaying := len(trace) > 0

	state := func() taskRun {
		return taskRun{
			Did: did, Landed: landed,
			Replayed: replayed, Calls: calls, Diverged: diverged,
		}
	}

	for i := 1; i <= limit; i++ {
		tree, err := e.windowToWorkFrom(ctx, app)
		if err != nil {
			return state(), err
		}

		var failures []string
		acted := false

		// One raise per step. Bringing a window forward is a change to the
		// world, so it is made deliberately and not in a loop (D-080a).
		raised := false

		// A replayed step costs no ask, so it is tried first and does not spend
		// an attempt. Only the asks are counted.
		asks := 0

		for asks <= modelAttempts {
			fromTrace := replaying && i <= len(trace)

			var answer synth.StepAnswer
			if fromTrace {
				answer = synth.StepAnswer{Step: trace[i-1]}
			} else if replaying && i > len(trace) {
				// The trace has run out, and a trace is only stored for a run the
				// model called finished. Its length is therefore the answer to
				// "is this done", and asking would spend the one call a warm run
				// is supposed to cost nothing for.
				e.logf("finished after %d step(s), all of them from the steps kept last time", len(done))
				return taskRun{
					Did: did, Landed: landed, Finished: true,
					Replayed: replayed, Calls: calls, Diverged: diverged,
					Why: "these are the steps that worked last time, and they have all been done",
				}, nil
			} else {
				asks++
				if asks == 1 {
					e.logf("working out step %d", i)
				}
				calls++
				answer, err = e.Steps.WriteStep(ctx, synth.StepRequest{
					Task:     task,
					Tree:     tree,
					Done:     done,
					Failures: failures,
				})
				if err != nil {
					// An answer with neither a step nor a reason is the model
					// dropping the contract for one turn, and it used to end the
					// whole run. It is worth exactly one more ask, which is what
					// the attempt budget is already for. A model that never
					// answered is not worth one: the re-ask waits out the whole
					// timeout again and has nothing new to offer.
					if asks < modelAttempts && !errors.Is(err, synth.ErrNoAnswer) {
						e.logf("step %d: nothing usable came back, asking again", i)
						continue
					}
					return state(), err
				}

				if answer.Done {
					e.logf("finished after %d step(s): %s", len(done), answer.Why)
					return taskRun{
						Did: did, Landed: landed, Finished: true, Why: answer.Why,
						Replayed: replayed, Calls: calls, Diverged: diverged,
					}, nil
				}
			}

			program := uiact.Program{App: tree.App, Steps: []uiact.Step{answer.Step}}
			if err := uiact.Validate(program, tree); err != nil {
				var invalid *uiact.InvalidError
				if !errors.As(err, &invalid) {
					return state(), err
				}
				failures = invalid.Reasons
				if fromTrace {
					replaying, diverged = false, true
					e.logf("step %d no longer fits %s, so the rest is worked out fresh: %s",
						i, app, strings.Join(invalid.Reasons, "; "))
					continue
				}
				e.logf("step %d rejected: %s", i, strings.Join(invalid.Reasons, "; "))
				continue
			}

			describe := strings.TrimSpace(answer.Step.Describe)
			if describe == "" {
				describe = uiact.Preview(program)
			}
			e.logf("step %d: %s", i, describe)

			err = handle(ctx, tree, program, i, describe)

			// Covered, not wrong. The model cannot write its way out of another
			// application being on top of the one it is working in, so the window
			// is brought forward and the same step tried again — no second ask,
			// because the answer would be the same and it was never the problem.
			// Once per step: a raise that did not take will not take on a third go.
			var invalid *uiact.InvalidError
			if errors.As(err, &invalid) && invalid.Covered && !raised {
				raised = true
				e.logf("step %d: %s", i, strings.Join(invalid.Reasons, "; "))

				if err := e.UIScreen.BringToFront(ctx, app); err != nil {
					return state(), err
				}
				if fresh, readErr := e.UIScreen.Capture(ctx, app); readErr == nil && len(fresh.Elements) > 0 {
					tree = fresh
				}
				err = handle(ctx, tree, program, i, describe)
			}

			if err != nil {
				if errors.As(err, &invalid) {
					if fromTrace {
						failures = invalid.Reasons
						replaying, diverged = false, true
						e.logf("step %d would not run against %s, so the rest is worked out fresh: %s",
							i, app, strings.Join(invalid.Reasons, "; "))
						continue
					}
					if asks < modelAttempts {
						failures = invalid.Reasons
						e.logf("step %d rejected: %s", i, strings.Join(invalid.Reasons, "; "))
						continue
					}
				}
				return state(), err
			}

			effect, changed := describeEffect(ctx, e, app, tree)

			// A replayed step that changed nothing is a divergence, not a success
			// — the same rule D-076 applies to a live one. Nothing moved, so
			// nothing has been spoiled by trying: the model is asked about this
			// same step instead.
			if !changed && fromTrace {
				replaying, diverged = false, true
				e.logf("step %d changed nothing in %s, so the rest is worked out fresh", i, app)
				continue
			}
			if !changed {
				landed = false
			}

			done = append(done, describe+" — "+effect)
			did = append(did, answer.Step)
			if fromTrace {
				replayed++
			}
			acted = true
			break
		}

		if !acted {
			return state(), stopped(app,
				"engine: nothing that fits %s's window survived being checked against it, after %d step(s)",
				app, len(done))
		}
	}

	return state(), fmt.Errorf(
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
