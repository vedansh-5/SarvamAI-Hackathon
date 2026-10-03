package uiact

import (
	"context"
	"fmt"
	"strconv"
)

type Screen interface {
	Capture(ctx context.Context, app string) (Tree, error)
	Act(ctx context.Context, p Program, t Tree) error
	Frontmost(ctx context.Context) (string, error)
	BringToFront(ctx context.Context, app string) error
	Type(ctx context.Context, text string) error
}

// RunningApps reports which applications are holding a window, so a decision
// about where a task belongs can prefer one that is already open.
//
// Optional rather than part of Screen: a build or a test double without it makes
// the decision with less to go on, which is worse but not broken, and putting it
// on Screen would oblige every fake to answer a question most of them do not care
// about.
type RunningApps interface {
	Running() []string
}

type WindowWatcher interface {
	Watch(ctx context.Context, app string) (<-chan struct{}, error)
}

type Scroller interface {
	ScrollIntoView(ctx context.Context, t Tree, sel Selector) error
}

type System struct{ Runner Runner }

func (s System) ScrollIntoView(ctx context.Context, t Tree, sel Selector) error {
	el, why := t.resolve(sel)
	if why != "" {
		return &InvalidError{App: t.App, Reasons: []string{why}}
	}
	pid, err := pidFor(ctx, s.Runner, t.App)
	if err != nil {
		return err
	}
	return scrollElement(pid, el.Path)
}

func (s System) Watch(ctx context.Context, app string) (<-chan struct{}, error) {
	pid, err := pidFor(ctx, s.Runner, app)
	if err != nil {
		return nil, err
	}
	return watchWindow(ctx, pid)
}

// Running needs no permission and prompts for nothing — it reads the window list.
func (s System) Running() []string { return appsOnScreen() }

func (s System) Capture(ctx context.Context, app string) (Tree, error) {
	return Capture(ctx, s.Runner, app)
}

func (s System) Act(ctx context.Context, p Program, t Tree) error {
	return RunScreen(ctx, s.Runner, p, t)
}

func (s System) BringToFront(ctx context.Context, app string) error {
	return BringToFront(ctx, s.Runner, app)
}

func (s System) Frontmost(ctx context.Context) (string, error) {
	return Frontmost(ctx, s.Runner)
}

func (s System) Type(ctx context.Context, text string) error {
	return TypeHere(ctx, text)
}

func (System) ClickAt(x, y int) { clickPoster(x, y) }

func (System) TypeText(text string) { textPoster(text) }

func (System) PressKey(code int, flags uint64) { keyPoster(code, flags) }

func (System) MoveTo(x, y int) { mover(x, y) }

func (System) Drag(x1, y1, x2, y2 int) { dragger(x1, y1, x2, y2) }

func (s System) ChooseFile(ctx context.Context, app, path string) error {
	return ChooseFile(ctx, s.Runner, app, path)
}

type SelectionReader interface {
	Selection(ctx context.Context, app string) (string, bool)
}

func (s System) Selection(ctx context.Context, app string) (string, bool) {
	pid, err := pidFor(ctx, s.Runner, app)
	if err != nil {
		return "", false
	}
	return selectedText(pid)
}

type ValueKeeper interface {
	ValueOf(ctx context.Context, t Tree, sel Selector) (string, bool)
	PutBack(ctx context.Context, app string, sel Selector, value string) error
}

var (
	valueSetter = setElementValue
	wholeValue  = elementValue
)

func (s System) ValueOf(ctx context.Context, t Tree, sel Selector) (string, bool) {
	el, why := t.resolve(sel)
	if why != "" || el.Blind || role(el.Role) == "securetextfield" {
		return "", false
	}
	if !el.Typeable() && !el.Adjustable() {
		return "", false
	}
	pid, err := pidFor(ctx, s.Runner, t.App)
	if err != nil {
		return "", false
	}
	if el.Adjustable() {
		n, ok := numberReader(pid, el.Path)
		return strconv.FormatFloat(n, 'g', -1, 64), ok
	}
	return wholeValue(pid, el.Path)
}

func (s System) PutBack(ctx context.Context, app string, sel Selector, value string) error {
	tree, err := Capture(ctx, s.Runner, app)
	if err != nil {
		return err
	}
	el, why := tree.resolve(sel)
	if why != "" {
		return fmt.Errorf("uiact: %s, so there is nowhere to put the text back", why)
	}
	pid, err := pidFor(ctx, s.Runner, app)
	if err != nil {
		return err
	}
	if el.Adjustable() {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("uiact: %q is not a value the %s %q can be put back to", value, el.Role, el.Title)
		}
		return numberSetter(pid, el.Path, n)
	}
	if err := valueSetter(pid, el.Path, value); err != nil {
		return err
	}
	if now, ok := wholeValue(pid, el.Path); ok && now != value {
		return fmt.Errorf("uiact: the %s %q did not take its text back", el.Role, el.Title)
	}
	return nil
}
