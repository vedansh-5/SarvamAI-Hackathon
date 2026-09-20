package uiact

import "context"

type Screen interface {
	Capture(ctx context.Context, app string) (Tree, error)
	Act(ctx context.Context, p Program, t Tree) error
	Frontmost(ctx context.Context) (string, error)
	BringToFront(ctx context.Context, app string) error
	Type(ctx context.Context, text string) error
}

type System struct{ Runner Runner }

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
