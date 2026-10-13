package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func quickGuide(t *testing.T) {
	t.Helper()
	poll, timeout, settle, click, back, away := guidePoll, guideStepTimeout, settleBeforeJudging, afterAClick, handBackWait, awayAtMost
	guidePoll, guideStepTimeout, settleBeforeJudging, afterAClick, handBackWait, awayAtMost = time.Millisecond, 200*time.Millisecond, 0, 20*time.Millisecond, 0, 200*time.Millisecond
	t.Cleanup(func() {
		guidePoll, guideStepTimeout, settleBeforeJudging, afterAClick, handBackWait, awayAtMost = poll, timeout, settle, click, back, away
	})
}

func quickOpen(t *testing.T) {
	t.Helper()
	wait := openAppTimeout
	openAppTimeout = 150 * time.Millisecond
	t.Cleanup(func() { openAppTimeout = wait })
}

func guidePlan() *Plan {
	return &Plan{Guide: &GuidePlan{App: "Editor", Task: "say hello"}}
}

func TestGuidePointsAtTheControlAndNeverTouchesIt(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
	}, nil); err != nil {
		t.Fatalf("runGuide: %v", err)
	}

	if len(shown) != 1 {
		t.Fatalf("pointed %d time(s), want 1: %+v", len(shown), shown)
	}
	got := shown[0]
	if got.X != 100 || got.Y != 200 || got.W != 80 || got.H != 40 {
		t.Errorf("pointed at (%d,%d %dx%d), want the control's rectangle", got.X, got.Y, got.W, got.H)
	}
	if !strings.Contains(got.Label, "Message") {
		t.Errorf("label = %q, want what a person would be told", got.Label)
	}
	if screen.acts != 0 {
		t.Errorf("acted %d time(s) — showing is not doing", screen.acts)
	}
}

// The next step is worked out from the window after the person has acted, so
// the loop has to wait for them rather than run ahead.
func TestGuideWaitsForTheWindowToChange(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 3}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	if _, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil); err != nil {
		t.Fatalf("runGuide: %v", err)
	}
	if screen.captures < 4 {
		t.Errorf("read the window %d time(s); it cannot have waited", screen.captures)
	}
}

func TestGuideStopsWhenNothingHappens(t *testing.T) {
	quickGuide(t)

	screen := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: screen, Steps: &alwaysStep{step: typeIntoMessage()}}

	_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil)
	if err == nil {
		t.Fatal("waited forever on a window nobody touched")
	}
	if !strings.Contains(err.Error(), "nothing happened") {
		t.Errorf("err = %v", err)
	}
}

func TestGuideRefusesAControlItCannotPointAt(t *testing.T) {
	quickGuide(t)

	hidden := editorTree()
	hidden.Elements[0].Position = nil
	hidden.Elements[0].Size = nil

	e := &Engine{UIScreen: &fakeScreen{tree: hidden}, Steps: &alwaysStep{step: typeIntoMessage()}}

	if _, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil); err == nil {
		t.Fatal("pointed at a control with no place on screen")
	}
}

func TestGuideAdvancesOnChannel(t *testing.T) {
	quickGuide(t)

	screen := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	advance := make(chan struct{}, 1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		advance <- struct{}{}
	}()

	var shown []GuideStep
	steps, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
	}, advance)
	if err != nil {
		t.Fatalf("runGuide with advance: %v", err)
	}
	if steps != 1 || len(shown) != 1 {
		t.Errorf("steps = %d, shown = %d; want 1", steps, len(shown))
	}
}

func TestGuideStepsThroughMultipleControls(t *testing.T) {
	quickGuide(t)

	step1Tree := editorTree()
	step2Tree := editorTree()
	step2Tree.Elements[0].Value = "hello"
	// A pane that answered a click, not a single control ticking over: one
	// appearance on its own is a clock or a refreshing list, and deliberately
	// does not move the coach on.
	step2Tree.Elements = append(step2Tree.Elements, uiact.Element{
		Role: "AXButton", Title: "Send", Enabled: true,
		Position: []int{200, 200}, Size: []int{60, 30}, Path: []int{0, 2},
	}, uiact.Element{
		Role: "AXStaticText", Title: "Draft saved", Enabled: true,
		Position: []int{200, 240}, Size: []int{90, 16}, Path: []int{0, 3},
	})

	screen := &fakeScreen{tree: step1Tree, afterwards: &step2Tree, changesAfter: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Step: uiact.Step{Do: "click", Target: uiact.Selector{Role: "AXButton", Title: "Send"}}},
			{Done: true, Why: "finished"},
		},
	}}

	advance := make(chan struct{}, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		advance <- struct{}{}
	}()

	var shown []GuideStep
	steps, err := e.RunGuide(context.Background(), guidePlan(), func(s GuideStep) {
		shown = append(shown, s)
	}, advance)
	if err != nil {
		t.Fatalf("runGuide: %v", err)
	}

	if len(shown) != 2 {
		t.Fatalf("shown %d steps, want 2", len(shown))
	}
	if shown[0].Number != 1 || shown[1].Number != 2 {
		t.Errorf("step numbers: %d, %d; want 1, 2", shown[0].Number, shown[1].Number)
	}
	if steps != 2 {
		t.Errorf("steps = %d, want 2", steps)
	}
}

type failureScreen struct {
	initial    uiact.Tree
	failCount  int
	maxFails   int
	afterwards uiact.Tree
}

func (f *failureScreen) Capture(context.Context, string) (uiact.Tree, error) {
	if f.failCount == 0 {
		f.failCount++
		return f.initial, nil
	}
	if f.failCount <= f.maxFails {
		f.failCount++
		return uiact.Tree{}, nil
	}
	return f.afterwards, nil
}

func (f *failureScreen) Act(context.Context, uiact.Program, uiact.Tree) error { return nil }
func (f *failureScreen) Frontmost(context.Context) (string, error)            { return "Editor", nil }
func (f *failureScreen) BringToFront(context.Context, string) error           { return nil }
func (f *failureScreen) Type(context.Context, string) error                   { return nil }

func TestGuideToleratesTransientCaptureFailures(t *testing.T) {
	quickGuide(t)

	initial := editorTree()
	changed := editorTree()
	changed.Elements[0].Value = "hello"

	screen := &failureScreen{
		initial:    initial,
		maxFails:   2,
		afterwards: changed,
	}
	e := &Engine{
		UIScreen: screen,
		Steps: &scriptedSteps{
			answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
		},
	}

	_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil)
	if err != nil {
		t.Fatalf("expected guide to tolerate 2 transient failures, got: %v", err)
	}
}

// A window that stops publishing is usually the person stepping away to another
// application, and coaching waits for them in the background. It runs out the
// same patience as a window nobody touches, rather than ending the moment they
// look somewhere else.
func TestGuideWaitsOutAWindowThatStopsPublishing(t *testing.T) {
	quickGuide(t)

	screen := &failureScreen{
		initial:  editorTree(),
		maxFails: 10000,
	}
	e := &Engine{
		UIScreen: screen,
		Steps: &scriptedSteps{
			answers: []synth.StepAnswer{{Step: typeIntoMessage()}},
		},
	}

	_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil)
	if err == nil {
		t.Fatal("waited forever on a window that never came back")
	}
	if !strings.Contains(err.Error(), "nothing happened") {
		t.Errorf("err = %v, want the same answer as a window nobody touched", err)
	}
}

func TestPlanGuideRefusesAWindowWithNothingInIt(t *testing.T) {
	e := &Engine{
		UIScreen: &fakeScreen{tree: uiact.Tree{App: "Editor", Window: "Editor"}},
		Steps:    &alwaysStep{step: typeIntoMessage()},
	}

	if _, err := e.planGuide(context.Background(), "say hello", nil); err == nil {
		t.Fatal("offered to point at a window with nothing in it")
	}
}

// Chromium hands back a window with no children, so the old path raised Chrome,
// walked an empty tree, and reported that the window had nothing in it — which
// reads as something that might work next time. The application's own name is
// enough to know better.
func TestPlanGuideRefusesABrowserWithoutReadingIt(t *testing.T) {
	screenReader := &fakeScreen{tree: uiact.Tree{App: "Google Chrome", Window: "Google Chrome"}}
	e := &Engine{UIScreen: screenReader, Steps: &alwaysStep{step: typeIntoMessage()}}

	_, err := e.planGuide(context.Background(), "how do I sign in", &screen.Context{AppName: "Google Chrome"})
	if err == nil {
		t.Fatal("offered to draw steps inside a browser")
	}
	if !errors.Is(err, ErrNoControlsToPointAt) {
		t.Errorf("err = %v, want it to carry ErrNoControlsToPointAt so the panel can say what to do instead", err)
	}
	if len(screenReader.raised) != 0 || screenReader.captures != 0 {
		t.Errorf("raised %v and read the window %d time(s) to learn what the name already said",
			screenReader.raised, screenReader.captures)
	}
}

func TestAppInQuestionPrefersContextAndRefusesVANS(t *testing.T) {
	e := &Engine{
		UIScreen: &fakeScreen{front: "FallbackApp"},
	}

	got, err := e.appInQuestion(context.Background(), &screen.Context{AppName: "Notes"})
	if err != nil || got != "Notes" {
		t.Errorf("appInQuestion with Notes = %q, %v; want Notes", got, err)
	}

	got, err = e.appInQuestion(context.Background(), &screen.Context{AppName: "VANS"})
	if err != nil || got != "FallbackApp" {
		t.Errorf("appInQuestion with VANS = %q, %v; want FallbackApp", got, err)
	}

	got, err = e.appInQuestion(context.Background(), nil)
	if err != nil || got != "FallbackApp" {
		t.Errorf("appInQuestion with nil = %q, %v; want FallbackApp", got, err)
	}
}

// A Next pressed while the previous step was still being worked out must not
// carry over. Draining once per step is the whole of that, and it is the
// easiest line in the loop to lose.
func TestGuideIgnoresAnAdvanceFromBeforeTheStep(t *testing.T) {
	quickGuide(t)

	screen := &fakeScreen{tree: editorTree()}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true}},
	}}

	advance := make(chan struct{}, 1)
	advance <- struct{}{}

	_, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, advance)
	if err == nil {
		t.Fatal("a stale advance carried into the step and skipped the wait")
	}
	if !strings.Contains(err.Error(), "nothing happened") {
		t.Errorf("err = %v, want the step to have waited and timed out", err)
	}
}

// The coach answers by pointing. A prompt that lets the model answer in words
// instead produces a guide that finishes with no steps and no error, which is
// silent and took a live run to notice — so the clauses that prevent it are
// pinned here.
func TestTheCoachIsToldToPointRatherThanDescribe(t *testing.T) {
	task := asTask("where is the search bar")

	for _, must := range []string{
		"Pointing at a control is the only way you can answer",
		"Never mark done on the first turn",
	} {
		if !strings.Contains(task, must) {
			t.Errorf("the task no longer says %q:\n%s", must, task)
		}
	}
}

// A step nobody has got to yet is still that step. The coach points at it again,
// with no new model call, rather than starting the task over from step one.
func TestGuideStaysOnTheStepNobodyHasDoneYet(t *testing.T) {
	poll, timeout, settle := guidePoll, guideStepTimeout, settleBeforeJudging
	guidePoll, guideStepTimeout, settleBeforeJudging = 5*time.Millisecond, 40*time.Millisecond, 0
	t.Cleanup(func() {
		guidePoll, guideStepTimeout, settleBeforeJudging = poll, timeout, settle
	})

	acted := editorTree()
	acted.Elements[0].Value = "hello"

	// More captures than one quiet spell can hold, so the window moves only after
	// the step has been pointed at again.
	screen := &fakeScreen{tree: editorTree(), afterwards: &acted, changesAfter: 12}
	writer := &scriptedSteps{answers: []synth.StepAnswer{{Step: typeIntoMessage()}}}
	e := &Engine{UIScreen: screen, Steps: writer}

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(g GuideStep) {
		shown = append(shown, g)
	}, nil)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	if len(shown) < 2 {
		t.Fatalf("pointed %d time(s); an unfinished step has to be pointed at again", len(shown))
	}
	for i, g := range shown {
		if g.Number != 1 {
			t.Fatalf("pointed at step %d on showing %d; the step number must not move on", g.Number, i+1)
		}
	}
	// One step was written, however many times it had to be pointed at.
	if writer.calls != 1 {
		t.Errorf("model wrote %d step(s), want 1; repeating a step costs no call", writer.calls)
	}
}

func TestTheGuideDoesNotBringTheApplicationForward(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	if _, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(screen.raised) != 0 {
		t.Errorf("brought %v to the front; switching applications is the person's step", screen.raised)
	}
}

type elsewhereThenBack struct {
	fakeScreen
	asked int
}

func (s *elsewhereThenBack) Frontmost(context.Context) (string, error) {
	s.asked++
	if s.asked <= 3 {
		return "Notes", nil
	}
	return "Editor", nil
}

func TestTheGuideAsksForTheSwitchWhenAnotherApplicationIsInFront(t *testing.T) {
	quickGuide(t)
	quickOpen(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &elsewhereThenBack{fakeScreen: fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 3}}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(g GuideStep) { shown = append(shown, g) }, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) < 2 {
		t.Fatalf("shown %d step(s), want the switch and then the control", len(shown))
	}
	if shown[0].Number != 1 || shown[0].Label != "Switch to Editor" || shown[0].W != 0 {
		t.Errorf("first = %+v, want an instruction to switch to Editor", shown[0])
	}
	if shown[1].Number != 2 {
		t.Errorf("second is numbered %d, want 2: the switch was step one", shown[1].Number)
	}
	if len(screen.raised) != 0 {
		t.Errorf("brought %v to the front", screen.raised)
	}
}

// A ring says which control. What to do with it is a different question, and the
// step already knows the answer — it has to reach the overlay to be drawn.
func TestGuideSaysWhatToDoWithTheControl(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(g GuideStep) {
		shown = append(shown, g)
	}, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}

	if len(shown) == 0 {
		t.Fatal("nothing was pointed at")
	}
	if shown[0].Do != "type" {
		t.Errorf("Do = %q, want the verb the step asked for", shown[0].Do)
	}
	if shown[0].Detail != "hello" {
		t.Errorf("Detail = %q, want the text to be typed", shown[0].Detail)
	}
}

func TestGuideCarriesTheKeysToHoldDown(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "x"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: uiact.Step{
			Do:        "press",
			Target:    uiact.Selector{Role: "AXTextArea", Title: "Message"},
			Key:       "Enter",
			Modifiers: []string{"Command"},
			Describe:  "Send it",
		}}, {Done: true, Why: "sent"}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(g GuideStep) {
		shown = append(shown, g)
	}, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) == 0 || shown[0].Detail != "Command-Enter" {
		t.Errorf("Detail = %q, want the combination spelled out", shown[0].Detail)
	}
}

// closedThenOpen publishes nothing until it has been asked a few times, the way
// an application does between being asked for and having drawn a window.
type closedThenOpen struct {
	fakeScreen
	opensAfter int
	asked      int
}

// Its own counter, so refusing to publish a window does not disturb the
// bookkeeping the embedded fake uses to decide when the window has moved on.
func (c *closedThenOpen) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	c.asked++
	if c.asked <= c.opensAfter {
		return uiact.Tree{}, uiact.ErrNoWindow
	}
	return c.fakeScreen.Capture(ctx, app)
}

// The task belongs somewhere that is not running. That is not a refusal — it is
// the first step, and the person carries it out.
func TestPlanGuideAsksForTheAppToBeOpened(t *testing.T) {
	chooser := &saysApp{choice: synth.AppChoice{App: "System Settings", Why: "appearance lives there"}}
	e := &Engine{
		Apps:     capability.New(),
		Chooser:  chooser,
		Steps:    &alwaysStep{step: typeIntoMessage()},
		UIScreen: &runningScreen{fakeScreen: fakeScreen{front: "Antigravity IDE"}, open: []string{"Antigravity IDE"}},
		Log:      func(string, ...any) {},
	}

	plan, err := e.planGuide(context.Background(), "how do I change the wallpaper",
		&screen.Context{AppName: "Antigravity IDE"})
	if err != nil {
		t.Fatalf("planGuide: %v", err)
	}
	if plan.Guide == nil || !plan.Guide.MustOpen {
		t.Fatalf("plan does not ask for the application to be opened: %+v", plan.Guide)
	}
	if plan.Guide.App != "System Settings" {
		t.Errorf("App = %q, want System Settings", plan.Guide.App)
	}
	if !strings.Contains(plan.Guide.Preview, "open it first") {
		t.Errorf("the preview does not say it has to be opened: %q", plan.Guide.Preview)
	}
}

// The step drawn while waiting has no control, because the window being asked
// for does not exist yet.
func TestGuideShowsOpenItWithNothingToPointAt(t *testing.T) {
	quickGuide(t)
	quickOpen(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screenReader := &closedThenOpen{
		fakeScreen: fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1},
		opensAfter: 2,
	}
	e := &Engine{UIScreen: screenReader, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	var shown []GuideStep
	plan := &Plan{Guide: &GuidePlan{
		App: "Editor", Task: "say hello", MustOpen: true, Why: "the editor is where text goes",
	}}

	if _, err := e.RunGuide(context.Background(), plan, func(g GuideStep) { shown = append(shown, g) }, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) == 0 {
		t.Fatal("nothing was shown")
	}

	first := shown[0]
	if first.Do != "open" {
		t.Errorf("first step Do = %q, want open", first.Do)
	}
	if first.Label != "Open Editor" {
		t.Errorf("first step Label = %q, want \"Open Editor\"", first.Label)
	}
	if first.W != 0 || first.H != 0 {
		t.Errorf("the open step carries a control to ring (%dx%d); there is nothing on screen yet", first.W, first.H)
	}
	if first.Detail == "" {
		t.Error("nothing said about why they are being sent there")
	}
	if first.Number != 1 {
		t.Errorf("open step Number = %d, want 1", first.Number)
	}

	// Opening it was step one, so pointing at the first control is step two.
	if len(shown) < 2 {
		t.Fatal("the coach never got past opening the application")
	}
	if shown[1].Number != 2 {
		t.Errorf("the step after opening is numbered %d, want 2", shown[1].Number)
	}
	if shown[1].W == 0 {
		t.Error("the step after opening has no control, so nothing was pointed at")
	}
}

// Nobody opened it. Saying so beats waiting forever with a dimmed screen.
func TestGuideGivesUpWhenTheAppIsNeverOpened(t *testing.T) {
	quickGuide(t)
	quickOpen(t)

	screenReader := &closedThenOpen{
		fakeScreen: fakeScreen{tree: editorTree()},
		opensAfter: 1 << 30,
	}
	e := &Engine{UIScreen: screenReader, Steps: &alwaysStep{step: typeIntoMessage()}}

	plan := &Plan{Guide: &GuidePlan{App: "Editor", Task: "say hello", MustOpen: true}}

	_, err := e.RunGuide(context.Background(), plan, func(GuideStep) {}, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "never opened") {
		t.Errorf("err = %v, want it to say the application was never opened", err)
	}
}

// keepsChanging redraws itself for a while after being asked, the way an
// application does when it has just been raised or has just opened.
type keepsChanging struct {
	fakeScreen
	settlesAfter int
	extra        int
}

func (k *keepsChanging) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	k.captures++
	tree := editorTree()

	// New controls arriving, which is what a window finishing its layout looks
	// like and also what a click that opens something looks like.
	grown := k.captures
	if grown > k.settlesAfter {
		grown = k.settlesAfter
	}
	for i := 0; i < grown+k.extra; i++ {
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: "AXStaticText", Title: fmt.Sprintf("row %d", i),
		})
	}
	return tree, nil
}

// The bug this fixes: the coach drew "Click the Wallpaper row", the window
// finished settling, and it moved straight on with nobody having touched
// anything — then told the person it was all done.
func TestGuideDoesNotMistakeAWindowSettlingForSomebodyActing(t *testing.T) {
	poll, timeout, settle := guidePoll, guideStepTimeout, settleBeforeJudging
	guidePoll, guideStepTimeout, settleBeforeJudging = time.Millisecond, 60*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() {
		guidePoll, guideStepTimeout, settleBeforeJudging = poll, timeout, settle
	})

	// Grows for the first few reads and then holds still. Nobody ever acts.
	screenReader := &keepsChanging{settlesAfter: 5}
	e := &Engine{UIScreen: screenReader, Steps: &alwaysStep{step: uiact.Step{
		Do:       "click",
		Target:   uiact.Selector{Role: "AXTextArea", Title: "Message"},
		Describe: "Click the Message area",
	}}}

	var shown []GuideStep
	_, err := e.RunGuide(context.Background(), guidePlan(), func(g GuideStep) {
		shown = append(shown, g)
	}, nil)

	if err == nil {
		t.Fatal("the run finished, though nobody ever acted on the step")
	}
	if !strings.Contains(err.Error(), "nothing happened") {
		t.Errorf("err = %v, want it to give up waiting rather than move on", err)
	}

	// The same step, pointed at again. Not a second step reached by mistake.
	for _, g := range shown {
		if g.Number != 1 {
			t.Errorf("reached step %d without anybody acting", g.Number)
		}
	}
}

// Next on the "open it" step is a claim that the application is open, and it is
// checked before it is believed. Seen live: ⌥Space pressed before System
// Settings was opened carried the run on into an application that was not
// running, and it stopped with an error instead of the step asking for it.
func TestPressingNextBeforeOpeningTheAppDoesNotSkipTheStep(t *testing.T) {
	quickGuide(t)
	quickOpen(t)

	screenReader := &closedThenOpen{
		fakeScreen: fakeScreen{tree: editorTree()},
		opensAfter: 1 << 30,
	}
	e := &Engine{UIScreen: screenReader, Steps: &alwaysStep{step: typeIntoMessage()}}

	advance := make(chan struct{}, 4)
	advance <- struct{}{}
	advance <- struct{}{}

	var shown []GuideStep
	plan := &Plan{Guide: &GuidePlan{App: "Editor", Task: "say hello", MustOpen: true}}
	_, err := e.RunGuide(context.Background(), plan, func(s GuideStep) { shown = append(shown, s) }, advance)

	if err == nil || !strings.Contains(err.Error(), "never opened") {
		t.Fatalf("err = %v; pressing Next must not stand in for opening the application", err)
	}
	for _, s := range shown {
		if s.Do != "open" {
			t.Fatalf("moved on to %q though the application was never opened", s.Label)
		}
	}
}

type awayForAWhile struct {
	fakeScreen
	reads     int
	awayFrom  int
	awayUntil int
	typedFrom int
}

func (s *awayForAWhile) Capture(context.Context, string) (uiact.Tree, error) {
	s.reads++
	if s.reads >= s.awayFrom && s.reads < s.awayUntil {
		return uiact.Tree{}, uiact.ErrNoWindow
	}
	if s.reads >= s.typedFrom {
		moved := editorTree()
		moved.Elements[0].Value = "hello"
		return moved, nil
	}
	return editorTree(), nil
}

func TestTimeSpentInAnotherApplicationDoesNotUseUpTheStep(t *testing.T) {
	quickGuide(t)
	guideStepTimeout = 30 * time.Millisecond
	awayAtMost = 5 * time.Second

	screen := &awayForAWhile{awayFrom: 4, awayUntil: 400, typedFrom: 420}
	e := &Engine{UIScreen: screen, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	var shown []GuideStep
	if _, err := e.RunGuide(context.Background(), guidePlan(), func(g GuideStep) { shown = append(shown, g) }, nil); err != nil {
		t.Fatalf("RunGuide: %v, want the step held while they were away", err)
	}
	if len(shown) != 1 {
		t.Errorf("the step was shown %d time(s), want it held as it was: %+v", len(shown), shown)
	}
}

type runningButHidden struct{ fakeScreen }

func (*runningButHidden) Capture(context.Context, string) (uiact.Tree, error) {
	return uiact.Tree{}, uiact.ErrNoWindow
}

func TestPlanningAGuideDoesNotBringAHiddenApplicationForward(t *testing.T) {
	screen := &runningButHidden{}
	e := &Engine{UIScreen: screen, Steps: &alwaysStep{step: typeIntoMessage()}, Log: func(string, ...any) {}}

	plan, err := e.planGuideIn(context.Background(), "say hello", "", nil, "Editor", nil)
	if err != nil {
		t.Fatalf("planGuideIn: %v", err)
	}
	if plan.Guide == nil || !plan.Guide.MustOpen || !plan.Guide.Running {
		t.Fatalf("guide = %+v, want switching to it left as the first step", plan.Guide)
	}
	if !strings.Contains(plan.Guide.Preview, "switch to it first") {
		t.Errorf("the preview does not say to switch: %q", plan.Guide.Preview)
	}
	if len(screen.raised) != 0 {
		t.Errorf("brought %v to the front while planning", screen.raised)
	}
}

func TestTheGuideSaysAtOnceThatAStepWasNoticed(t *testing.T) {
	quickGuide(t)

	moved := editorTree()
	moved.Elements[0].Value = "hello"
	screen := &fakeScreen{tree: editorTree(), afterwards: &moved, changesAfter: 1}
	noticed := 0
	e := &Engine{UIScreen: screen, Noticed: func() { noticed++ }, Steps: &scriptedSteps{
		answers: []synth.StepAnswer{{Step: typeIntoMessage()}, {Done: true, Why: "that is all"}},
	}}

	if _, err := e.RunGuide(context.Background(), guidePlan(), func(GuideStep) {}, nil); err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if noticed != 1 {
		t.Errorf("said a step was noticed %d time(s), want once for the one step done", noticed)
	}
}

type pointsAgain struct{ calls int }

func (p *pointsAgain) WritePlan(_ context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	p.calls++
	return synth.PlanAnswer{Steps: []uiact.Step{clickMessage()}}, nil
}

func TestAskedWhereSomethingIsTheGuideStopsOnceItHasPointedAtIt(t *testing.T) {
	quickGuide(t)

	planner := &pointsAgain{}
	advance := make(chan struct{}, 4)
	advance <- struct{}{}
	e := &Engine{UIScreen: &movingScreen{}, Plans: planner, Steps: &countingSteps{}}

	var shown []GuideStep
	plan := &Plan{Guide: &GuidePlan{App: "Editor", Task: "show me where the message box is"}}
	_, err := e.RunGuide(context.Background(), plan, func(g GuideStep) {
		shown = append(shown, g)
		select {
		case advance <- struct{}{}:
		default:
		}
	}, advance)
	if err != nil {
		t.Fatalf("RunGuide: %v", err)
	}
	if len(shown) != 1 {
		t.Errorf("pointed %d time(s), want once: the second plan named the same control", len(shown))
	}
	if planner.calls != 2 {
		t.Errorf("the planner was asked %d time(s), want 2", planner.calls)
	}
}
