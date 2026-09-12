package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type UIActionWriter interface {
	WriteUIAction(ctx context.Context, req synth.UIActRequest) (uiact.Program, error)
}

type UIActionCache interface {
	WindowAction(ctx context.Context, app, shape string) (uiact.Program, bool, error)
	SaveWindowAction(ctx context.Context, app, shape string, p uiact.Program, model string) error
	ForgetWindowAction(ctx context.Context, app, shape string) error
}

type UIPlan struct {
	App     string
	Preview string

	program uiact.Program
}

func (e *Engine) canActOnUI() bool {
	return e.UIWriter != nil && e.UIRunner != nil
}

func (e *Engine) actOnUI(ctx context.Context, r route.Route, shape, app string) (*Plan, int, error) {
	if !e.canActOnUI() {
		return nil, 0, fmt.Errorf("engine: this build cannot act through accessibility")
	}

	tree, err := uiact.Capture(ctx, e.UIRunner, app)
	if err != nil {
		return nil, 0, err
	}
	if len(tree.Elements) == 0 {
		return nil, 0, fmt.Errorf("engine: %s has nothing on screen to act on", app)
	}

	template, calls, err := e.programForWindow(ctx, tree, r, shape)
	if err != nil {
		return nil, calls, err
	}

	filled := template.Fill(func(v string) string { return route.Fill(v, r.Literals) })
	if err := uiact.Validate(filled, tree); err != nil {
		return nil, calls, err
	}

	return &Plan{
		Task:           Task{Intent: r.Slot("action"), Title: tree.App},
		Destination:    tree.App,
		SynthesisCalls: calls,
		FromCache:      calls == 0,
		UI: &UIPlan{
			App:     tree.App,
			Preview: uiact.Preview(filled),
			program: filled,
		},
	}, calls, nil
}

func (e *Engine) programForWindow(ctx context.Context, tree uiact.Tree, r route.Route, shape string) (uiact.Program, int, error) {
	if e.UIActions != nil {
		if cached, found, err := e.UIActions.WindowAction(ctx, tree.App, shape); err != nil {
			e.logf("window action cache unreadable: %v", err)
		} else if found {
			if err := uiact.Validate(cached, tree); err == nil {
				e.logf("window action cache hit for %s", tree.App)
				return cached, 0, nil
			}
			e.logf("cached window program no longer fits %s; writing it again", tree.App)
			if err := e.UIActions.ForgetWindowAction(ctx, tree.App, shape); err != nil {
				e.logf("could not drop the stale window action: %v", err)
			}
		}
	}

	var failures []string
	budget := e.maxSynthesis()

	for attempt := 1; attempt <= budget; attempt++ {
		program, err := e.UIWriter.WriteUIAction(ctx, synth.UIActRequest{
			Action:       templateOr(r, "action"),
			Target:       templateOr(r, "target"),
			Tree:         tree,
			Placeholders: route.PlaceholdersIn(shape),
			Failures:     failures,
		})
		if err != nil {
			return uiact.Program{}, attempt, err
		}

		var rejected []string
		if err := uiact.Validate(program, tree); err != nil {
			var invalid *uiact.InvalidError
			if !errors.As(err, &invalid) {
				return uiact.Program{}, attempt, err
			}
			rejected = invalid.Reasons
		}
		for _, baked := range uiact.Baked(program, r.Literals) {
			rejected = append(rejected, fmt.Sprintf(
				"it writes %q into the program instead of the placeholder standing for it; "+
					"this program is reused for every request of this shape", baked))
		}
		if len(rejected) > 0 {
			e.logf("window program rejected: %s", strings.Join(rejected, "; "))
			failures = rejected
			continue
		}

		if e.UIActions != nil {
			if err := e.UIActions.SaveWindowAction(ctx, tree.App, shape, program, e.Model); err != nil {
				e.logf("could not cache the window action: %v", err)
			}
		}
		return program, attempt, nil
	}

	return uiact.Program{}, budget, &UnroutableError{
		Instruction: r.Slot("action"),
		Why:         fmt.Sprintf("no way to do that in %s's window survived being checked against it", tree.App),
		Can:         e.capabilities(),
	}
}

func (e *Engine) runUI(ctx context.Context, plan *Plan) (Receipt, error) {
	if !e.canActOnUI() {
		return Receipt{}, fmt.Errorf("engine: this build cannot act through accessibility")
	}

	fresh, err := uiact.Capture(ctx, e.UIRunner, plan.UI.App)
	if err != nil {
		return Receipt{}, err
	}
	if err := uiact.Validate(plan.UI.program, fresh); err != nil {
		return Receipt{}, fmt.Errorf("engine: %s's window changed after the plan was made: %w", plan.UI.App, err)
	}

	receipt, err := uiact.Run(ctx, e.UIRunner, plan.UI.program, fresh)
	if err != nil {
		return Receipt{}, err
	}

	e.logf("ran %d step(s) in %s's window, verified=%v",
		len(plan.UI.program.Steps), plan.UI.App, receipt.Verified)
	return Receipt{Title: plan.UI.App, Verified: receipt.Verified}, nil
}
