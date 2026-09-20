package uiact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const StepPause = 350 * time.Millisecond

type Act struct {
	Step int
	Do   string
	X, Y int
	Text string
	Code int
}

func Plan(p Program, t Tree) ([]Act, error) {
	if err := Validate(p, t); err != nil {
		return nil, err
	}

	var out []Act
	for i, step := range p.Steps {
		el, why := t.resolve(step.Target)
		if why != "" {
			return nil, &InvalidError{App: t.App, Reasons: []string{why}}
		}

		switch verb(step.Do) {
		case "click", "type":
			x, y, ok := t.ClickPoint(el)
			if !ok {
				return nil, &InvalidError{App: t.App, Reasons: []string{fmt.Sprintf(
					"step %d: no part of the %s %q is visible on screen, so it cannot be clicked",
					i+1, el.Role, el.Title)}}
			}
			out = append(out, Act{Step: i, Do: "click", X: x, Y: y})
			if verb(step.Do) == "type" {
				out = append(out, Act{Step: i, Do: "type", X: x, Y: y, Text: step.Text})
			}

		case "press":
			out = append(out, Act{Step: i, Do: "press", Code: Keys[step.Key]})
		}
	}
	return out, nil
}

func RunScreen(ctx context.Context, r Runner, p Program, t Tree) error {
	acts, err := Plan(p, t)
	if err != nil {
		return err
	}

	pid, err := pidFor(ctx, r, t.App)
	if err != nil {
		return err
	}
	if err := bringForward(ctx, r, t.App, pid); err != nil {
		return err
	}
	if err := VerifyTargets(pid, p, t); err != nil {
		return err
	}

	for _, a := range acts {
		switch a.Do {
		case "click":
			clickPoster(a.X, a.Y)
		case "type":
			textPoster(a.Text)
		case "press":
			keyPoster(a.Code)
		}
		if !pause(ctx, StepPause) {
			return ctx.Err()
		}
	}
	return nil
}

func TypeHere(ctx context.Context, text string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("uiact: nothing to type")
	}
	postText(text)
	pause(ctx, StepPause)
	return nil
}

func pause(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

var (
	lookupAt     = elementAt
	frontOwnerAt = frontWindowAt
	onScreen     = windowOnScreen
	raise        = raiseApp
	clickPoster  = postClick
	textPoster   = postText
	keyPoster    = postKey
)

var windowWait = 1200 * time.Millisecond

func VerifyTargets(pid int, p Program, t Tree) error {
	for i, step := range p.Steps {
		if verb(step.Do) == "press" {
			continue
		}

		want, why := t.resolve(step.Target)
		if why != "" {
			return &InvalidError{App: t.App, Reasons: []string{why}}
		}
		x, y, ok := t.ClickPoint(want)
		if !ok {
			continue
		}

		if owner, name, known := frontOwnerAt(x, y); known && owner != pid {
			return &InvalidError{App: t.App, Reasons: []string{fmt.Sprintf(
				"step %d: (%d,%d) is inside %s's window, which is in front of %s, "+
					"so clicking there would not reach it",
				i+1, x, y, name, t.App)}}
		}

		role, title, owned, err := lookupAt(pid, x, y)
		if err != nil {
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		if owned && !sameControl(want, role, title) {
			return &InvalidError{App: t.App, Reasons: []string{fmt.Sprintf(
				"step %d: expected the %s %q at (%d,%d) and found a %s %q",
				i+1, want.Role, want.Title, x, y, role, title)}}
		}
	}
	return nil
}

func sameControl(want Element, role, title string) bool {
	if !strings.EqualFold(strings.TrimSpace(role), strings.TrimSpace(want.Role)) {
		return false
	}
	if strings.TrimSpace(want.Title) == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(title), strings.TrimSpace(want.Title))
}

// Raising an application is done through accessibility, which VANS already has
// to hold in order to read a window at all. System Events can raise it too, and
// that asks for Automation on top — a second permission, for something the
// first one already covers. It stays as the fallback for the case where
// accessibility cannot do it.
func bringForward(ctx context.Context, r Runner, app string, pid int) error {
	if alreadyInFront(app) {
		return settled(ctx, app, pid)
	}
	if err := raise(pid); err == nil {
		return settled(ctx, app, pid)
	}
	return forwardWithEvents(ctx, r, app, pid)
}

func alreadyInFront(app string) bool {
	name, ok := askTheWindowList()
	return ok && strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(app))
}

func settled(ctx context.Context, app string, pid int) error {
	if !waitForWindow(ctx, pid) {
		return fmt.Errorf("%w — %s", ErrWindowHidden, app)
	}
	return nil
}

func forwardWithEvents(ctx context.Context, r Runner, app string, pid int) error {
	name, err := literal(app)
	if err != nil {
		return err
	}

	out, err := r.Run(ctx, fmt.Sprintf(forwardScript, name))
	if err != nil {
		return classify(err.Error(), err)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out, &envelope); err != nil {
		return fmt.Errorf("uiact: unreadable reply (output %q): %w", truncate(string(out), 200), err)
	}
	if !envelope.OK {
		return classify(envelope.Error, fmt.Errorf("uiact: %s", envelope.Error))
	}

	return settled(ctx, app, pid)
}

func waitForWindow(ctx context.Context, pid int) bool {
	deadline := time.Now().Add(windowWait)
	for {
		if onScreen(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

const forwardScript = `(function () {
  ObjC.import('Foundation');
  const se = Application('System Events');
  function wait(seconds) {
    $.NSThread.sleepForTimeInterval(seconds);
  }
  function reply(f) {
    try { return JSON.stringify({ ok: true, value: f() }); }
    catch (e) { return JSON.stringify({ ok: false, error: String((e && e.message) || e) }); }
  }
  return reply(function () {
    const proc = se.applicationProcesses.byName(%[1]s);
    if (proc.frontmost()) return 'front';
    try { Application(%[1]s).activate(); }
    catch (e) { proc.frontmost = true; }
    wait(0.5);
    if (!proc.frontmost()) throw new Error('could not bring that application to the front');
    return 'front';
  });
})();
`
