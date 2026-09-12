package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

type AppSink struct {
	Screen uiact.Screen

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
	if s.Screen == nil {
		return Receipt{}, fmt.Errorf("engine: this build cannot type into another application")
	}

	app, err := s.Screen.Frontmost(ctx)
	if err != nil {
		return Receipt{}, err
	}
	if strings.EqualFold(strings.TrimSpace(app), s.self()) {
		return Receipt{}, fmt.Errorf(
			"engine: VANS is the application in front, so there is nothing behind it to type into")
	}

	tree, err := s.Screen.Capture(ctx, app)
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

	if err := s.Screen.Act(ctx, program, tree); err != nil {
		return Receipt{}, err
	}
	return Receipt{Title: app, Verified: false}, nil
}
