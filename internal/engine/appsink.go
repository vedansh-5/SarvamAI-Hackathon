package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

type AppSink struct {
	Runner uiact.Runner

	Self string
}

func (AppSink) Destination() Destination {
	return Destination{
		Name:         "app",
		Describe:     "Type it into the application in front, where the cursor is.",
		TouchesWorld: true,
	}
}

func (s AppSink) self() string {
	if s.Self != "" {
		return s.Self
	}
	return "VANS"
}

func (s AppSink) Write(ctx context.Context, doc Document) (Receipt, error) {
	if s.Runner == nil {
		return Receipt{}, fmt.Errorf("engine: this build cannot type into another application")
	}

	app, err := uiact.Frontmost(ctx, s.Runner)
	if err != nil {
		return Receipt{}, err
	}
	if strings.EqualFold(strings.TrimSpace(app), s.self()) {
		return Receipt{}, fmt.Errorf(
			"engine: VANS is the application in front, so there is nothing behind it to type into")
	}

	tree, err := uiact.Capture(ctx, s.Runner, app)
	if err != nil {
		return Receipt{}, err
	}

	target, why := tree.TypingTarget()
	if why != "" {
		return Receipt{}, fmt.Errorf("engine: %s: %s", app, why)
	}

	program := uiact.Program{App: tree.App, Steps: []uiact.Step{{
		Do:       "type",
		Target:   tree.SelectorFor(target),
		Text:     doc.Text(),
		Describe: fmt.Sprintf("Type the answer into the %s in %s", target.Role, app),
	}}}

	receipt, err := uiact.Run(ctx, s.Runner, program, tree)
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{Title: app, Verified: receipt.Verified}, nil
}
