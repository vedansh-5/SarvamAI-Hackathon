package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type savedTrace struct {
	app     string
	shape   string
	program uiact.Program
	pinned  map[string]string
}

type traceRecorder struct {
	saved   []savedTrace
	saveErr error

	// What a lookup answers with, and what the run reported back about it.
	stored    uiact.Program
	pinned    map[string]string
	lookups   int
	worked    int
	diverged  int
	dropAfter bool
}

func (r *traceRecorder) DriveTrace(_ context.Context, _, _ string, literals map[string]string) (uiact.Program, bool, error) {
	r.lookups++
	if len(r.stored.Steps) == 0 {
		return uiact.Program{}, false, nil
	}
	for key, value := range r.pinned {
		if literals[key] != value {
			return uiact.Program{}, false, nil
		}
	}
	return r.stored, true, nil
}

func (r *traceRecorder) DriveTraceWorked(context.Context, string, string) error {
	r.worked++
	return nil
}

func (r *traceRecorder) DriveTraceDiverged(context.Context, string, string) (bool, error) {
	r.diverged++
	return r.dropAfter, nil
}

func (r *traceRecorder) SaveDriveTrace(_ context.Context, app, shape string, p uiact.Program, pinned map[string]string, _ string) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = append(r.saved, savedTrace{app: app, shape: shape, program: p, pinned: pinned})
	return nil
}

const helloShape = "type {s0} into the message"

func tracePlan() *Plan {
	return &Plan{Drive: &DrivePlan{
		App:      "Editor",
		Task:     "type hello into the message",
		Shape:    helloShape,
		Literals: map[string]string{"s0": "hello"},
	}}
}

// A window that changes after every capture, so each step is seen to land. The
// fake in screen_fake_test.go flips once; a landed run needs it to keep moving.
type movingScreen struct {
	fakeScreen
	moves int
}

func (m *movingScreen) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	m.moves++
	tree := editorTree()
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: "AXStaticText", Title: fmt.Sprintf("change %d", m.moves),
		Enabled: true, Position: []int{0, 0}, Size: []int{10, 10},
		Path: []int{0, m.moves + 1},
	})
	m.fakeScreen.captures++
	return tree, nil
}

func TestARunThatFinishedAndLandedIsKept(t *testing.T) {
	traces := &traceRecorder{}
	e := &Engine{
		UIScreen: &movingScreen{},
		Traces:   traces,
		Model:    "test-model",
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "the message says hello"},
		}},
	}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}

	if len(traces.saved) != 1 {
		t.Fatalf("kept %d trace(s), want 1", len(traces.saved))
	}
	got := traces.saved[0]
	if got.app != "Editor" || got.shape != helloShape {
		t.Errorf("keyed on (%q, %q), want (Editor, %q)", got.app, got.shape, helloShape)
	}
	if len(got.program.Steps) != 1 {
		t.Fatalf("kept %d step(s), want 1", len(got.program.Steps))
	}
	if text := got.program.Steps[0].Text; text != "{s0}" {
		t.Errorf("kept text %q, want the value lifted out to {s0}", text)
	}
	if len(got.pinned) != 0 {
		t.Errorf("pinned = %v, want a general trace", got.pinned)
	}
}

// The model reaching the step limit means the sequence is a prefix of the task,
// not the task. Keeping it would replay half a job and report it finished.
func TestARunThatNeverFinishedIsNotKept(t *testing.T) {
	traces := &traceRecorder{}
	e := &Engine{
		UIScreen: &movingScreen{},
		Traces:   traces,
		Steps:    &alwaysStep{step: typeIntoMessage()},
	}

	// alwaysStep never says Done, so this runs to MaxDriveSteps and errors.
	if _, err := e.runDrive(context.Background(), tracePlan()); err == nil {
		t.Fatal("runDrive succeeded on a run that never finished")
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d trace(s) from a run that hit the step limit", len(traces.saved))
	}
}

// A step that changed nothing is not known to have worked, so the sequence is not
// known to do the task (D-004).
func TestARunWithAStepThatChangedNothingIsNotKept(t *testing.T) {
	traces := &traceRecorder{}
	e := &Engine{
		// The plain fake returns the same tree every time, so nothing ever
		// changes and Landed stays false.
		UIScreen: &fakeScreen{tree: editorTree()},
		Traces:   traces,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: clickMessage()},
			{Done: true, Why: "done"},
		}},
	}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d trace(s) from a run whose step changed nothing", len(traces.saved))
	}
}

// The rule that matters most. Replaying a sentence the model composed would type
// this request's words into a different request, and no check downstream would
// notice.
func TestARunThatTypedComposedWordsIsNotKept(t *testing.T) {
	traces := &traceRecorder{}
	composed := typeIntoMessage()
	composed.Text = "Hi there, confirming our meeting on Tuesday."

	e := &Engine{
		UIScreen: &movingScreen{},
		Traces:   traces,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: composed},
			{Done: true, Why: "sent"},
		}},
	}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d trace(s) containing words the model wrote", len(traces.saved))
	}
}

// Without a shape there is no key, and caching it under the empty string would
// answer every unshaped request with the same steps.
func TestARunWithNoShapeIsNotKept(t *testing.T) {
	traces := &traceRecorder{}
	plan := tracePlan()
	plan.Drive.Shape = ""

	e := &Engine{
		UIScreen: &movingScreen{},
		Traces:   traces,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "done"},
		}},
	}

	if _, err := e.runDrive(context.Background(), plan); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
	if len(traces.saved) != 0 {
		t.Errorf("kept %d trace(s) with no shape to key them by", len(traces.saved))
	}
}

// Learning from a run is not part of carrying it out. The task has already
// happened by the time the trace is written, so failing the run over it would
// report a change that was made as a failure.
func TestFailingToKeepARunDoesNotFailTheRun(t *testing.T) {
	traces := &traceRecorder{saveErr: fmt.Errorf("disk is full")}
	e := &Engine{
		UIScreen: &movingScreen{},
		Traces:   traces,
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "done"},
		}},
	}

	receipt, err := e.runDrive(context.Background(), tracePlan())
	if err != nil {
		t.Fatalf("runDrive failed because the trace could not be saved: %v", err)
	}
	if !receipt.Verified {
		t.Error("receipt not verified, though every step landed")
	}
}

// A build with no store learns nothing and still works.
func TestARunWithNoTraceStoreIsFine(t *testing.T) {
	e := &Engine{
		UIScreen: &movingScreen{},
		Steps: &scriptedSteps{answers: []synth.StepAnswer{
			{Step: typeIntoMessage()},
			{Done: true, Why: "done"},
		}},
	}

	if _, err := e.runDrive(context.Background(), tracePlan()); err != nil {
		t.Fatalf("runDrive: %v", err)
	}
}

func TestARunThatWentInCirclesIsNotKept(t *testing.T) {
	traces := &traceRecorder{}
	e := &Engine{Traces: traces}
	d := &DrivePlan{App: "Editor", Task: "say hello", Shape: "say hello"}

	e.rememberTheRun(context.Background(), d, taskRun{
		Did:      []uiact.Step{clickMessage(), clickMessage(), typeIntoMessage()},
		Finished: true, Landed: true,
	})
	if len(traces.saved) != 0 {
		t.Errorf("kept %d run(s) that clicked the same control twice in a row", len(traces.saved))
	}
}

func TestStepsKeptFromALoopedRunAreNotReplayed(t *testing.T) {
	traces := &traceRecorder{stored: uiact.Program{App: "Editor", Steps: []uiact.Step{clickMessage(), clickMessage()}}}
	e := &Engine{Traces: traces}

	if got := e.stepsFromLastTime(context.Background(), "Editor", "say hello", nil); got != nil {
		t.Errorf("replayed %d step(s) kept from a run that went in circles", len(got))
	}
}
