package uiact

import (
	"context"
	"fmt"
)

var ErrEstimated = fmt.Errorf("uiact: that control's place was estimated from a picture, so VANS points at it and leaves the click to you")

func RunByPointer(ctx context.Context, r Runner, p Program, t Tree) error {
	acts, err := Plan(p, t)
	if err != nil {
		return err
	}
	for _, step := range p.Steps {
		if step.Switches() || step.Waits() || verb(step.Do) == "press" {
			continue
		}
		if el, why := t.resolve(step.Target); why == "" && el.Estimated {
			return fmt.Errorf("%w (%s %q)", ErrEstimated, el.Role, el.Label())
		}
	}

	pid, err := pidFor(ctx, r, t.App)
	if err != nil {
		return err
	}
	if err := bringForward(ctx, r, t.App, pid); err != nil {
		return err
	}
	_ = raise(pid)
	if !pause(ctx, keyboardSettle) {
		return ctx.Err()
	}

	for _, a := range acts {
		switch a.Do {
		case "click", "focus":
			clickPoster(a.X, a.Y)
		case "type":
			if !pause(ctx, keyboardSettle) {
				return ctx.Err()
			}
			textPoster(a.Text)
		case "press":
			keyPoster(a.Code, a.Flags)
		case "choose":
			if err := ChooseFile(ctx, r, t.App, a.Text); err != nil {
				return err
			}
		case "doubleclick":
			doubleClicker(a.X, a.Y)
		case "rightclick":
			rightClicker(a.X, a.Y)
		case "hover":
			mover(a.X, a.Y)
		case "scroll":
			down, right := scrollBy(a.Text)
			scroller(a.X, a.Y, down, right)
		case "drag":
			dragger(a.X, a.Y, a.ToX, a.ToY)
		case "set":
			return fmt.Errorf("uiact: a slider seen only in a picture cannot be set; drag it instead")
		}
		if !pause(ctx, StepPause) {
			return ctx.Err()
		}
	}
	return nil
}
