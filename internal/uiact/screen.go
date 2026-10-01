package uiact

import (
	"context"
	"errors"
	"fmt"
	"github.com/vedansh/vans/internal/names"
	"strings"
	"time"
)

const StepPause = 350 * time.Millisecond

type Act struct {
	Step  int
	Do    string
	X, Y  int
	Text  string
	Code  int
	Flags uint64

	// Path addresses the control through accessibility, so it can be pressed or
	// focused without the pointer going anywhere. X and Y stay as the fallback
	// for a control that publishes no action of its own.
	Path []int
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

			// Typing wants the control to have the keyboard, which a click is
			// only one way of arranging. Asking for it directly is the other, and
			// it does not move the pointer.
			reach := "click"
			if verb(step.Do) == "type" {
				reach = "focus"
			}
			out = append(out, Act{Step: i, Do: reach, X: x, Y: y, Path: el.Path})
			if verb(step.Do) == "type" {
				out = append(out, Act{Step: i, Do: "type", X: x, Y: y, Text: step.Text})
			}

		case "press":
			out = append(out, Act{Step: i, Do: "press", Code: Keys[step.Key], Flags: step.HeldDown()})
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
			reachFor(pid, a)
		case "focus":
			takeTheKeyboard(pid, a)
		case "type":
			textPoster(a.Text)
		case "press":
			keyPoster(a.Code, a.Flags)
		}
		if !pause(ctx, StepPause) {
			return ctx.Err()
		}
	}
	return nil
}

// reachFor presses a control, through accessibility where it can and with the
// mouse where it cannot.
//
// The pointer belongs to the person sitting there. Moving it to click something
// is what makes a runtime feel like it has taken the chair — and it is not needed:
// a control that publishes AXPress can be pressed where it stands, on a window
// that is not even in front, without anything of theirs moving. The mouse is the
// fallback for controls that publish nothing, not the default.
func reachFor(pid int, a Act) {
	if len(a.Path) > 0 {
		switch pressed, err := elementPresser(pid, a.Path); {
		case err != nil:
			// Not fatal. The click still reaches it, and the step is verified by
			// the window afterwards either way.
		case pressed:
			return
		}
	}
	clickPoster(a.X, a.Y)
}

// takeTheKeyboard gives a control the cursor before text is typed into it.
func takeTheKeyboard(pid int, a Act) {
	if len(a.Path) > 0 && elementFocuser(pid, a.Path) == nil {
		return
	}
	clickPoster(a.X, a.Y)
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

	elementPresser = pressElement
	elementFocuser = focusElement
)

var windowWait = 1200 * time.Millisecond

// How long a raise gets to take effect. Shorter than windowWait: coming to the
// front is the fast part, and every millisecond here is spent on every raise.
var frontWait = 800 * time.Millisecond

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
			return &InvalidError{App: t.App, Covered: true, Reasons: []string{fmt.Sprintf(
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

// ErrCannotBringForward is an application that would not come to the front.
//
// Its own error because there is exactly one thing to do about it and VANS is not
// the one to do it: the person brings the application up themselves.
var ErrCannotBringForward = errors.New("uiact: that application would not come to the front")

// CannotRaiseError names the application, so the caller can say which one to open
// rather than only that something did not work.
type CannotRaiseError struct{ App string }

func (e *CannotRaiseError) Error() string {
	return fmt.Sprintf("uiact: %s would not come to the front — open it yourself, then ask again", e.App)
}

func (e *CannotRaiseError) Unwrap() error { return ErrCannotBringForward }

// Raising an application is done through accessibility, and only through
// accessibility.
//
// This used to fall back to System Events, which asks for a second permission and
// — the reason the fallback is gone — opens the application. `Application(x).
// activate()` launches something that is not running, so an application that had
// merely lost its window was started by a runtime whose first rule is that it does
// not start things (D-032). It was reached exactly when the accessibility raise
// had failed, which is exactly when the application is least likely to be in the
// state the caller assumed.
//
// Setting kAXFrontmostAttribute cannot launch anything. It raises a running
// application or it fails, and failing is the honest answer: the caller says so
// and the person opens it.
func bringForward(ctx context.Context, r Runner, app string, pid int) error {
	if alreadyInFront(app) {
		return settled(ctx, app, pid)
	}
	if err := raise(pid); err != nil {
		return &CannotRaiseError{App: app}
	}
	return settled(ctx, app, pid)
}

// BringToFront gives an application the keyboard and waits for its window.
//
// The coach needs this before it points at anything: the panel holds the
// keyboard while the question is being typed, and the person is about to act in
// the application, not in VANS.
func BringToFront(ctx context.Context, r Runner, app string) error {
	pid, err := pidFor(ctx, r, app)
	if err != nil {
		return err
	}
	return bringForward(ctx, r, app, pid)
}

func alreadyInFront(app string) bool {
	name, ok := askTheWindowList()
	return ok && names.Same(name, app)
}

func settled(ctx context.Context, app string, pid int) error {
	if !waitForWindow(ctx, pid) {
		return fmt.Errorf("%w — %s", ErrWindowHidden, app)
	}

	// Then wait for it to actually be in front, which is a different question.
	// waitForWindow asks whether the application has a window on screen, and a
	// window with something on top of it has one — so raising a covered window
	// used to return the instant it was asked, while the raise was still in
	// flight. The step that followed was refused for being covered, by a window
	// that was on its way out of the way (measured 2026-09-26: it cost a model
	// call on a one-step task).
	//
	// Best effort, and deliberately not an error. Whether the target is really
	// reachable is settled by VerifyTargets against the window itself; this only
	// gives the raise time to happen. An application whose window-list name
	// differs from the name asked for would never satisfy this, and failing the
	// step over that would be worse than trying it.
	waitUntilInFront(ctx, app)
	return nil
}

func waitUntilInFront(ctx context.Context, app string) bool {
	deadline := time.Now().Add(frontWait)
	for {
		if alreadyInFront(app) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
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
