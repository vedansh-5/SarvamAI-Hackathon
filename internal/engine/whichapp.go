package engine

import (
	"context"
	"fmt"
	"github.com/vedansh/vans/internal/names"
	"strings"

	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// AppChooser names the application a task is carried out in. Implemented by the
// synthesis backend.
type AppChooser interface {
	WhichApp(ctx context.Context, req synth.AppChoiceRequest) (synth.AppChoice, error)
}

// ErrNowhereToDoIt is a task no application on this Mac carries out.
var ErrNowhereToDoIt = fmt.Errorf("engine: nothing installed does that")

// AppNotOpenError is the task belonging somewhere that is not running.
//
// Not a failure. VANS does not open applications (D-032), so the person opens it
// and the coach picks up inside it — which means this has to carry enough for
// them to act on rather than just a name.
type AppNotOpenError struct {
	App  string
	Task string
	Why  string
}

func (e *AppNotOpenError) Error() string {
	if e.Why != "" {
		return fmt.Sprintf("engine: that is done in %s, which is not open — %s", e.App, e.Why)
	}
	return fmt.Sprintf("engine: that is done in %s, which is not open", e.App)
}

// appForTask works out where a task is carried out, which is usually not where
// the person was looking when they asked.
//
// The application behind the panel used to be taken as the answer. It is only
// ever the context: someone reading mail asks how to change the wallpaper, and
// hunting for wallpaper controls in a mail client finds nothing and reports that
// the window had nothing to point at — true, useless, and the reason this exists.
//
// Falls back to the application in front when there is no chooser or the question
// cannot be answered. Being unable to improve on the old behaviour is not a
// reason to refuse the request.
type PlaceStore interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

func placeKey(shape string) string { return "place:" + shape }

func (e *Engine) placeRemembered(ctx context.Context, shape, task string) (string, error, bool) {
	if e.Places == nil || e.Apps == nil || strings.TrimSpace(shape) == "" {
		return "", nil, false
	}
	kept, err := e.Places.Setting(ctx, placeKey(shape))
	if err != nil || strings.TrimSpace(kept) == "" {
		return "", nil, false
	}
	name, why, _ := strings.Cut(kept, "\t")
	app, err := e.Apps.FindInstalled(name)
	if err != nil {
		e.logf("%s was where this is done, and is no longer installed; working it out again", name)
		return "", nil, false
	}
	e.logf("this is done in %s, remembered from the last time it was shown", app.Name)
	if !e.isRunning(app.Name) {
		return "", &AppNotOpenError{App: app.Name, Task: task, Why: why}, true
	}
	return app.Name, nil, true
}

func (e *Engine) rememberThePlace(ctx context.Context, g *GuidePlan) {
	if e.Places == nil || g == nil || !g.Elsewhere || g.Web || strings.TrimSpace(g.Shape) == "" {
		return
	}
	if err := e.Places.SetSetting(context.WithoutCancel(ctx), placeKey(g.Shape), g.App+"\t"+g.Why); err != nil {
		e.logf("could not remember where this is done: %v", err)
	}
}

func (e *Engine) appForTask(ctx context.Context, task string, lookingAt *screen.Context) (string, error) {
	front, frontErr := e.appInQuestion(ctx, lookingAt)

	if e.Chooser == nil || e.Apps == nil {
		return front, frontErr
	}

	installed := namesOf(e.Apps.Installed())
	if len(installed) == 0 {
		return front, frontErr
	}

	inFrontHere := strings.TrimSpace(front) != "" && !strings.EqualFold(front, "vans")
	if inFrontHere && !named(installed, front) {
		installed = append(installed, front)
	}

	choice, err := e.Chooser.WhichApp(ctx, synth.AppChoiceRequest{
		Task:      task,
		InFront:   front,
		Running:   e.runningApps(installed),
		Installed: installed,
	})
	if err != nil {
		e.logf("could not work out where that is done (%v); using the window in front", err)
		return front, frontErr
	}

	if choice.Nowhere {
		return "", fmt.Errorf("%w: %s", ErrNowhereToDoIt, reasonOr(choice.Why, "no reason given"))
	}

	// The model answers with a name off the installed list, and the catalogue is
	// what turns that into the application as it is actually installed.
	if inFrontHere && names.Same(choice.App, front) {
		return front, frontErr
	}

	app, err := e.Apps.FindInstalled(choice.App)
	if err != nil {
		e.logf("%q is not an application here (%v); using the window in front", choice.App, err)
		return front, frontErr
	}

	if !strings.EqualFold(app.Name, front) {
		e.logf("that is done in %s, not %s", app.Name, front)
	}

	if !e.isRunning(app.Name) {
		return "", &AppNotOpenError{App: app.Name, Task: task, Why: choice.Why}
	}
	return app.Name, nil
}

// runningApps is what is open, narrowed to things that are actually
// applications. The window list also names helpers nobody switches to —
// AutoFill, WindowManager, loginwindow — and an installed bundle is the test for
// the difference, which beats a list of names to ignore that would go stale.
func (e *Engine) runningApps(installed []string) []string {
	reporter, ok := e.UIScreen.(uiact.RunningApps)
	if !ok {
		return nil
	}

	known := make(map[string]bool, len(installed))
	for _, name := range installed {
		known[strings.ToLower(name)] = true
	}

	var out []string
	for _, name := range reporter.Running() {
		if known[strings.ToLower(name)] {
			out = append(out, name)
		}
	}
	return out
}

func (e *Engine) isRunning(app string) bool {
	reporter, ok := e.UIScreen.(uiact.RunningApps)
	if !ok {
		// Unknown, and guessing "not open" would send someone to open something
		// that is already in front of them.
		return true
	}
	for _, name := range reporter.Running() {
		if strings.EqualFold(name, app) {
			return true
		}
	}
	return false
}

func namesOf(apps []capability.App) []string {
	out := make([]string, 0, len(apps))
	for _, a := range apps {
		out = append(out, a.Name)
	}
	return out
}

func named(list []string, name string) bool {
	for _, n := range list {
		if names.Same(n, name) {
			return true
		}
	}
	return false
}
