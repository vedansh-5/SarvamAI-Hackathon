package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/store"
)

// Ledger keeps what VANS has done. Implemented by store; an interface here so
// the engine can be exercised without one, and so a build without a store simply
// remembers nothing rather than refusing to act.
type Ledger interface {
	RecordDeed(ctx context.Context, d store.Deed) (int64, error)
}

// writeItDown records a committed plan, whatever came of it.
//
// Never fails the commit. The change has already happened by the time this runs,
// and refusing to report a change that was made because the reporting failed
// would be the worst of both.
func (e *Engine) writeItDown(ctx context.Context, plan *Plan, receipt Receipt, failure error) {
	if e.Deeds == nil {
		return
	}

	did := previewOf(plan)
	if plan.Drive != nil {
		steps := receipt.Did
		if len(steps) == 0 {
			steps = plan.Drive.Done
		}
		if len(steps) > 0 {
			var b strings.Builder
			for i, step := range steps {
				fmt.Fprintf(&b, "%d. %s\n", i+1, strings.SplitN(step, " \u2014 ", 2)[0])
			}
			did = strings.TrimRight(b.String(), "\n")
		}
	}

	deed := store.Deed{
		Kind:     kindOf(plan),
		Where:    whereItHappened(plan, receipt),
		Asked:    strings.TrimSpace(plan.Task.Intent),
		Did:      did,
		Verified: failure == nil && receipt.Verified,
		Undo:     undoFor(plan),
	}
	if failure != nil {
		// Kept as part of the record rather than dropped. "It was tried and it
		// did not finish" is a different thing to be told than silence, and it
		// is the case somebody most needs to go and look at.
		deed.Changed = "did not finish: " + failure.Error()
	}

	if _, err := e.Deeds.RecordDeed(context.WithoutCancel(ctx), deed); err != nil {
		e.logf("could not write down what was just done: %v", err)
	}
}

func kindOf(plan *Plan) string {
	switch {
	case plan.Web != nil:
		return "page"
	case plan.Style != nil:
		return "restyle"
	case plan.UI != nil:
		return "window"
	case plan.Drive != nil:
		return "steps"
	case plan.Files != nil:
		return "files"
	case plan.Setting != nil:
		return "setting"
	case plan.Action != nil:
		return "application"
	default:
		return "document"
	}
}

func whereItHappened(plan *Plan, receipt Receipt) string {
	switch {
	case plan.Web != nil:
		return hostOf(plan.Web.URL)
	case plan.Style != nil:
		return hostOf(plan.Style.URL)
	case plan.UI != nil:
		return plan.UI.App
	case plan.Drive != nil:
		return plan.Drive.App
	case plan.Files != nil:
		return whereFilesLive
	case plan.Setting != nil:
		return whereSettingsLive
	case plan.Action != nil:
		return plan.Action.App
	}
	if title := strings.TrimSpace(receipt.Title); title != "" {
		return title
	}
	return strings.TrimSpace(plan.Destination)
}

// The sentence the person agreed to, which is the honest record of what they
// agreed to — not a description written afterwards to fit what happened.
func previewOf(plan *Plan) string {
	switch {
	case plan.Web != nil:
		return plan.Web.Preview
	case plan.Style != nil:
		return plan.Style.Preview
	case plan.UI != nil:
		return plan.UI.Preview
	case plan.Drive != nil:
		return plan.Drive.Preview
	case plan.Files != nil:
		return plan.Files.Preview
	case plan.Setting != nil:
		return plan.Setting.Preview
	case plan.Action != nil:
		return plan.Action.Preview
	default:
		return fmt.Sprintf("Write %d row(s) to %s", len(plan.Rows), plan.Destination)
	}
}

// undoFor is how to put this back, and empty is the honest answer for most of it
// today.
//
// A restyle is the one tier that already has an inverse, and it is a complete
// one: taking the highlighting off puts every page back exactly as it was. The
// rest are one-way for now and say so, which is worth more than a hopeful guess
// — being told a thing cannot be undone before agreeing to it is the whole point
// of recording this.
func undoFor(plan *Plan) string {
	if plan.Style != nil && !plan.Style.Clears {
		return marshalUndo(howToUndo{Kind: undoByClearingTheStyle})
	}
	if plan.Drive != nil && len(plan.Drive.typed) > 0 && !plan.Drive.committed {
		return marshalUndo(howToUndo{Kind: undoByPuttingTextBack, Typed: plan.Drive.typed})
	}
	if plan.Files != nil && len(plan.Files.done) > 0 {
		return marshalUndo(howToUndo{Kind: undoByMovingFilesBack, Files: plan.Files.done})
	}
	if plan.Setting != nil {
		return marshalUndo(howToUndo{Kind: undoBySettingItBack, Setting: plan.Setting.Setting, Was: plan.Setting.Was})
	}
	return ""
}
