//go:build integration

package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func liveApp(t *testing.T) (string, context.Context) {
	t.Helper()
	if os.Getenv("VANS_LIVE") == "" {
		t.Skip("set VANS_LIVE=1 to drive a real window")
	}
	app := os.Getenv("VANS_LIVE_APP")
	if app == "" {
		t.Skip("set VANS_LIVE_APP to the application to drive, e.g. VANS_LIVE_APP=TextEdit")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	return app, ctx
}

// lines collects what the engine says, so an assertion can be made about what
// actually happened rather than only about the receipt at the end.
type lines struct {
	mu   sync.Mutex
	said []string
}

func (l *lines) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.said = append(l.said, strings.TrimSpace(fmt.Sprintf(format, args...)))
}

func (l *lines) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.said, "\n")
}

// TestLiveDriveTask drives a real window with a real model and checks the
// window moved.
//
// It used to script `{Done: true}` as the only answer, which drove nothing: the
// model was never asked, no step was performed, and the receipt came back
// verified because every step having landed is trivially true of none. It
// proved the plumbing compiled.
//
//	VANS_LIVE=1 VANS_LIVE_DRIVE_OK=1 \
//	VANS_LIVE_APP=TextEdit VANS_LIVE_TASK="type the word hello" \
//	go test -tags integration -run TestLiveDriveTask ./internal/engine/
//
// The target must be something with nothing to lose — an empty TextEdit
// document, not a window holding work.
func TestLiveDriveTask(t *testing.T) {
	app, ctx := liveApp(t)
	if os.Getenv("VANS_LIVE_DRIVE_OK") == "" {
		t.Skip("set VANS_LIVE_DRIVE_OK=1 to run a live driving task on screen")
	}

	task := os.Getenv("VANS_LIVE_TASK")
	if task == "" {
		t.Skip("set VANS_LIVE_TASK to the task to carry out, e.g. VANS_LIVE_TASK=\"type the word hello\"")
	}

	steps, err := synth.FromEnv()
	if err != nil {
		t.Skipf("no model configured: %v", err)
	}
	writer, canWriteSteps := steps.(StepWriter)
	if !canWriteSteps {
		t.Skip("the configured backend cannot write steps")
	}

	runner := appleevents.Osascript{Timeout: 120 * time.Second}
	screen := uiact.System{Runner: runner}

	before, err := screen.Capture(ctx, app)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if len(before.Elements) == 0 {
		t.Fatalf("%s has no controls on screen", app)
	}
	t.Logf("%s: %d controls before", app, len(before.Elements))

	said := &lines{}
	eng := &Engine{UIScreen: screen, Steps: writer, Log: said.logf}

	receipt, err := eng.Commit(ctx, &Plan{Drive: &DrivePlan{App: app, Task: task}})
	t.Logf("what the engine said:\n%s", said.all())
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The receipt alone is not the proof. A run that performed nothing is no
	// longer verified, so this is now a real assertion rather than one that
	// holds however little happened.
	if !receipt.Verified {
		t.Errorf("receipt.Verified = false: the window did not move for every step performed")
	}
	if !strings.Contains(said.all(), "step 1") {
		t.Error("no step was ever worked out, so nothing was driven")
	}

	// Read independently of the engine. The receipt is the engine's account of
	// itself; this is the window's.
	after, err := screen.Capture(ctx, app)
	if err != nil {
		t.Fatalf("Capture after: %v", err)
	}
	changes := uiact.Changes(before, after)
	if len(changes) == 0 {
		t.Error("the window is exactly as it started, so nothing reached it")
	}
	t.Logf("what changed in the window:\n  %s", strings.Join(changes, "\n  "))
	t.Logf("receipt: title=%s verified=%v", receipt.Title, receipt.Verified)
}

// TestChoosesTheRightAppLive checks the decision that a task belongs somewhere
// other than the window in front, against a real model and the real machine.
//
// The case it exists for: asking how to change the wallpaper while looking at an
// editor. That used to hunt for wallpaper controls in the editor and report that
// the window had nothing to point at.
//
//	go test -tags integration -v -run TestChoosesTheRightAppLive ./internal/engine/
func TestChoosesTheRightAppLive(t *testing.T) {
	backend, err := synth.FromEnv()
	if err != nil {
		t.Skipf("no model configured: %v", err)
	}
	chooser, canChoose := backend.(AppChooser)
	if !canChoose {
		t.Skip("the configured backend cannot choose an application")
	}

	e := &Engine{
		Apps:     capability.New(),
		Chooser:  chooser,
		UIScreen: uiact.System{Runner: appleevents.Osascript{}},
		Log:      func(format string, args ...any) { t.Logf(format, args...) },
	}

	// Named applications rather than whatever happens to be open, so the answer
	// does not depend on the state of the machine it runs on.
	for _, c := range []struct {
		task  string
		front string
		want  string
	}{
		{"how do I change the wallpaper", "Antigravity IDE", "System Settings"},
		{"how do I turn on dark mode", "Telegram", "System Settings"},
		{"how do I start a new chat", "Telegram", "Telegram"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		got, err := e.appForTask(ctx, c.task, &screen.Context{AppName: c.front})
		cancel()

		// Not open is the right answer too, and it names the application it
		// would have used.
		var notOpen *AppNotOpenError
		if errors.As(err, &notOpen) {
			got = notOpen.App
			err = nil
		}
		if err != nil {
			t.Errorf("%q with %s in front: %v", c.task, c.front, err)
			continue
		}
		if got != c.want {
			t.Errorf("%q with %s in front chose %q, want %q", c.task, c.front, got, c.want)
			continue
		}
		t.Logf("%-34q front=%-16s -> %s", c.task, c.front, got)
	}
}

// TestLiveDriveTaskTwice runs the same task twice against a real window with a
// real model, and measures what each run cost.
//
// This is the test the step cache exists to pass. Everything else about it is
// proved against fakes, where "asked the model nothing" is something a test
// double was told to report. Here the model is real, the window is real, and the
// second run's zero is the actual number of requests that left the machine.
//
//	VANS_LIVE=1 VANS_LIVE_DRIVE_OK=1 \
//	VANS_LIVE_APP=TextEdit VANS_LIVE_TASK="type the word hello" \
//	go test -tags integration -v -count=1 -run TestLiveDriveTaskTwice ./internal/engine/
//
// The target must be something with nothing to lose — an empty TextEdit
// document, not a window holding work. The task runs twice, so whatever it does
// happens twice.
func TestLiveDriveTaskTwice(t *testing.T) {
	app, ctx := liveApp(t)
	if os.Getenv("VANS_LIVE_DRIVE_OK") == "" {
		t.Skip("set VANS_LIVE_DRIVE_OK=1 to run a live driving task on screen")
	}

	task := os.Getenv("VANS_LIVE_TASK")
	if task == "" {
		t.Skip("set VANS_LIVE_TASK to the task to carry out, e.g. VANS_LIVE_TASK=\"type the word hello\"")
	}

	backend, err := synth.FromEnv()
	if err != nil {
		t.Skipf("no model configured: %v", err)
	}
	writer, canWriteSteps := backend.(StepWriter)
	if !canWriteSteps {
		t.Skip("the configured backend cannot write steps")
	}

	// A store of its own, so the first run is genuinely the first. Pointing this
	// at the real database would make the result depend on what had been run
	// before it, which is the one thing this test must not do.
	st, err := store.Open(filepath.Join(t.TempDir(), "traces.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	runner := appleevents.Osascript{Timeout: 120 * time.Second}
	screen := uiact.System{Runner: runner}
	shape, literals := route.Shape(task)
	t.Logf("task %q has shape %q with values %v", task, shape, literals)

	cold := driveOnceLive(ctx, t, "cold", app, task, shape, literals, screen, writer, st)
	warm := driveOnceLive(ctx, t, "warm", app, task, shape, literals, screen, writer, st)

	if cold.work == nil || warm.work == nil {
		t.Fatal("a run reported no counts, so there is nothing to compare")
	}

	if cold.work.Calls == 0 {
		t.Error("the first run asked the model nothing, so it was not a cold run and this proves nothing")
	}
	if cold.work.Replayed != 0 {
		t.Errorf("the first run reused %d step(s), so the store was not empty", cold.work.Replayed)
	}

	if warm.replayOffered == 0 {
		t.Fatal("the second run was offered no steps from the first, so the first was never kept " +
			"— the log above says why")
	}
	if warm.work.Calls != 0 {
		t.Errorf("the second run asked the model %d time(s), want none", warm.work.Calls)
	}
	if !warm.work.Warm() {
		t.Errorf("the second run was not warm: %+v", *warm.work)
	}
	if !warm.verified {
		t.Error("the second run was not verified, so replaying is not doing the task")
	}

	t.Logf("cold: %d step(s), %d model call(s), %s", cold.work.Steps, cold.work.Calls, cold.took.Round(time.Millisecond))
	t.Logf("warm: %d step(s), %d model call(s), %s", warm.work.Steps, warm.work.Calls, warm.took.Round(time.Millisecond))
	if warm.took < cold.took {
		t.Logf("the warm run was %s faster", (cold.took - warm.took).Round(time.Millisecond))
	} else {
		t.Logf("the warm run was not faster in wall time (%s against %s), though it asked nothing; "+
			"on a task this short the window reads dominate", warm.took.Round(time.Millisecond), cold.took.Round(time.Millisecond))
	}
}

type liveRun struct {
	work          *StepWork
	took          time.Duration
	verified      bool
	replayOffered int
}

func driveOnceLive(
	ctx context.Context, t *testing.T, which, app, task, shape string,
	literals map[string]string, screen uiact.Screen, writer StepWriter, st *store.Store,
) liveRun {
	t.Helper()

	said := &lines{}
	eng := &Engine{UIScreen: screen, Steps: writer, Traces: st, Log: said.logf, Model: "live"}

	plan, err := eng.driveOn(ctx, app, task, shape, literals)
	if err != nil {
		t.Fatalf("%s: driveOn: %v", which, err)
	}

	start := time.Now()
	receipt, err := eng.Commit(ctx, plan)
	took := time.Since(start)

	t.Logf("--- %s run ---\n%s", which, said.all())
	if err != nil {
		t.Fatalf("%s: Commit: %v", which, err)
	}

	return liveRun{
		work:          receipt.Work,
		took:          took,
		verified:      receipt.Verified,
		replayOffered: len(plan.Drive.Replay),
	}
}
