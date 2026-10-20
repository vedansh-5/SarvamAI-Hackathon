package engine

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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

	Vision bool

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

	Whole bool

	Web     bool
	Browser string
	Open    string
	Profile string

	Done    []string
	Resumed bool

	Looking string

	typed     []TypedField
	committed bool

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
	answer, calls := e.planOn(ctx, req)
	return answer.Steps, calls
}

type walk struct {
	app, task string
	limit     int
	handle    stepHandler
	trace     []uiact.Step
	confirmed bool
	whole     bool
	acting    bool
	pointing  bool
	already   []string
	noted     string
}

func took(since time.Time) string {
	return time.Since(since).Round(10 * time.Millisecond).String()
}

func (e *Engine) planOn(ctx context.Context, req synth.PlanRequest) (synth.PlanAnswer, int) {
	if e.Plans == nil {
		return synth.PlanAnswer{}, 0
	}

	if len(req.Done) > 0 {
		e.logf("planning the rest of it in one go")
	} else {
		e.logf("planning the whole of it in one go")
	}
	if len(req.Open) == 0 {
		req.Open = e.openApps()
	}
	if len(req.Similar) == 0 && !req.Rehearse {
		req.Similar = e.similarRuns(ctx, req.Tree.App, askedIn(ctx, req.Task))
		if len(req.Similar) > 0 {
			e.logf("%d similar task(s) done in %s before go with the plan", len(req.Similar), req.Tree.App)
		}
	}
	req.Task = taskWithLooking(ctx, req.Task)
	if len(req.Known) == 0 {
		req.Known = e.knownAbout(ctx, req.Tree)
		if len(req.Known) > 0 {
			e.logf("%d thing(s) known about %s from before go with the plan", len(req.Known), req.Tree.App)
		}
	}
	asked := time.Now()
	answer, err := e.Plans.WritePlan(ctx, req)
	if err != nil {
		e.logf("could not plan ahead, so working it out step by step: %v", err)
		return synth.PlanAnswer{}, 1
	}
	if answer.Done {
		e.logf("nothing is left to do, answered in %s", took(asked))
		return answer, 1
	}
	if answer.Question != nil {
		return answer, 1
	}
	if answer.Stuck {
		e.logf("the planner cannot go further: %s", reasonOr(answer.Why, "no reason given"))
		return answer, 1
	}
	if len(answer.Steps) == 0 {
		e.logf("no plan came back in %s (%s), so working it out step by step",
			took(asked), reasonOr(answer.Why, "no reason given"))
		return synth.PlanAnswer{}, 1
	}

	e.logf("planned %d step(s) in %s", len(answer.Steps), took(asked))
	return answer, 1
}

func (e *Engine) canDrive() bool {
	return e.Steps != nil && e.UIScreen != nil
}

func (e *Engine) driveOn(ctx context.Context, app, task, shape string, literals map[string]string) (*Plan, error) {
	if !e.canDrive() {
		return nil, fmt.Errorf("engine: this build cannot work through a task on screen")
	}

	tree, err := e.windowToWorkFrom(ctx, app)
	if err != nil {
		return nil, err
	}
	vision := false
	if e.Eyes != nil {
		if seen := uiact.Assess(tree); seen.Verdict == uiact.Blind {
			e.logf("%s cannot be read (%s), so its window will be looked at instead", app, seen.Why)
			byEye := *e
			byEye.UIScreen = e.Eyes
			if looked, lookErr := byEye.UIScreen.Capture(ctx, tree.App); lookErr == nil {
				tree, vision, e = looked, true, &byEye
			}
		}
	}
	if len(tree.Elements) == 0 {
		return nil, fmt.Errorf("engine: %s has nothing on screen to work with", app)
	}

	replay := e.stepsFromLastTime(ctx, tree.App, shape, literals)
	kept := len(replay) > 0
	planCalls := 0
	whole := false
	if !kept {
		var planned synth.PlanAnswer
		planned, planCalls = e.planOn(ctx, synth.PlanRequest{Task: task, Tree: tree, Most: MaxDriveSteps})
		if planned.Stuck && len(planned.Steps) == 0 {
			return nil, &synth.UnwritableError{App: tree.App, Why: reasonOr(planned.Why, "nothing in "+tree.App+" matches what was asked")}
		}
		replay, whole = planned.Steps, planned.Whole
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

	if vision {
		preview += "\n\n" + tree.App + " does not publish its controls, so VANS reads the words on its window. " +
			"It clicks only what it can place exactly; anything it could only estimate, it points at and leaves to you."
	}

	return &Plan{
		Task:        Task{Intent: task, Title: tree.App},
		Destination: tree.App,
		Drive: &DrivePlan{
			Vision:    vision,
			App:       tree.App,
			Task:      task,
			Preview:   preview,
			Shape:     shape,
			Literals:  literals,
			Replay:    replay,
			Kept:      kept,
			Whole:     whole,
			PlanCalls: planCalls,
		},
		SynthesisCalls: planCalls,
	}, nil
}

func (e *Engine) runDrive(ctx context.Context, plan *Plan) (Receipt, error) {
	d := plan.Drive
	ctx = withLooking(ctx, d.Looking)
	limit := MaxDriveSteps
	if d.Web {
		if !e.canDrivePages() {
			return Receipt{}, fmt.Errorf("engine: this build cannot work through a task on a page")
		}
		e = e.onThePage()
		limit = MaxWebSteps
		if err := e.UIScreen.BringToFront(ctx, d.Browser); err != nil {
			return Receipt{}, err
		}
		if d.Open != "" {
			if err := e.openThePage(ctx, d); err != nil {
				return Receipt{}, err
			}
		}
	}
	if d.Vision && e.Eyes != nil {
		byEye := *e
		byEye.UIScreen = e.Eyes
		e = &byEye
	}
	if !e.canDrive() {
		return Receipt{}, fmt.Errorf("engine: this build cannot work through a task on screen")
	}
	act := func(ctx context.Context, on *Engine, tree uiact.Tree, program uiact.Program, number int, describe string) (uiact.Tree, error) {
		step := program.Steps[0]
		if step.Switches() {
			on.logf("bringing %s to the front", step.App)
			return tree, on.UIScreen.BringToFront(ctx, step.App)
		}
		if step.Waits() {
			return tree, on.showBeforeActing(ctx, tree, step, number, describe)
		}
		tree = on.bringIntoView(ctx, tree, step)
		if err := on.showBeforeActing(ctx, tree, step, number, describe); err != nil {
			return tree, err
		}
		if err := on.overTheLimit(ctx, tree, step); err != nil {
			return tree, err
		}
		ask := on.askFirst(tree, step)
		if ask == "" && on.Paused != nil && on.Resume != nil {
			ask = on.asksFirstAt(ctx, tree.App, d.Browser)
		}
		if ask == "" && on.Paused != nil && on.Resume != nil && checkpointEvery > 0 && number > 1 && (number-1)%checkpointEvery == 0 {
			ask = fmt.Sprintf("%s%d steps done so far", checkpointPrefix, number-1)
		}
		if ask != "" {
			fresh, err := on.waitOutThePause(ctx, tree.App, tree, program, number, describe, ask)
			if err != nil {
				return fresh, err
			}
			tree = fresh
		}
		return tree, on.UIScreen.Act(ctx, program, tree)
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
	if d.Resumed {
		e.logf("carrying on after %d step(s) already done", len(d.Done))
	}
	noticed := ""
	if d.Web && !d.Resumed {
		noticed, _ = e.closeTheNotice(ctx, d.App)
	}
	before := e.Meter.Now()
	run, err := e.stepThroughTask(ctx, walk{
		app: d.App, task: d.Task, limit: limit, handle: act,
		trace: d.Replay, confirmed: d.Kept, whole: d.Whole, acting: true, already: d.Done, noted: noticed,
	})
	used := e.noteTheTask(ctx, d.App, d.Shape, ModeDo, run, d.PlanCalls, err, before)
	d.typed, d.committed = append(d.typed, run.Typed...), d.committed || run.Committed
	if err != nil {
		e.settleTheTrace(ctx, d, run)
		if ctx.Err() == nil && len(run.Told) > 0 {
			d.Done, d.Resumed = run.Told, true
			d.Replay, d.Kept, d.Whole, d.Open, d.PlanCalls = nil, false, false, "", 0
			return Receipt{}, &BrokeOffError{App: d.App, Did: run.Told, Err: err}
		}
		return Receipt{}, err
	}

	if run.Replayed > 0 {
		from := "were planned ahead"
		if d.Kept {
			from = "came from last time"
		}
		e.logf("%d of %d step(s) %s, and the model was asked %d time(s)",
			run.Replayed, run.Steps(), from, run.Calls)
	}

	// Before the empty-run check below, not after it. A trace that goes off on its
	// very first step leaves a run that did nothing, and that is precisely the
	// run whose trace most needs marking as having gone off — skipping it meant a
	// sequence that reliably breaks was never counted and so never forgotten.
	e.settleTheTrace(ctx, d, run)

	// A run that performed nothing has nothing to verify, and every step having
	// landed is trivially true of no steps. Left alone it reported success for
	// having done nothing at all — the same hole D-095 closed on the coach side.
	work := &StepWork{
		Steps: run.Steps(), Replayed: run.Replayed, Calls: run.Calls + d.PlanCalls,
		TokensIn: used.In, TokensOut: used.Out,
	}

	if run.Steps() == 0 {
		e.logf("nothing was done in %s: %s", d.App, reasonOr(run.Why, "the model gave no reason"))
		return Receipt{Title: d.App, Verified: false, Work: work}, nil
	}

	return Receipt{Title: d.App, Verified: run.Landed, Work: work, Did: run.Told}, nil
}

// How long a step is shown before it is performed: long enough for the ghost to
// glide to the control and land, and a beat after that. It is also the window in
// which Escape stops the run with the step not done, so it is the dial between
// too quick to veto and slow (ghost cursor spec).
var lookBeforeActing = 600 * time.Millisecond

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

	e.ShowStep(clearOfTheMenu(GuideStep{
		Number: number,
		Label:  describe,
		X:      x, Y: y, W: w, H: h,
		Do:     strings.ToLower(strings.TrimSpace(step.Do)),
		Detail: detailOf(step),
	}, tree))

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

var pausedFor = 2 * time.Minute

var checkpointEvery = 8

const checkpointPrefix = "checkpoint: "

func (e *Engine) askFirst(tree uiact.Tree, step uiact.Step) string {
	if e.Paused == nil || e.Resume == nil {
		return ""
	}
	el, _ := tree.Resolve(step.Target)
	final, why := uiact.Commits(step, el)
	if !final {
		return ""
	}
	return why
}

func (e *Engine) waitOutThePause(ctx context.Context, app string, tree uiact.Tree, program uiact.Program, number int, describe, ask string) (uiact.Tree, error) {
	step := program.Steps[0]
	if strings.HasPrefix(ask, checkpointPrefix) {
		e.logf("%s; waiting to be told to carry on", strings.TrimPrefix(ask, checkpointPrefix))
	} else {
		e.logf("step %d cannot be taken back (%s), so it waits for you to say go ahead", number, ask)
	}

drain:
	for {
		select {
		case <-e.Resume:
		default:
			break drain
		}
	}

	x, y, w, h, _ := tree.RectFor(step.Target)
	e.Paused(clearOfTheMenu(GuideStep{
		Number: number, Label: describe,
		X: x, Y: y, W: w, H: h,
		Do: strings.ToLower(strings.TrimSpace(step.Do)), Detail: detailOf(step),
		Ask: ask,
	}, tree))

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

type BrokeOffError struct {
	App string
	Did []string
	Err error
}

func (e *BrokeOffError) Error() string {
	return fmt.Sprintf("engine: stopped in %s after %d step(s): %v", e.App, len(e.Did), e.Err)
}

func (e *BrokeOffError) Unwrap() error { return e.Err }

type stepHandler func(ctx context.Context, on *Engine, tree uiact.Tree, program uiact.Program, number int, describe string) (uiact.Tree, error)

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

	Asked int
	Told  []string

	Typed     []TypedField
	Committed bool
}

// Steps is how many were carried out.
func (r taskRun) Steps() int { return len(r.Did) }

// modelAttempts is how many times the model is asked about one step.
//
// Twice, not once: a step refused by the window is worth re-asking with the
// reasons in hand, and a third try has never turned one around.
const modelAttempts = 2

const mostReplans = 2

const mostQuestions = 4

func (e *Engine) askThePerson(ctx context.Context, app string, q *synth.Question, asked *int, recalled map[string]bool) (string, error) {
	var fresh []string
	for _, fact := range e.recall(ctx, q.Ask) {
		if !recalled[fact] {
			recalled[fact] = true
			fresh = append(fresh, fact)
		}
	}
	if len(fresh) > 0 {
		e.logf("not asking %q yet: something remembered may answer it", q.Ask)
		return fmt.Sprintf("Wanted to ask %q. %s Ask only if that does not answer it.", q.Ask, rememberedNote(fresh)), nil
	}
	told, err := e.reallyAsk(ctx, app, q, *asked)
	if err == nil {
		*asked++
	}
	return told, err
}

func (e *Engine) reallyAsk(ctx context.Context, app string, q *synth.Question, asked int) (string, error) {
	if e.Question == nil {
		return "", stopped(app, "engine: this needs an answer from you first: %s", q.Ask)
	}
	if asked >= mostQuestions {
		return "", stopped(app, "engine: stopped after %d questions without getting on with it", asked)
	}
	e.logf("asking: %s", q.Ask)
	answer, err := e.Question(ctx, q.Ask, q.Options)
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", stopped(app, "engine: the question went unanswered, so it stopped: %s", q.Ask)
	}
	e.logf("answered: %s", answer)
	return fmt.Sprintf("Asked the person %q \u2014 they answered %q", q.Ask, answer), nil
}

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
func (e *Engine) stepThroughTask(ctx context.Context, w walk) (taskRun, error) {
	app, task, limit, handle := w.app, w.task, w.limit, w.handle
	trace, confirmed, whole, acting, already := w.trace, w.confirmed, w.whole, w.acting, w.already
	lastCommitted := false
	done := append([]string{}, already...)
	if w.noted != "" {
		done = append(done, w.noted)
	}
	var did []uiact.Step
	landed := true
	replayed, calls := 0, 0
	diverged := false

	// A trace is a suggestion. It is dropped the moment one of its steps stops
	// fitting the window, and the model takes over from that step onwards (§9.8).
	replaying := len(trace) > 0 || (len(already) > 0 && e.Plans != nil)

	replans, replannedAt := 0, 0
	questions := 0
	recalled := map[string]bool{}
	var next uiact.Tree
	var soFar stepsSoFar
	filled := map[string]bool{}
	open := e.openApps()
	var typed typedSoFar
	committedSomething := false
	stalled, gotTo := 0, ""

	state := func() taskRun {
		return taskRun{
			Did: did, Landed: landed,
			Replayed: replayed, Calls: calls, Diverged: diverged, Asked: questions, Told: done,
			Typed: typed.fields, Committed: committedSomething,
		}
	}

	finished := func(why string) taskRun {
		run := state()
		run.Finished, run.Why = true, why
		return run
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
			again, n := e.planOn(ctx, synth.PlanRequest{
				Task: task, Tree: against.WithFilled(filled), Most: limit - i + 1,
				Done: done, Failures: why, Open: open,
			})
			calls += n
			if len(again.Steps) == 0 {
				return false
			}
			trace = append(append([]uiact.Step{}, trace[:i-1]...), again.Steps...)
			replaying, confirmed, diverged, whole = true, false, true, again.Whole
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
		looked := false
		circling := ""

		for asks <= modelAttempts {
			// A plan written this run has been carried out to the end. Whether
			// that finished the task is the model's to say, with the window it
			// left in view.
			if replaying && i > len(trace) && !confirmed && whole && len(did) > 0 && !diverged &&
				((acting && lastCommitted) || w.pointing) {
				why := "the plan was the whole task, and its last step went through"
				if w.pointing {
					why = "what was asked about has been pointed at"
				}
				e.logf("finished after %d step(s) without asking: %s", len(done), why)
				return finished(why), nil
			}
			if replaying && i > len(trace) && !confirmed {
				e.logf("the %d planned step(s) are done; asking what is left", len(trace))
				more, n := e.planOn(ctx, synth.PlanRequest{Task: task, Tree: tree.WithFilled(filled), Most: limit - i + 1, Done: done, Open: open})
				calls += n
				if more.Question != nil {
					told, err := e.askThePerson(ctx, app, more.Question, &questions, recalled)
					if err != nil {
						return state(), err
					}
					done = append(done, told)
					continue
				}
				if more.Stuck && len(done) > 0 {
					return state(), stopped(app, "engine: could not go any further in %s: %s", app,
						reasonOr(more.Why, "something the task needs is not on screen"))
				}
				switch {
				case more.Done:
					e.logf("finished after %d step(s): %s", len(done), more.Why)
					return finished(more.Why), nil
				case w.pointing && len(did) > 0 && reflect.DeepEqual(more.Steps[0].Target, did[len(did)-1].Target):
					e.logf("finished after %d step(s): asked where it is, and the next step would point at the same thing again", len(done))
					return finished("what was asked about has been pointed at"), nil
				case len(more.Steps) > 0:
					trace = append(append([]uiact.Step{}, trace[:i-1]...), more.Steps...)
					whole = more.Whole
				default:
					replaying = false
				}
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
				return finished("these are the steps that worked last time, and they have all been done"), nil
			} else {
				asks++
				if asks == 1 {
					e.logf("working out step %d", i)
				}
				calls++
				asked := time.Now()
				answer, err = e.Steps.WriteStep(ctx, synth.StepRequest{
					Task:     taskWithLooking(ctx, task),
					Tree:     tree.WithFilled(filled),
					Done:     done,
					Failures: failures,
					Open:     open,
				})
				e.logf("the model answered in %s", took(asked))
				if err != nil {
					// An answer with neither a step nor a reason is the model
					// dropping the contract for one turn, and it used to end the
					// whole run. It is worth exactly one more ask, which is what
					// the attempt budget is already for. A model that never
					// answered is not worth one: the re-ask waits out the whole
					// timeout again and has nothing new to offer.
					var empty *synth.NoStepError
					if asks < modelAttempts && !errors.Is(err, synth.ErrNoAnswer) {
						e.logf("step %d: nothing usable came back, asking again", i)
						if errors.As(err, &empty) {
							failures = []string{synth.NoStepNote}
						}
						continue
					}
					if errors.As(err, &empty) && len(done) > 0 {
						e.logf("step %d: %v", i, err)
						return state(), stopped(app, "engine: could not go any further in %s: %s", app,
							reasonOr(empty.Why, "it could not work out what to do next"))
					}
					return state(), err
				}

				if answer.Question != nil {
					told, err := e.askThePerson(ctx, app, answer.Question, &questions, recalled)
					if err != nil {
						return state(), err
					}
					asks--
					done = append(done, told)
					continue
				}

				if answer.Stuck && len(done) > 0 {
					return state(), stopped(app, "engine: could not go any further in %s: %s", app,
						reasonOr(answer.Why, "something the task needs is not on screen"))
				}
				if answer.Stuck {
					answer.Done = true
				}

				if answer.Done {
					e.logf("finished after %d step(s): %s", len(done), answer.Why)
					return finished(answer.Why), nil
				}
			}

			answer.Step = typedAsAsked(answer.Step, task)
			program := uiact.Program{App: tree.App, Steps: []uiact.Step{answer.Step}}

			check := uiact.Validate
			if !acting {
				check = uiact.ValidateToPoint
			}
			var refused *uiact.InvalidError
			if err := check(program, tree); err != nil {
				if !errors.As(err, &refused) {
					return state(), err
				}
			}
			if refused == nil && acting && answer.Step.Chooses() {
				path, why := fileToChoose(answer.Step.Text, taskWithLooking(ctx, task)+"\n"+strings.Join(done, "\n"))
				if why != "" {
					refused = &uiact.InvalidError{App: tree.App, Reasons: []string{why}}
				} else {
					answer.Step.Text = path
					program.Steps[0].Text = path
				}
			}
			if refused == nil && answer.Step.Switches() {
				name, why := e.whereToSwitch(ctx, answer.Step, app, acting)
				if why != "" {
					refused = &uiact.InvalidError{App: tree.App, Reasons: []string{why}}
				} else {
					answer.Step.App = name
					program.Steps[0].App = name
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

			if refused != nil && !looked {
				looked = true
				fresh, arrived := e.waitForItToArrive(ctx, app, answer.Step)
				if len(fresh.Elements) > 0 {
					tree = fresh
					program.App = fresh.App
				}
				if arrived {
					e.logf("step %d: what it needs had not appeared yet, and now has", i)
					refused = nil
				}
			}

			if refused == nil {
				about := strings.TrimSpace(answer.Step.Describe)
				if about == "" {
					about = uiact.Preview(program)
				}
				if why := soFar.goingInCircles(answer.Step, about); why != "" {
					circling = why
					soFar.warned++
					e.logf("step %d: %s", i, why)
					if soFar.warned >= mostWarnings {
						break
					}
					refused = &uiact.InvalidError{App: tree.App, Reasons: []string{why}}
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

			if answer.Step.Switches() {
				to := answer.Step.App
				if acting && !e.isOpen(to) {
					return state(), stopped(app,
						"engine: the next part is done in %s, which is not open. Open %s, then carry on", to, to)
				}
				there := e.screenFor(ctx, to)
				if _, err := handle(ctx, there, tree, program, i, describe); err != nil {
					return state(), err
				}
				arrived, err := there.windowToWorkFrom(ctx, to)
				if err != nil {
					return state(), err
				}
				e, app = there, to
				if strings.TrimSpace(arrived.App) != "" {
					app = arrived.App
				}
				done = append(done, fmt.Sprintf("%s \u2014 %s is now the application in front", describe, to))
				did = append(did, answer.Step)
				soFar.did(answer.Step, describe)
				lastCommitted = false
				next = arrived
				if fromTrace {
					replayed++
				}
				acted = true
				break
			}

			if answer.Step.Waits() {
				if _, err := handle(ctx, e, tree, program, i, describe); err != nil {
					return state(), err
				}
				arrived, what, err := e.waitFor(ctx, app, answer.Step)
				if err != nil {
					return state(), err
				}
				done = append(done, describe+" \u2014 "+what)
				did = append(did, answer.Step)
				soFar.did(answer.Step, describe)
				lastCommitted = false
				next = arrived
				if fromTrace {
					replayed++
				}
				acted = true
				break
			}

			if acting {
				typed.note(ctx, e, tree, answer.Step)
			}
			startedFrom, err := handle(ctx, e, tree, program, i, describe)
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
				startedFrom, err = handle(ctx, e, tree, program, i, describe)
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
			if !changed && acting && strings.EqualFold(strings.TrimSpace(answer.Step.Do), "click") && e.closedAStrayChooser(ctx) {
				effect, changed = strayChooser, true
			}
			if e.onPage && strings.TrimSpace(after.App) != "" && !strings.EqualFold(after.App, tree.App) {
				e.logf("the browser is now on %s", after.App)
				effect = fmt.Sprintf("the browser is now showing %s (%q), so what is on screen next is that page; %s", after.App, after.Window, effect)
				changed, app = true, after.App
			}

			// A replayed step that changed nothing is a divergence, not a success
			// — the same rule D-076 applies to a live one. Nothing moved, so
			// nothing has been spoiled by trying: the model is asked about this
			// same step instead.
			if !changed {
				soFar.changedNothing(answer.Step)
			}
			if !changed && fromTrace {
				e.logf("step %d changed nothing in %s", i, app)
				if replan(after, []string{describe + " changed nothing on screen"}) {
					continue
				}
				replaying, diverged = false, true
				e.logf("so the rest is worked out a step at a time")
				continue
			}
			if !changed && acting && strings.EqualFold(strings.TrimSpace(answer.Step.Do), "type") {
				return state(), stopped(app,
					"engine: the text was typed and nothing appeared in %s, so it stopped rather than carry on as if it had", app)
			}
			if !changed {
				landed = false
			}

			lastCommitted = false
			if el, why := tree.Resolve(answer.Step.Target); why == "" {
				if commits, _ := uiact.Commits(answer.Step, el); commits {
					committedSomething = true
					lastCommitted = changed
				}
			}
			done = append(done, describe+" — "+effect)
			did = append(did, answer.Step)
			soFar.did(answer.Step, describe)
			if changed {
				stalled, gotTo = 0, describe
			} else {
				stalled++
			}
			if stalled >= mostStalled {
				far := "nothing it did changed anything"
				if gotTo != "" {
					far = "the last step that changed something was: " + gotTo
				}
				return state(), stopped(app,
					"engine: the last %d steps in %s changed nothing on screen, so it stopped there; %s", stalled, app, far)
			}
			if verb := answer.Step.Verb(); verb == "type" || verb == "set" {
				filled[uiact.FieldKey(answer.Step.Target.Role, answer.Step.Target.Title)] = true
			}
			next = after
			if changed {
				e.learnFrom(ctx, tree, after, answer.Step)
			}
			if fromTrace {
				replayed++
			}
			acted = true
			break
		}

		if !acted && circling != "" {
			return state(), stopped(app,
				"engine: kept coming back to the same steps in %s without getting anywhere, so it stopped: %s",
				app, circling)
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
