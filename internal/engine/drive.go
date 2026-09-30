package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	// Replay is the steps to try before asking the model anything, already
	// filled with this request's values. Empty means every step is worked out as
	// it goes.
	Replay []uiact.Step

	// Kept reports that Replay came out of the store — steps that worked last
	// time and that the model called the whole task. Otherwise Replay is a plan
	// written for this run, which is a good guess and nothing more.
	Kept bool

	// PlanCalls is what writing that plan cost, so the count a run reports is
	// the whole of what the task cost and not just what happened after it
	// started.
	PlanCalls int
}

// PlanWriter asks for a whole task in one go. Optional: without one, every step
// is worked out as the run goes, which is how it always worked.
type PlanWriter interface {
	WritePlan(ctx context.Context, req synth.PlanRequest) (synth.PlanAnswer, error)
}

// planAhead asks for every step of a task at once, before any of them is done.
//
// One call instead of one per step. The steps after the first are the model's
// expectation of windows it has not seen yet, so they are checked against the
// real window as each one comes up, exactly as a remembered step is, and the run
// is worked out live from the first one that no longer fits (D-139). A plan that
// is right about the first few steps and wrong about the rest still saved every
// call up to where it went wrong.
//
// Never a reason to stop. No planner, a failed call or an empty plan all mean
// the run works itself out one step at a time, as it did before this existed.
func (e *Engine) planAhead(ctx context.Context, req synth.PlanRequest) ([]uiact.Step, int) {
	if e.Plans == nil {
		return nil, 0
	}

	if len(req.Done) > 0 {
		e.logf("planning the rest of it in one go")
	} else {
		e.logf("planning the whole of it in one go")
	}
	answer, err := e.Plans.WritePlan(ctx, req)
	if err != nil {
		e.logf("could not plan ahead, so working it out step by step: %v", err)
		return nil, 1
	}
	if len(answer.Steps) == 0 {
		e.logf("no plan came back (%s), so working it out step by step",
			reasonOr(answer.Why, "no reason given"))
		return nil, 1
	}

	e.logf("planned %d step(s)", len(answer.Steps))
	return answer.Steps, 1
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
	kept := len(replay) > 0
	planCalls := 0
	if !kept {
		replay, planCalls = e.planAhead(ctx, synth.PlanRequest{Task: task, Tree: tree, Most: MaxDriveSteps})
	}

	// A known sequence gets a confirmation that names the steps, because it can.
	// "Up to 12 steps" is the honest thing to say about a run nobody has planned
	// yet; saying it about a run whose steps are in hand would be withholding what
	// somebody is being asked to agree to.
	preview := fmt.Sprintf(
		"Work through this in %s, on screen, one step at a time:\n\n  %s\n\n"+
			"Up to %d steps. Every click and every keystroke happens where you can watch it.",
		tree.App, task, MaxDriveSteps)
	switch {
	case kept:
		preview = fmt.Sprintf(
			"Do this in %s, on screen, the way it worked last time:\n\n%s\n\n"+
				"Every step is checked against the window before it runs, and anything that "+
				"no longer fits is worked out again.",
			tree.App, uiact.Preview(uiact.Program{App: tree.App, Steps: replay}))
	case len(replay) > 0:
		preview = fmt.Sprintf(
			"Do this in %s, on screen:\n\n%s\n\n"+
				"This is the plan. Every step is checked against the window before it runs, "+
				"and anything that turns out differently is worked out again from there.",
			tree.App, uiact.Preview(uiact.Program{App: tree.App, Steps: replay}))
	}

	return &Plan{
		Task:        Task{Intent: task, Title: tree.App},
		Destination: tree.App,
		Drive: &DrivePlan{
			App:       tree.App,
			Task:      task,
			Preview:   preview,
			Shape:     shape,
			Literals:  literals,
			Replay:    replay,
			Kept:      kept,
			PlanCalls: planCalls,
		},
		SynthesisCalls: planCalls,
	}, nil
}

func (e *Engine) runDrive(ctx context.Context, plan *Plan) (Receipt, error) {
	if !e.canDrive() {
		return Receipt{}, fmt.Errorf("engine: this build cannot work through a task on screen")
	}

	d := plan.Drive
	hand := e.watchTheHand()
	act := func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) (uiact.Tree, error) {
		tree = e.bringIntoView(ctx, tree, program.Steps[0])
		if err := e.showBeforeActing(ctx, tree, program.Steps[0], number, describe); err != nil {
			return tree, err
		}
		if hand.moved() {
			fresh, err := e.waitOutThePause(ctx, d.App, tree, program, number, describe)
			hand.rest()
			if err != nil {
				return fresh, err
			}
			tree = fresh
		}
		err := e.UIScreen.Act(ctx, program, tree)
		hand.rest()
		return tree, err
	}

	switch {
	case d.Kept:
		e.logf("%d step(s) kept from last time; the model is only asked if one stops fitting", len(d.Replay))
	case len(d.Replay) > 0:
		e.logf("%d step(s) planned; the model is only asked again if one stops fitting", len(d.Replay))
	}

	// Verified is evidence, not a claim. The model saying the task is done is
	// the thing being checked, so it cannot also be the check (D-004): what
	// counts is that the window moved for every step VANS performed. A replayed
	// step is held to exactly the same standard, so where the steps came from
	// never enters into whether the run is verified.
	run, err := e.stepThroughTask(ctx, d.App, d.Task, MaxDriveSteps, act, d.Replay, d.Kept)
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
	work := &StepWork{Steps: run.Steps(), Replayed: run.Replayed, Calls: run.Calls + d.PlanCalls}

	if run.Steps() == 0 {
		e.logf("nothing was done in %s: %s", d.App, reasonOr(run.Why, "the model gave no reason"))
		return Receipt{Title: d.App, Verified: false, Work: work}, nil
	}

	return Receipt{Title: d.App, Verified: run.Landed, Work: work}, nil
}

// How long a step is shown before it is performed: long enough for the ghost to
// glide to the control and land, and a beat after that. It is also the window in
// which Escape stops the run with the step not done, so it is the dial between
// too quick to veto and slow (ghost cursor spec).
var lookBeforeActing = 850 * time.Millisecond

// showBeforeActing draws where a step is about to happen, then waits before it
// does. Showing the click before making it is what makes a run watchable rather
// than something that happened, and the pause is a veto that costs nothing
// unless it is used.
//
// A step with no rectangle to draw is performed without the pause: there is
// nothing to show, and waiting to show nothing is only slower.
func (e *Engine) showBeforeActing(ctx context.Context, tree uiact.Tree, step uiact.Step, number int, describe string) error {
	if e.ShowStep == nil {
		return nil
	}
	x, y, w, h, why := tree.RectFor(step.Target)
	if why != "" {
		return nil
	}

	e.ShowStep(GuideStep{
		Number: number,
		Label:  describe,
		X:      x, Y: y, W: w, H: h,
		Do:     strings.ToLower(strings.TrimSpace(step.Do)),
		Detail: detailOf(step),
	})

	if lookBeforeActing <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(lookBeforeActing):
		return nil
	}
}

var (
	pointerSlack = 4
	pausedFor    = 2 * time.Minute
)

type hand struct {
	read func() (int, int, bool)
	x, y int
	ok   bool
}

func (e *Engine) watchTheHand() *hand {
	p, ok := e.UIScreen.(uiact.PointerReader)
	if !ok || e.Paused == nil || e.Resume == nil {
		return &hand{}
	}
	h := &hand{read: p.PointerAt}
	h.rest()
	return h
}

func (h *hand) rest() {
	if h.read == nil {
		return
	}
	h.x, h.y, h.ok = h.read()
}

func (h *hand) moved() bool {
	if h.read == nil || !h.ok {
		return false
	}
	x, y, ok := h.read()
	if !ok {
		return false
	}
	return abs(x-h.x) > pointerSlack || abs(y-h.y) > pointerSlack
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (e *Engine) waitOutThePause(ctx context.Context, app string, tree uiact.Tree, program uiact.Program, number int, describe string) (uiact.Tree, error) {
	step := program.Steps[0]
	e.logf("the pointer moved, so step %d waits until you carry on", number)

drain:
	for {
		select {
		case <-e.Resume:
		default:
			break drain
		}
	}

	x, y, w, h, _ := tree.RectFor(step.Target)
	e.Paused(GuideStep{
		Number: number, Label: describe,
		X: x, Y: y, W: w, H: h,
		Do: strings.ToLower(strings.TrimSpace(step.Do)), Detail: detailOf(step),
	})

	select {
	case <-ctx.Done():
		return tree, ctx.Err()
	case <-time.After(pausedFor):
		return tree, stopped(app, "engine: paused in %s and nobody carried on, so it stopped", app)
	case <-e.Resume:
	}
	e.logf("carrying on from step %d", number)

	fresh, err := e.windowToWorkFrom(ctx, app)
	if err != nil {
		return tree, err
	}
	if len(fresh.Elements) == 0 {
		fresh = tree
	}
	if err := uiact.Validate(program, fresh); err != nil {
		return fresh, err
	}
	fresh = e.bringIntoView(ctx, fresh, step)
	if err := e.showBeforeActing(ctx, fresh, step, number, describe); err != nil {
		return fresh, err
	}
	return fresh, nil
}

type stepHandler func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) (uiact.Tree, error)

func (e *Engine) bringIntoView(ctx context.Context, tree uiact.Tree, step uiact.Step) uiact.Tree {
	if !tree.OutOfView(step.Target) {
		return tree
	}
	scroller, ok := e.UIScreen.(uiact.Scroller)
	if !ok {
		return tree
	}
	if err := scroller.ScrollIntoView(ctx, tree, step.Target); err != nil {
		e.logf("could not scroll %s into view: %v", step.Target.Title, err)
		return tree
	}
	fresh, err := e.UIScreen.Capture(ctx, tree.App)
	if err != nil || len(fresh.Elements) == 0 {
		return tree
	}
	e.logf("scrolled %s into view", step.Target.Title)
	return fresh
}

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

const mostReplans = 2

// stepThroughTask works through a task one step at a time, taking each step from
// the trace where there is one and from the model where there is not.
//
// Divergence is handled inside this loop rather than by starting a second one.
// That is the point: a replayed step goes through the same validation, the same
// covered-window raise and the same read-back as a live one, and it cannot drift
// from them because there is only one of each.
// confirmed says whether the model has already called the trace the whole task.
// A stored trace was only kept for a run it called finished, so running out of
// one is that answer. A plan written up front for this run promised nothing of
// the kind, so running out of it is not evidence of anything and the model is
// asked once whether there is more.
func (e *Engine) stepThroughTask(ctx context.Context, app, task string, limit int, handle stepHandler, trace []uiact.Step, confirmed bool) (taskRun, error) {
	var done []string
	var did []uiact.Step
	landed := true
	replayed, calls := 0, 0
	diverged := false

	// A trace is a suggestion. It is dropped the moment one of its steps stops
	// fitting the window, and the model takes over from that step onwards (§9.8).
	replaying := len(trace) > 0

	replans, replannedAt := 0, 0
	var next uiact.Tree

	state := func() taskRun {
		return taskRun{
			Did: did, Landed: landed,
			Replayed: replayed, Calls: calls, Diverged: diverged,
		}
	}

	for i := 1; i <= limit; i++ {
		tree, err := next, error(nil)
		next = uiact.Tree{}
		if len(tree.Elements) == 0 {
			tree, err = e.windowToWorkFrom(ctx, app)
		}
		if err != nil {
			return state(), err
		}

		replan := func(against uiact.Tree, why []string) bool {
			if e.Plans == nil || replans >= mostReplans || replannedAt == i || i > len(trace) {
				return false
			}
			replans++
			replannedAt = i
			steps, n := e.planAhead(ctx, synth.PlanRequest{
				Task: task, Tree: against, Most: limit - i + 1,
				Done: done, Failures: why,
			})
			calls += n
			if len(steps) == 0 {
				return false
			}
			trace = append(append([]uiact.Step{}, trace[:i-1]...), steps...)
			replaying, confirmed, diverged = true, false, true
			return true
		}

		var failures []string
		acted := false

		// One raise per step. Bringing a window forward is a change to the
		// world, so it is made deliberately and not in a loop (D-080a).
		raised := false

		// A replayed step costs no ask, so it is tried first and does not spend
		// an attempt. Only the asks are counted.
		asks := 0

		// One correction per step. A name that is on screen under a different
		// role is fixed here rather than asked about, and if the fixed step is
		// refused too then the window was never the problem.
		mended := false

		for asks <= modelAttempts {
			// A plan written this run has been carried out to the end. Whether
			// that finished the task is the model's to say, with the window it
			// left in view.
			if replaying && i > len(trace) && !confirmed {
				replaying = false
				e.logf("the %d planned step(s) are done; asking whether that is all of it", len(trace))
			}
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

			var refused *uiact.InvalidError
			if err := uiact.Validate(program, tree); err != nil {
				if !errors.As(err, &refused) {
					return state(), err
				}
			}

			// The window often already says what the step should have said: the
			// name is there, under a different role. Correcting it costs nothing,
			// where asking again costs a call and — seen twice, live — came back
			// with the same answer.
			if refused != nil && !mended {
				if fixed, what, ok := uiact.Repair(answer.Step, tree); ok {
					candidate := uiact.Program{App: tree.App, Steps: []uiact.Step{fixed}}
					if uiact.Validate(candidate, tree) == nil {
						e.logf("step %d: %s — corrected without asking", i, what)
						mended = true
						answer.Step, program, refused = fixed, candidate, nil

						// A corrected trace step means the stored sequence has
						// drifted, so what actually worked replaces it.
						if fromTrace {
							diverged = true
						}
					}
				}
			}

			if refused != nil {
				failures = refused.Reasons
				if fromTrace {
					e.logf("step %d no longer fits %s: %s", i, app, strings.Join(refused.Reasons, "; "))
					if replan(tree, refused.Reasons) {
						continue
					}
					replaying, diverged = false, true
					e.logf("so the rest is worked out a step at a time")
					continue
				}
				e.logf("step %d rejected: %s", i, strings.Join(refused.Reasons, "; "))
				continue
			}

			describe := strings.TrimSpace(answer.Step.Describe)
			if describe == "" {
				describe = uiact.Preview(program)
			}
			e.logf("step %d: %s", i, describe)

			startedFrom, err := handle(ctx, tree, program, i, describe)
			if len(startedFrom.Elements) > 0 {
				tree = startedFrom
			}

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
				startedFrom, err = handle(ctx, tree, program, i, describe)
				if len(startedFrom.Elements) > 0 {
					tree = startedFrom
				}
			}

			if err != nil {
				if errors.As(err, &invalid) {
					if fromTrace {
						failures = invalid.Reasons
						e.logf("step %d would not run against %s: %s", i, app, strings.Join(invalid.Reasons, "; "))
						if replan(tree, invalid.Reasons) {
							continue
						}
						replaying, diverged = false, true
						e.logf("so the rest is worked out a step at a time")
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

			effect, changed, after := describeEffect(ctx, e, app, tree)

			// A replayed step that changed nothing is a divergence, not a success
			// — the same rule D-076 applies to a live one. Nothing moved, so
			// nothing has been spoiled by trying: the model is asked about this
			// same step instead.
			if !changed && fromTrace {
				e.logf("step %d changed nothing in %s", i, app)
				if replan(after, []string{describe + " changed nothing on screen"}) {
					continue
				}
				replaying, diverged = false, true
				e.logf("so the rest is worked out a step at a time")
				continue
			}
			if !changed {
				landed = false
			}

			done = append(done, describe+" — "+effect)
			did = append(did, answer.Step)
			next = after
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

func describeEffect(ctx context.Context, e *Engine, app string, before uiact.Tree) (string, bool, uiact.Tree) {
	after, err := e.UIScreen.Capture(ctx, app)
	if err != nil {
		return "could not read the window back", false, uiact.Tree{}
	}
	changes := uiact.Changes(before, after)
	if len(changes) == 0 {
		return "nothing in the window changed", false, after
	}
	return strings.Join(changes, "; "), true, after
}
