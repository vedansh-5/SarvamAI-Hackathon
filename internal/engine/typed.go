package engine

import (
	"context"
	"fmt"

	"github.com/vedansh/vans/internal/uiact"
)

type TypedField struct {
	App     string         `json:"app"`
	Browser string         `json:"browser,omitempty"`
	Target  uiact.Selector `json:"target"`
	Was     string         `json:"was"`
}

const longestTextKept = 8000

type typedSoFar struct {
	fields []TypedField
	seen   map[string]bool
}

func (t *typedSoFar) note(ctx context.Context, e *Engine, tree uiact.Tree, step uiact.Step) {
	if verb := step.Verb(); verb != "type" && verb != "set" {
		return
	}
	keeper, ok := e.UIScreen.(uiact.ValueKeeper)
	if !ok {
		return
	}
	key := tree.App + "\x1f" + uiact.FieldKey(step.Target.Role, step.Target.Title) + fmt.Sprintf("\x1f%d", step.Target.Index)
	if t.seen[key] {
		return
	}
	was, ok := keeper.ValueOf(ctx, tree, step.Target)
	if !ok || len([]rune(was)) > longestTextKept {
		return
	}
	if t.seen == nil {
		t.seen = map[string]bool{}
	}
	t.seen[key] = true
	field := TypedField{App: tree.App, Target: step.Target, Was: was}
	if e.onPage {
		field.Browser = e.pageBrowser(ctx)
	}
	t.fields = append(t.fields, field)
}

func (e *Engine) putTheTextBack(ctx context.Context, fields []TypedField) error {
	if len(fields) == 0 {
		return fmt.Errorf("%w: nothing was kept of what those fields held", ErrCannotBeUndone)
	}
	for i := len(fields) - 1; i >= 0; i-- {
		f := fields[i]
		on, where := e, f.App
		if f.Browser != "" {
			if !e.canDrivePages() {
				return fmt.Errorf("%w: this build cannot reach a page", ErrCannotBeUndone)
			}
			on, where = e.onThePage(), f.Browser
		}
		keeper, ok := on.UIScreen.(uiact.ValueKeeper)
		if !ok {
			return fmt.Errorf("%w: this build cannot write text back into a field", ErrCannotBeUndone)
		}
		if err := keeper.PutBack(ctx, where, f.Target, f.Was); err != nil {
			return fmt.Errorf("engine: could not put back what %s held in %s: %w", f.Target.Title, f.App, err)
		}
	}
	return nil
}
