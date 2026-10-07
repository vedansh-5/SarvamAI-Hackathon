package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// ActionWriter writes a program against an application's surface. Implemented
// by the synthesis backend; kept as an interface here so the engine can be
// exercised without one.
type ActionWriter interface {
	WriteAction(ctx context.Context, req synth.AppActRequest) (appact.Program, error)
}

// ActionCache keeps programs by application and action shape. Declared here
// and implemented in store, the same way route.Cache is.
type ActionCache interface {
	Action(ctx context.Context, app, shape string) (appact.Program, bool, error)
	SaveAction(ctx context.Context, app, shape string, p appact.Program, model string) error
	ForgetAction(ctx context.Context, app, shape string) error
}

// ActionPlan is a prepared change inside an application, awaiting confirmation.
type ActionPlan struct {
	// App is the application, as it is installed. Resolved from the catalogue
	// rather than taken from the model.
	App string

	// Preview is what will happen, in the application's own words.
	Preview string

	// Writes reports whether running it changes anything. Decided from the
	// dictionary (D-042), never from the model.
	Writes bool

	program appact.Program
	surface capability.Surface
}

// UnknownAppError is an instruction naming an application VANS cannot drive.
type UnknownAppError struct {
	Name string
	Near []string
}

func (e *UnknownAppError) Error() string {
	msg := fmt.Sprintf("engine: no scriptable application named %q", e.Name)
	if len(e.Near) > 0 {
		msg += ": did you mean " + strings.Join(e.Near, ", ") + "?"
	}
	return msg
}

// act prepares an action inside an application. It never runs it.
func (e *Engine) actInApplication(ctx context.Context, r route.Route, shape string) (*Plan, int, error) {
	name := r.Slot("app")

	app, err := e.Apps.Find(name)
	if err != nil {
		var unknown *capability.UnknownAppError
		if errors.As(err, &unknown) {
			// Not a scriptable application. It may still be one the user has
			// open in a browser tab — most of what people want to drive is.
			if e.canDrivePages() {
				if tree, err := e.PageScreen.Capture(ctx, e.pageBrowser(ctx)); err == nil && pageBelongsTo("https://"+tree.App, name) {
					plan, err := e.driveOnPage(ctx, theTask(r), shape, r.Literals, name)
					if err == nil {
						e.logf("%s is the page in front; working through it there", name)
						return plan, plan.SynthesisCalls, nil
					}
					e.logf("could not work through the page: %v", err)
				}
			}
			if plan, calls, pageErr := e.actOnPage(ctx, r, shape, name); pageErr == nil {
				e.logf("%s has no scripting dictionary; acting on the page instead", name)
				return plan, calls, nil
			}
			if e.canDrive() {
				window := name
				if installed, err := e.Apps.FindInstalled(name); err == nil {
					window = installed.Name
				}
				plan, driveErr := e.driveOn(ctx, window, theTask(r), shape, r.Literals)
				if driveErr == nil {
					e.logf("working through it in %s's window, a step at a time", name)
					return plan, plan.SynthesisCalls, nil
				}
				var cannot *synth.UnwritableError
				if errors.As(driveErr, &cannot) {
					e.logf("%s cannot do that, so it is not offered: %s", window, cannot.Why)
					return nil, 1, driveErr
				}
				e.logf("could not work through %s's window a step at a time: %v", name, driveErr)
			}
			if e.canActOnUI() {
				plan, calls, uiErr := e.actOnUI(ctx, r, shape, name)
				if uiErr == nil {
					e.logf("%s declares nothing and is not a page; acting through its window in one go", name)
					return plan, calls, nil
				}
				e.logf("could not act through %s's window: %v", name, uiErr)
				if installed, err := e.Apps.FindInstalled(name); err == nil {
					var closed *uiact.NotRunningError
					if errors.As(uiErr, &closed) {
						e.logf("%s is installed but not open, and VANS does not open applications", installed.Name)
						return nil, 0, &uiact.NotRunningError{App: installed.Name}
					}
					e.logf("%s is installed, so its website is not used in its place", installed.Name)
					return nil, 0, uiErr
				}
				if e.canDrivePages() {
					if plan, err := e.driveOnPage(ctx, theTask(r), shape, r.Literals, name); err == nil {
						e.logf("%s has no window here; working through its site instead", name)
						return plan, plan.SynthesisCalls, nil
					}
				}
				if plan, calls, webErr := e.actOnWebVersion(ctx, r, shape, name); webErr == nil {
					e.logf("%s publishes no window; driving its web version instead", name)
					return plan, calls, nil
				}
				if errors.Is(uiErr, uiact.ErrNoAssistiveAccess) {
					return nil, 0, uiErr
				}
			}
			return nil, 0, &UnknownAppError{Name: unknown.Name, Near: unknown.Near}
		}
		return nil, 0, err
	}

	dict, err := e.Apps.Dictionary(app)
	if err != nil {
		return nil, 0, fmt.Errorf("engine: %s: %w", app.Name, err)
	}
	surface := capability.Describe(app, dict)

	template, calls, err := e.programFor(ctx, surface, r, shape)
	if err != nil {
		return nil, calls, err
	}

	// Filled here, never before. What is cached is written against the shape;
	// what runs is written against this instruction.
	program := template.Fill(func(v string) string { return route.Fill(v, r.Literals) })
	if err := appact.Validate(program, surface); err != nil {
		return nil, calls, err
	}

	plan := &Plan{
		Task:           Task{Intent: r.Slot("action"), Title: app.Name},
		Destination:    app.Name,
		SynthesisCalls: calls,
		FromCache:      calls == 0,
		Action: &ActionPlan{
			App:     app.Name,
			Preview: appact.Preview(program, surface),
			Writes:  appact.Writes(program, surface),
			program: program,
			surface: surface,
		},
	}
	return plan, calls, nil
}

// programFor returns a program for this action shape, from the cache if one is
// there and by asking the model if not.
//
// The cache is keyed by the shape rather than by the instruction, so "add milk
// to Groceries" and "add bread to Groceries" are one program with different
// values — which is the whole claim: a cold run costs a call, and every warm
// run on the same shape costs none.
func (e *Engine) programFor(ctx context.Context, s capability.Surface, r route.Route, shape string) (appact.Program, int, error) {
	key := s.App + "\x00" + shape

	if cached, found, err := e.Actions.Action(ctx, s.App, shape); err != nil {
		e.logf("action cache unreadable: %v", err)
	} else if found {
		// Revalidated on the way out. A cached program was valid against the
		// dictionary the day it was written, and an application update can
		// remove a command or make a property read-only — which would
		// otherwise surface as a failed Apple event after a confirmation.
		if err := appact.Validate(cached, s); err == nil {
			e.logf("action cache hit for %s", key)
			return cached, 0, nil
		}
		e.logf("cached action no longer fits %s; writing it again", s.App)
		if err := e.Actions.ForgetAction(ctx, s.App, shape); err != nil {
			e.logf("could not drop the stale action: %v", err)
		}
	}

	var failures []string
	budget := e.maxSynthesis()

	for attempt := 1; attempt <= budget; attempt++ {
		program, err := e.Writer.WriteAction(ctx, synth.AppActRequest{
			Action:       templateOr(r, "action"),
			Target:       templateOr(r, "target"),
			Surface:      s,
			Placeholders: route.PlaceholdersIn(shape),
			Failures:     failures,
		})
		if err != nil {
			return appact.Program{}, attempt, err
		}

		var rejected []string
		if err := appact.Validate(program, s); err != nil {
			var invalid *appact.InvalidError
			if errors.As(err, &invalid) {
				rejected = invalid.Reasons
			} else {
				return appact.Program{}, attempt, err
			}
		}
		for _, baked := range appact.Baked(program, r.Literals) {
			rejected = append(rejected, fmt.Sprintf(
				"it writes %q into the program instead of the placeholder standing for it; "+
					"this program is reused for every request of this shape, so that value "+
					"would be used for all of them", baked))
		}
		if len(rejected) > 0 {
			e.logf("program rejected: %s", strings.Join(rejected, "; "))
			failures = rejected
			continue
		}

		if err := e.Actions.SaveAction(ctx, s.App, shape, program, e.Model); err != nil {
			e.logf("could not cache the action: %v", err)
		}
		return program, attempt, nil
	}

	return appact.Program{}, budget, &UnroutableError{
		Instruction: r.Slot("action"),
		Why: fmt.Sprintf("no program for %s survived checking against what it declares it can do",
			s.App),
		Can: e.capabilities(),
	}
}

// templateOr prefers the unfilled slot. A route that arrived already resolved
// carries no templates, and for that path the filled value is the only thing
// there — it is also not cached, so nothing is baked in by using it.
func templateOr(r route.Route, name string) string {
	if tmpl := r.Template(name); tmpl != "" {
		return tmpl
	}
	return r.Slot(name)
}

// runAction is the side effect. Reached only through Commit.
func (e *Engine) runAction(ctx context.Context, plan *Plan) (Receipt, error) {
	if e.Runner == nil {
		return Receipt{}, fmt.Errorf("engine: this build cannot run application programs")
	}
	a := plan.Action

	receipt, err := appact.Run(ctx, e.Runner, a.program, a.surface)
	if err != nil {
		return Receipt{}, err
	}

	title := receipt.Name
	if title == "" {
		title = a.App
	}
	e.logf("ran %d step(s) in %s, verified=%v", len(a.program.Steps), a.App, receipt.Verified)
	return Receipt{ID: receipt.ID, Title: title, Verified: receipt.Verified}, nil
}
