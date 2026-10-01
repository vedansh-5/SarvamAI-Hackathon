package engine

import (
	"context"

	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
)

// TraceStore keeps the step sequences of drive runs that worked. Implemented by
// store; an interface here so a build without one simply learns nothing rather
// than refusing to act.
type TraceStore interface {
	DriveTrace(ctx context.Context, app, shape string, literals map[string]string) (uiact.Program, bool, error)
	SaveDriveTrace(ctx context.Context, app, shape string, p uiact.Program, pinned map[string]string, model string) error
	DriveTraceWorked(ctx context.Context, app, shape string) error
	DriveTraceDiverged(ctx context.Context, app, shape string) (bool, error)
}

// stepsFromLastTime is the sequence this task took before, filled with this
// request's values, or nothing when there is none to use.
//
// A miss is the ordinary case and never an error: the run simply costs what it
// costs today. Nothing here is allowed to stop a task going ahead, because a
// broken cache making VANS refuse to work would be worse than a slow VANS.
func (e *Engine) stepsFromLastTime(ctx context.Context, app, shape string, literals map[string]string) []uiact.Step {
	if e.Traces == nil || shape == "" {
		return nil
	}

	stored, found, err := e.Traces.DriveTrace(ctx, app, shape, literals)
	if err != nil {
		e.logf("could not read back the steps kept for %s: %v", app, err)
		return nil
	}
	if !found || len(stored.Steps) == 0 {
		return nil
	}

	filled := stored.Fill(func(v string) string { return route.Fill(v, literals) })
	return filled.Steps
}

// settleTheTrace records what became of a run: keeping the steps of one that
// worked, and marking a stored sequence as having held up or gone off.
//
// Called on the failure path as well as the success one. A replay that went off
// and then could not be repaired is exactly the case the divergence count exists
// to notice, and only counting the runs that finished would mean never noticing a
// trace that reliably breaks.
func (e *Engine) settleTheTrace(ctx context.Context, d *DrivePlan, run taskRun) {
	if e.Traces == nil || d.Shape == "" || d.Resumed {
		return
	}

	// A plan written for this run is not in the store, so there is nothing there
	// to mark as having held up or gone off. The run is judged as any first run
	// is, and kept if it earns it.
	if !d.Kept {
		e.rememberTheRun(ctx, d, run)
		return
	}

	if !run.Diverged {
		// It held up. The count is cleared, because it exists to find a trace
		// that is reliably wrong rather than one that was once unlucky.
		if err := e.Traces.DriveTraceWorked(ctx, d.App, d.Shape); err != nil {
			e.logf("could not record that the kept steps worked: %v", err)
		}
		return
	}

	dropped, err := e.Traces.DriveTraceDiverged(ctx, d.App, d.Shape)
	if err != nil {
		e.logf("could not record that the kept steps went off: %v", err)
	}
	if dropped {
		e.logf("the steps kept for %q went off twice running, so they are forgotten", d.Shape)
	}

	// Whatever the run turned into is the newest thing known to work, so it
	// replaces what was there — including re-creating an entry just dropped,
	// which is the right answer: a sequence that finished beats one that did not.
	e.rememberTheRun(ctx, d, run)
}

// rememberTheRun keeps the steps of a run that got the task done, so the next
// request of the same shape costs no model calls.
//
// Four things have to hold, and each of them is a way for a stored trace to be
// a lie rather than a shortcut:
//
//   - the model said the task was done, rather than the run hitting the step
//     limit — otherwise the sequence is a prefix of the task, not the task;
//   - every step was seen to change the window, so the sequence is known to have
//     worked and not merely to have been sent (D-004);
//   - the values from this instruction lift back out into placeholders, so the
//     trace answers for the shape rather than for one request;
//   - no step types words the model supplied itself, because replaying those
//     would put this run's words into a different one. Words from the instruction
//     that were never values are fine: two requests of the same shape differ only
//     in their values, so those words are the same for every request that can hit
//     this key.
//
// A run that fails any of them is not kept, and the log says which — a cache that
// silently never fills is indistinguishable from one that is not wired up.
//
// Never fails the run. The task has already been carried out by the time this
// runs; refusing to report a change that was made because learning from it failed
// would be the worst of both.
func (e *Engine) rememberTheRun(ctx context.Context, d *DrivePlan, run taskRun) {
	if e.Traces == nil || d.Shape == "" {
		return
	}

	if !run.Finished {
		e.logf("not keeping these steps: the run stopped before the task was done")
		return
	}
	if !run.Landed {
		e.logf("not keeping these steps: not every one of them changed the window")
		return
	}
	if run.Asked > 0 {
		e.logf("not keeping these steps: they depend on an answer that was asked for, and the next time may want a different one")
		return
	}

	templated := uiact.Templatize(uiact.Program{App: d.App, Steps: run.Did}, d.Task, d.Literals)
	if !templated.Keep() {
		e.logf("not keeping these steps: %s", templated.Why)
		return
	}

	if err := e.Traces.SaveDriveTrace(ctx, d.App, d.Shape, templated.Program, templated.Pinned, e.Model); err != nil {
		e.logf("could not keep these steps: %v", err)
		return
	}

	if len(templated.Pinned) > 0 {
		e.logf("kept these %d step(s) for %q, for these values only", run.Steps(), d.Shape)
		return
	}
	e.logf("kept these %d step(s) for %q", run.Steps(), d.Shape)
}
