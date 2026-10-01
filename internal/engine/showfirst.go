package engine

import (
	"context"
	"errors"
	"strings"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
)

func (e *Engine) showInsteadOfDoing(ctx context.Context, in Ask, res route.Result) (*Plan, error) {
	task := in.Instruction
	shape, literals := res.Shape, res.Route.Literals

	if res.Route.Kind == route.KindPageAction {
		if !e.canDrivePages() {
			return e.planGuideFor(ctx, task, shape, literals, in.Context)
		}
		return e.showOnThePage(ctx, task, shape, literals, res.Route.Slot("site"))
	}

	name := strings.TrimSpace(res.Route.Slot("app"))
	if name == "" || e.Apps == nil {
		return e.planGuideFor(ctx, task, shape, literals, in.Context)
	}

	installed, err := e.Apps.FindInstalled(name)
	if err != nil {
		if _, known := siteAddress(name); known && e.canDrivePages() {
			return e.showOnThePage(ctx, task, shape, literals, name)
		}
		return e.planGuideFor(ctx, task, shape, literals, in.Context)
	}

	var closed *uiact.NotRunningError
	if _, err := e.UIScreen.Capture(ctx, installed.Name); errors.As(err, &closed) {
		return e.planGuideIn(ctx, task, shape, literals, "", &AppNotOpenError{App: installed.Name, Task: task})
	}
	return e.planGuideIn(ctx, task, shape, literals, installed.Name, nil)
}

func (e *Engine) showOnThePage(ctx context.Context, task, shape string, literals map[string]string, site string) (*Plan, error) {
	browser := e.pageBrowser(ctx)
	plan, err := e.planWebGuide(ctx, task, shape, literals, browser, false, "", site)
	if errors.Is(err, appleevents.ErrNotRunning) {
		return e.planWebGuide(ctx, task, shape, literals, browser, true, "", site)
	}
	return plan, err
}

func (e *Engine) doInsteadOfShowing(ctx context.Context, in Ask, res route.Result) (*Plan, error) {
	task := in.Instruction
	shape, literals := res.Shape, res.Route.Literals

	app, err := e.appForTask(ctx, task, in.Context)
	var notOpen *AppNotOpenError
	if errors.As(err, &notOpen) {
		return nil, &uiact.NotRunningError{App: notOpen.App}
	}
	if err != nil {
		return nil, err
	}

	if e.canDrivePages() && strings.EqualFold(app, e.pageBrowser(ctx)) {
		site, _, _ := siteIn(task)
		return e.driveOnPage(ctx, task, shape, literals, site)
	}
	return e.driveOn(ctx, app, task, shape, literals)
}
