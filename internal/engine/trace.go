package engine

import (
	"context"

	"github.com/vedansh/vans/internal/uiact"
)

// TraceStore keeps the step sequences of drive runs that worked. Implemented by
// store; an interface here so a build without one simply learns nothing rather
// than refusing to act.
type TraceStore interface {
	SaveDriveTrace(ctx context.Context, app, shape string, p uiact.Program, pinned map[string]string, model string) error
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
