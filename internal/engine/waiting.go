package engine

import (
	"context"
	"time"

	"github.com/vedansh/vans/internal/uiact"
)

var (
	waitCeiling = 3 * time.Minute
	waitPoll    = time.Second
)

func met(tree uiact.Tree, step uiact.Step) (bool, string) {
	el, why := tree.Resolve(step.Target)
	switch step.WaitsFor() {
	case uiact.WaitGoes:
		return why != "", "it has gone"
	case uiact.WaitReady:
		return why == "" && el.Enabled, "it is ready"
	default:
		return why == "", "it appeared"
	}
}

func (e *Engine) waitFor(ctx context.Context, app string, step uiact.Step) (uiact.Tree, string, error) {
	started := time.Now()
	for {
		tree, err := e.UIScreen.Capture(ctx, app)
		if err == nil && len(tree.Elements) > 0 {
			if ok, what := met(tree, step); ok {
				if waited := time.Since(started); waited > 2*waitPoll {
					e.logf("waited %s: %s", waited.Round(time.Second), what)
				}
				return tree, what, nil
			}
		}
		if time.Since(started) >= waitCeiling {
			return uiact.Tree{}, "", stopped(app,
				"engine: waited %s for the %s %q in %s and it never %s, so it stopped",
				waitCeiling.Round(time.Second), step.Target.Role, step.Target.Title, app, wanted(step))
		}
		select {
		case <-ctx.Done():
			return uiact.Tree{}, "", ctx.Err()
		case <-time.After(waitPoll):
		}
	}
}

func wanted(step uiact.Step) string {
	switch step.WaitsFor() {
	case uiact.WaitGoes:
		return "went"
	case uiact.WaitReady:
		return "became ready"
	default:
		return "appeared"
	}
}
