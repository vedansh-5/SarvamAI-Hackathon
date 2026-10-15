package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/pagemark"
	"github.com/vedansh/vans/internal/screen"
	"github.com/vedansh/vans/internal/theme"
	"github.com/vedansh/vans/internal/webstyle"
)

const (
	ThemeKey  = "theme.accent"
	marksOnIn = "marks.on"
)

type PrefStore interface {
	Setting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

type PageAsker interface {
	FrontBrowser(ctx context.Context) (string, bool)
	Handles(app string) bool
	AskPage(ctx context.Context, app, script string, out any) error
}

func (e *Engine) Theme(ctx context.Context) theme.Theme {
	if e.Prefs == nil {
		return theme.Default()
	}
	id, _ := e.Prefs.Setting(ctx, ThemeKey)
	return theme.Named(id)
}

func (e *Engine) SetTheme(ctx context.Context, id string) error {
	if !theme.Known(id) {
		return fmt.Errorf("engine: %q is not one of VANS's colours", id)
	}
	if e.Prefs == nil {
		return nil
	}
	return e.Prefs.SetSetting(ctx, ThemeKey, strings.ToLower(strings.TrimSpace(id)))
}

func (e *Engine) pageAsker() (PageAsker, bool) {
	a, ok := e.PageScreen.(PageAsker)
	return a, ok
}

func (e *Engine) browserInFront(ctx context.Context, seen *screen.Context) (string, bool) {
	asker, ok := e.pageAsker()
	if !ok {
		return "", false
	}
	if seen != nil && strings.TrimSpace(seen.AppName) != "" {
		if asker.Handles(seen.AppName) {
			return seen.AppName, true
		}
		return "", false
	}
	return asker.FrontBrowser(ctx)
}

func (e *Engine) aboutMarks(ctx context.Context, in Ask) (*Plan, bool, error) {
	if asksToClear(in.Instruction) && e.canStyle() {
		if err := e.ClearStyle(ctx); err != nil {
			return nil, true, err
		}
		return saying("Highlighting off", "Took the highlighting off every page VANS put it on."), true, nil
	}
	match, ok := pagemark.Understand(in.Instruction)
	if !ok {
		return nil, false, nil
	}
	browser, ok := e.browserInFront(ctx, in.Context)
	if !ok {
		return nil, false, nil
	}
	asker, _ := e.pageAsker()
	script, err := pagemark.Script(match, e.Theme(ctx))
	if err != nil {
		return nil, true, err
	}
	var got pagemark.Result
	if err := asker.AskPage(ctx, browser, script, &got); err != nil {
		return nil, true, fmt.Errorf("engine: mark the page in %s: %w", browser, err)
	}
	if !got.OK {
		return nil, true, fmt.Errorf("engine: the page in %s could not be marked: %s", browser, got.Error)
	}
	if got.Matched == 0 {
		return saying("Nothing to mark", fmt.Sprintf("Nothing on this page seems to %s.", match.Says())), true, nil
	}
	if !got.Painted {
		return nil, true, fmt.Errorf("engine: %d matched, but the page in %s would not show the marks", got.Matched, browser)
	}
	e.noteMarks(ctx, browser)
	e.logf("marked %d of %d on the page in %s", got.Matched, got.Alike, browser)

	said := fmt.Sprintf("%d that %s.", got.Matched, match.Says())
	if got.Alike > got.Matched {
		said = fmt.Sprintf("%d of %d that %s.", got.Matched, got.Alike, match.Says())
	}
	if got.Scrolled {
		said += " Scrolled to the first."
	}
	plan := saying("Marked on the page", said+" Press Esc on the page, or ask for something else, to take the marks off.")
	plan.Marked = true
	return plan, true, nil
}

func (e *Engine) noteMarks(ctx context.Context, browser string) {
	if e.Prefs == nil {
		return
	}
	_ = e.Prefs.SetSetting(ctx, marksOnIn, browser)
}

func (e *Engine) TakeMarksOff(ctx context.Context) {
	if e.Prefs == nil {
		return
	}
	browser, _ := e.Prefs.Setting(ctx, marksOnIn)
	if strings.TrimSpace(browser) == "" {
		return
	}
	if driver, ok := e.pageDriver(); ok {
		if cleared, err := e.takeTheStyleOff(ctx, driver); err == nil {
			e.logf("took the marks off %d page(s)", cleared)
		}
	}
	if asker, ok := e.pageAsker(); ok && asker.Handles(browser) {
		var ignored struct{}
		_ = asker.AskPage(ctx, browser, webstyle.Clear(), &ignored)
	}
	_ = e.Prefs.SetSetting(ctx, marksOnIn, "")
}
