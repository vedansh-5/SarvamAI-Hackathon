package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

const MaxGuideSteps = 24

// Long enough to find a control and read a line about it, short enough that a
// person who walked away is not left with a dimmed screen. Polling is how we
// notice they acted, since nothing tells us.
var (
	guidePoll        = 500 * time.Millisecond
	guideStepTimeout = 45 * time.Second
)

// How long a window is given to finish moving on its own before anything it does
// is read as somebody using it.
//
// An application that has just been raised, or has just opened, keeps drawing for
// a moment: rows arrive, a pane lays itself out, a scroll bar appears. That looks
// exactly like the upheaval a click causes, and the coach was taking it as one —
// it drew "Click the Wallpaper row", the window finished settling, and it moved
// on to the next step with nobody having touched anything. The run then ended by
// telling the person it was all done.
//
// The fix is not to ignore the change but to fold it into the baseline. Whatever
// the window does by itself in this window of time is what it looked like to
// begin with, and the comparison starts from there.
var settleBeforeJudging = 900 * time.Millisecond

var (
	settlePoll = 150 * time.Millisecond
	quietFor   = 250 * time.Millisecond
)

// How many quiet spells one step is pointed at through before the coach accepts
// that nobody is coming back. Three is a couple of minutes: long enough to find
// a control, short enough not to leave a dimmed screen up for someone who walked
// away.
const timesToRepeatAStep = 3

// ErrNoControlsToPointAt is a coaching question asked with a browser in front.
//
// Chromium publishes a window with no children at all (§8), so there is nothing
// to point at and no amount of re-reading produces any. It has its own error
// because "that window has nothing on screen to point at" reads as a transient
// problem worth retrying, and this one never is — while what to do instead is
// specific and VANS is good at it: ask about the page and it works through the
// page directly.
var ErrNoControlsToPointAt = errors.New("engine: a browser publishes no controls to point at")

type GuidePlan struct {
	App     string
	Task    string
	Preview string

	// MustOpen reports that the application the task belongs to is not running.
	// The coach asks the person to open it and waits — VANS does not open
	// applications (D-032), and stopping to make them ask again would break the
	// one thing the coach is for, which is not having to work out the next move
	// yourself.
	MustOpen bool

	// Why is the model's line about what the application is for, said while they
	// open it.
	Why string

	Shape    string
	Literals map[string]string

	Web     bool
	Browser string
	Site    string

	Vision bool

	Looking string
}

type GuideStep struct {
	Number int
	Label  string
	X, Y   int
	W, H   int

	// Do is the verb — click, type or press — and Detail is what it needs: the
	// text to type, or the key combination to hold. A ring says which control;
	// these are what say what to do with it, which is the difference between a
	// highlight and an instruction.
	Do     string
	Detail string

	Ask   string
	Offer bool
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
	return e.planGuideFor(ctx, task, "", nil, lookingAt)
}

func (e *Engine) planGuideFor(ctx context.Context, task, shape string, literals map[string]string, lookingAt *screen.Context) (*Plan, error) {
	if !e.canGuide() {
		return nil, fmt.Errorf("engine: this build cannot show you what to do on screen")
	}

	// Where the task is done, which is usually not the window behind the panel.
	app, err := e.appForTask(ctx, task, lookingAt)
	return e.planGuideIn(ctx, task, shape, literals, app, err)
}

func (e *Engine) planGuideIn(ctx context.Context, task, shape string, literals map[string]string, app string, err error) (*Plan, error) {
	// Not running is not a refusal. It becomes the first step, carried out by
	// the person, and the coach picks up inside the application once it appears.
	var notOpen *AppNotOpenError
	if errors.As(err, &notOpen) {
		if e.canDrivePages() && strings.EqualFold(notOpen.App, e.pageBrowser(ctx)) {
			return e.planWebGuide(ctx, task, shape, literals, notOpen.App, true, notOpen.Why, "")
		}
		return &Plan{
			Task:        Task{Intent: task, Title: notOpen.App},
			Destination: notOpen.App,
			Guide: &GuidePlan{
				App:      notOpen.App,
				Task:     task,
				MustOpen: true,
				Why:      notOpen.Why,
				Shape:    shape,
				Literals: literals,
				Preview: fmt.Sprintf("Show you how to do this in %s, a step at a time — you will need to open it first:\n\n  %s",
					notOpen.App, task),
			},
		}, nil
	}
	if err != nil {
		return nil, err
	}

	// Decided before anything is read. Reaching for the accessibility tier here
	// costs a window raise and a tree walk to learn what the application's name
	// already says.
	if e.canDrivePages() && strings.EqualFold(app, e.pageBrowser(ctx)) {
		return e.planWebGuide(ctx, task, shape, literals, app, false, "", "")
	}
	if isBrowser(app) {
		return nil, fmt.Errorf("%w, so the steps cannot be drawn inside %s", ErrNoControlsToPointAt, app)
	}

	// Planning needs a window, so this is one of the two places allowed to go
	// and get one (D-080a). Reading, everywhere else, stays passive.
	tree, err := e.windowToWorkFrom(ctx, app)
	if err != nil {
		return nil, err
	}
	if e.Eyes != nil {
		if seen := uiact.Assess(tree); seen.Verdict == uiact.Blind {
			e.logf("%s cannot be read (%s), so its window will be looked at instead", app, seen.Why)
			return &Plan{
				Task:        Task{Intent: task, Title: app},
				Destination: app,
				Guide: &GuidePlan{
					App: app, Task: task, Shape: shape, Literals: literals, Vision: true,
					Preview: fmt.Sprintf("Show you how to do this in %s, a step at a time, by looking at its window:\n\n  %s\n\n"+
						"%s does not publish its controls, so each step takes a moment longer and the pointer is a best estimate.",
						app, task, app),
				},
			}, nil
		}
	}
	if len(tree.Elements) == 0 {
		return nil, stopped(app, "engine: %s has nothing on screen to point at", app)
	}

	return &Plan{
		Task:        Task{Intent: task, Title: tree.App},
		Destination: tree.App,
		Guide: &GuidePlan{
			App:      tree.App,
			Task:     task,
			Shape:    shape,
			Literals: literals,
			Preview: fmt.Sprintf("Show you how to do this in %s, a step at a time:\n\n  %s",
				tree.App, task),
		},
	}, nil
}

func (e *Engine) RunGuide(ctx context.Context, plan *Plan, show func(GuideStep), advance <-chan struct{}) (int, error) {
	work, err := e.RunGuideCounted(ctx, plan, show, advance)
	return work.Steps, err
}

func coachKey(shape string) string {
	if strings.TrimSpace(shape) == "" {
		return ""
	}
	return "coach: " + shape
}

func (e *Engine) RunGuideCounted(ctx context.Context, plan *Plan, show func(GuideStep), advance <-chan struct{}) (StepWork, error) {
	g := plan.Guide
	ctx = withLooking(ctx, g.Looking)
	limit := MaxGuideSteps
	if g.Web {
		if !e.canDrivePages() {
			return StepWork{}, fmt.Errorf("engine: this build cannot show you what to do on a page")
		}
		e = e.onThePage()
		limit = MaxWebSteps
	}
	if g.Vision {
		if e.Eyes == nil {
			return StepWork{}, fmt.Errorf("engine: this build cannot look at a window")
		}
		byEye := *e
		byEye.UIScreen = e.Eyes
		e = &byEye
	}
	if !e.canGuide() {
		return StepWork{}, fmt.Errorf("engine: this build cannot show you what to do on screen")
	}

	// Opening it is step one, and it is theirs to carry out. Nothing can be read
	// or pointed at until that is done, so this comes before everything.
	opened := 0
	if g.MustOpen {
		if err := e.waitForTheAppToOpen(ctx, g, show, advance); err != nil {
			return StepWork{}, err
		}
		opened = 1
	}

	// The panel held the keyboard while the question was typed, and the person is
	// about to act in their own application rather than in VANS. Handing it back
	// is also what keeps the window readable: macOS withdraws the windows of an
	// application that is not the active one (D-075).
	if err := e.UIScreen.BringToFront(ctx, g.App); err != nil {
		return StepWork{}, err
	}

	if g.Web {
		if g.Site != "" {
			went, err := e.waitForTheSite(ctx, g, opened+1, show, advance)
			if err != nil {
				return StepWork{}, err
			}
			if went {
				opened++
			}
		}
		tree, err := e.UIScreen.Capture(ctx, g.Browser)
		if err != nil {
			return StepWork{}, err
		}
		onPage := *g
		onPage.App = tree.App
		g = &onPage
	}

	changed := e.watchTheWindow(ctx, g.App)

	point := func(ctx context.Context, tree uiact.Tree, program uiact.Program, number int, describe string) (uiact.Tree, error) {
		step := program.Steps[0]
		if tree.OutOfView(step.Target) {
			scrolled, err := e.waitForTheScroll(ctx, g.App, tree, step, number+opened, show, advance, changed)
			if err != nil {
				return tree, err
			}
			tree = scrolled
		}
		x, y, w, h, why := tree.RectFor(step.Target)
		if why != "" {
			return tree, stopped(g.App, "engine: %s", why)
		}

		// A step nobody has done yet is still the step. Pointing at it again
		// costs nothing and asks the model for nothing; starting over from the
		// first step because someone was slow is the behaviour being avoided.
		offering := false
		if e.Reply != nil {
			if el, why := tree.Resolve(step.Target); why == "" {
				offering = uiact.EntersAValue(step, el)
			}
		}
		if offering {
			select {
			case <-e.Reply:
			default:
			}
		}

		for attempt := 1; ; attempt++ {
			show(GuideStep{
				Number: number + opened, Label: describe,
				X: x, Y: y, W: w, H: h,
				Do: step.Do, Detail: detailOf(step),
				Offer: offering,
			})

			var reply <-chan bool
			if offering {
				reply = e.Reply
			}

			// The hand is theirs. Nothing is clicked here — the next step is only
			// worked out once the window shows they have done this one.
			err := e.waitForTheUserToAct(ctx, g.App, tree, step, advance, changed, reply)
			if errors.Is(err, errOfferTaken) {
				e.logf("step %d: doing this one for you", number)
				if err := e.UIScreen.Act(ctx, program, tree); err != nil {
					return tree, err
				}
				e.waitForTheWindowToAnswer(ctx, g.App, tree, changed)
				return tree, nil
			}
			if errors.Is(err, errOfferDeclined) {
				e.logf("step %d: yours to do", number)
				offering = false
				attempt = 0
				continue
			}
			if !errors.Is(err, errStillWaiting) {
				return tree, err
			}
			if attempt >= timesToRepeatAStep {
				return tree, stopped(g.App, "engine: nothing happened in %s, so the steps stopped", g.App)
			}
			e.logf("still on step %d: %s", number, describe)
		}
	}
	// Planned once, up front. Without it the next step was only worked out after
	// the person had done the current one, so every step began with a wait for the
	// model — about two seconds of nothing, every time. With it the next step is
	// already known when they finish this one, and only has to be checked against
	// the window they left.
	memo := &DrivePlan{App: g.App, Task: g.Task, Shape: coachKey(g.Shape), Literals: g.Literals}
	planned := e.stepsFromLastTime(ctx, g.App, memo.Shape, g.Literals)
	memo.Kept = len(planned) > 0
	planCalls := 0
	switch {
	case memo.Kept:
		e.logf("%d step(s) kept from showing this last time; the model is only asked if one stops fitting", len(planned))
	case e.Plans != nil:
		if tree, err := e.windowToWorkFrom(ctx, g.App); err == nil && len(tree.Elements) > 0 {
			planned, planCalls = e.planAhead(ctx, synth.PlanRequest{Task: asTask(g.Task), Tree: tree, Most: limit})
		}
	}

	run, err := e.stepThroughTask(ctx, g.App, asTask(g.Task), limit, point, planned, memo.Kept, false, nil)
	e.settleTheTrace(context.WithoutCancel(ctx), memo, run)
	e.noteTheTask(ctx, g.App, g.Shape, ModeShow, run, planCalls, err)
	work := StepWork{Steps: run.Steps(), Replayed: run.Replayed, Calls: run.Calls + planCalls}
	if err != nil {
		return work, err
	}

	// Nothing was pointed at, and the panel would otherwise come back saying
	// "that is all of it" having shown nobody anything. The model's own sentence
	// is the only account of why — usually that the answer is in an application
	// other than the one being looked at.
	if run.Steps() == 0 {
		if why := strings.TrimSpace(run.Why); why != "" {
			return work, stopped(g.App, "engine: nothing in %s to point at — %s", g.App, why)
		}
		return work, stopped(g.App, "engine: there was nothing in %s to point at", g.App)
	}
	return work, nil
}

// How long a click gets to show up in the window before it is judged.
var afterAClick = 1500 * time.Millisecond

// waitForTheWindowToAnswer holds on after the person acts until the window has
// shown it.
//
// The shell reports a click the instant it happens (D-109), which is before the
// application has done anything about it. Reading the window at that moment
// found it unchanged, the step was written down as "clicked Displays — nothing in
// the window changed", and the model, told the click had done nothing, asked for
// it again. Every coached run showed its first step twice (measured 2026-09-29),
// and that repeat was most of the wait between steps.
//
// So it waits for the window to move, then for it to finish moving. A click that
// genuinely changes nothing runs out the wait and is recorded as such, which is
// the truth.
func (e *Engine) waitForTheWindowToAnswer(ctx context.Context, app string, before uiact.Tree, changed <-chan struct{}) {
	deadline := time.Now().Add(afterAClick)
	for time.Now().Before(deadline) {
		heard := false
		select {
		case <-ctx.Done():
			return
		case <-changed:
			heard = true
		case <-time.After(guidePoll / 5):
		}
		if heard {
			if settled := e.settledView(ctx, app, before, changed); len(uiact.Changes(before, settled)) > 0 {
				return
			}
			continue
		}
		after, err := e.UIScreen.Capture(ctx, app)
		if err != nil || len(after.Elements) == 0 {
			continue
		}
		if len(uiact.Changes(before, after)) > 0 {
			e.settledView(ctx, app, after, changed)
			return
		}
	}
}

func (e *Engine) watchTheWindow(ctx context.Context, app string) <-chan struct{} {
	w, ok := e.UIScreen.(uiact.WindowWatcher)
	if !ok {
		return nil
	}
	changed, err := w.Watch(ctx, app)
	if err != nil {
		e.logf("cannot hear %s change, so it is checked on a timer: %v", app, err)
		return nil
	}
	return changed
}

// How long someone gets to open an application before the coach accepts nobody
// is coming back. Longer than a step: finding an application in Spotlight or the
// dock, and waiting for it to launch, is slower than finding a button in a window
// that is already open.
var openAppTimeout = 90 * time.Second

// waitForTheAppToOpen shows the one step VANS will not carry out itself, and
// waits for the application to appear.
//
// Drawn with no rectangle, because there is nothing on screen to point at — the
// window being asked for does not exist yet. The overlay reads a zero-sized
// target as an instruction and puts it in the middle of the screen rather than
// ringing a dot in the corner.
//
// Readiness is the window publishing controls, not the process existing. An
// application that has launched but has not drawn anything cannot be read, and
// going on to plan a step against it would fail for a reason nobody could act on.
func (e *Engine) waitForTheAppToOpen(ctx context.Context, g *GuidePlan, show func(GuideStep), advance <-chan struct{}) error {
	label := "Open " + g.App

	if advance != nil {
		select {
		case <-advance:
		default:
		}
	}

	for attempt := 1; attempt <= timesToRepeatAStep; attempt++ {
		show(GuideStep{
			Number: 1,
			Label:  label,
			Do:     "open",
			Detail: strings.TrimSpace(g.Why),
		})

		deadline := time.Now().Add(openAppTimeout)
		for {
			// Next is a claim that it is open, and the window is asked before the
			// claim is believed. Taking it on trust carried a run on into an
			// application that was not running, and it stopped there with an error
			// about the application rather than the step telling them to open it
			// (seen 2026-09-29, when ⌥Space was pressed before opening it).
			pressedNext := false
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-advance:
				pressedNext = true
			case <-time.After(guidePoll):
			}

			if tree, err := e.UIScreen.Capture(ctx, g.App); err == nil && len(tree.Elements) > 0 {
				e.logf("%s is open; carrying on", g.App)
				return nil
			}
			if pressedNext {
				e.logf("%s is not open yet, so this step stays", g.App)
				show(GuideStep{
					Number: 1,
					Label:  label,
					Do:     "open",
					Detail: strings.TrimSpace(g.Why),
				})
			}
			if time.Now().After(deadline) {
				break
			}
		}
		e.logf("still waiting for %s to be opened", g.App)
	}

	return stopped(g.App, "engine: %s was never opened, so the steps stopped", g.App)
}

func (e *Engine) waitForTheScroll(ctx context.Context, app string, tree uiact.Tree, step uiact.Step, number int, show func(GuideStep), advance <-chan struct{}, changed <-chan struct{}) (uiact.Tree, error) {
	x, y, w, h, way, ok := tree.ScrollHint(step.Target)
	if !ok {
		return tree, stopped(app, "engine: the %s %q is out of view and there is nothing to scroll to reach it", step.Target.Role, step.Target.Title)
	}

	name := strings.TrimSpace(step.Target.Title)
	if name == "" {
		name = "it"
	}
	label := fmt.Sprintf("Scroll %s to find %s", way, name)
	e.logf("step %d is out of view; asking for a scroll %s", number, way)

	for attempt := 1; attempt <= timesToRepeatAStep; attempt++ {
		show(GuideStep{Number: number, Label: label, X: x, Y: y, W: w, H: h, Do: "scroll", Detail: way})

		deadline := time.Now().Add(guideStepTimeout)
		for time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return tree, ctx.Err()
			case <-advance:
			case <-changed:
			case <-time.After(guidePoll):
			}
			now, err := e.UIScreen.Capture(ctx, app)
			if err != nil || len(now.Elements) == 0 {
				continue
			}
			if !now.OutOfView(step.Target) {
				if _, _, _, _, why := now.RectFor(step.Target); why == "" {
					return e.settledView(ctx, app, now, changed), nil
				}
			}
		}
	}
	return tree, stopped(app, "engine: %s was never scrolled into view, so the steps stopped", name)
}

// What the verb needs spelled out: the text to type, or the keys to hold down.
func detailOf(step uiact.Step) string {
	switch {
	case strings.EqualFold(step.Do, "type"):
		return step.Text
	case strings.EqualFold(step.Do, "press"):
		return step.Chord()
	default:
		return ""
	}
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
			"They are performing every action themselves, and are learning as they go. "+
			"Give one small action per step: one click, one thing to type, or one key to press. Never fold two into one step, "+
			"and do not leave out a step because it seems obvious, such as clicking a field before typing in it or opening a menu before choosing from it. "+
			"Say what the step is for in a few words when it is not plain from the control's name. "+
			"Pointing at a control is the only way you can answer: describing where something is in words reaches nobody. "+
			"Never mark done on the first turn — the control has not been pointed at yet. "+
			"If they asked where something is, point at it, and mark done on the next turn, once it appears in what has been done so far. "+
			"If they asked how to accomplish something, mark done only when it is fully accomplished or nothing further is needed.",
		question)
}

// Nothing has happened yet, which is not a failure until it has gone on long
// enough that nobody is there.
var errStillWaiting = errors.New("engine: the window has not changed yet")

// settledView re-reads the window until it stops changing by itself, and returns
// what it finally looked like.
//
// The tree handed in was captured before the step was drawn, which is before the
// application had finished appearing. Comparing against it attributes the
// window's own arrival to the person.
func (e *Engine) settledView(ctx context.Context, app string, before uiact.Tree, changed <-chan struct{}) uiact.Tree {
	if settleBeforeJudging <= 0 {
		return before
	}

	start := time.Now()
	deadline := start.Add(settleBeforeJudging)
	lastMoved := start

	if changed != nil {
		for time.Since(lastMoved) < quietFor && time.Now().Before(deadline) {
			wait := min(quietFor-time.Since(lastMoved), time.Until(deadline))
			select {
			case <-ctx.Done():
				return before
			case <-changed:
				lastMoved = time.Now()
			case <-time.After(wait):
			}
		}
		if after, err := e.UIScreen.Capture(ctx, app); err == nil && len(after.Elements) > 0 {
			return after
		}
		return before
	}

	for {
		select {
		case <-ctx.Done():
			return before
		case <-time.After(settlePoll):
		}

		if after, err := e.UIScreen.Capture(ctx, app); err == nil && len(after.Elements) > 0 {
			if len(uiact.Changes(before, after)) > 0 {
				lastMoved = time.Now()
			}
			before = after
		}
		if time.Since(lastMoved) >= quietFor || time.Now().After(deadline) {
			return before
		}
	}
}

var (
	errOfferTaken    = errors.New("engine: asked to do this step for them")
	errOfferDeclined = errors.New("engine: they will do this step themselves")
)

func (e *Engine) waitForTheUserToAct(ctx context.Context, app string, before uiact.Tree, step uiact.Step, advance <-chan struct{}, changed <-chan struct{}, reply <-chan bool) error {
	// A Next left over from the step before belongs to that step. Cleared before
	// settling rather than after, so a click the person makes while the window is
	// still settling is kept rather than thrown away with the stale one.
	if advance != nil {
		select {
		case <-advance:
		default:
		}
	}

	// Anything the window does to itself in the first moment belongs to the
	// window, not to the person being coached.
	before = e.settledView(ctx, app, before, changed)

	deadline := time.Now().Add(guideStepTimeout)
	unreadable := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-advance:
			e.waitForTheWindowToAnswer(ctx, app, before, changed)
			return nil
		case yes := <-reply:
			if yes {
				return errOfferTaken
			}
			return errOfferDeclined
		case <-changed:
		case <-time.After(guidePoll):
		}

		after, err := e.UIScreen.Capture(ctx, app)
		if err != nil || len(after.Elements) == 0 {
			// Not a reason to stop. A window that publishes nothing is usually
			// the person having gone to another application for a moment, and
			// coaching is meant to survive that — the coach waits in the
			// background and picks up where it was. A window that is really
			// gone runs out the same patience as one nobody touches.
			if !unreadable {
				e.logf("%s is not showing a window; still waiting", app)
				unreadable = true
			}
			if time.Now().After(deadline) {
				return errStillWaiting
			}
			continue
		}
		unreadable = false

		// The coach's test, not the drive loop's: somebody else is acting, and
		// the shell reports their click directly (D-109).
		if uiact.PersonActedOn(before, after, step) {
			return nil
		}

		if time.Now().After(deadline) {
			return errStillWaiting
		}
	}
}
