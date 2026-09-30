package uiact

import "context"

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

type PointerReader interface {
	PointerAt() (x, y int, ok bool)
}

type System struct{ Runner Runner }

func (System) PointerAt() (int, int, bool) { return pointerAt() }

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
