package engine

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/uiact"
)

var saysNo = regexp.MustCompile(`(?i)^(reject( all)?( cookies)?|decline( all)?( cookies)?|deny( all)?|refuse( all)?|(use |accept |allow )?(only )?(strictly )?(necessary|essential|required)( cookies)?( only)?|continue without accepting|no,? thanks?|disagree)$`)

func wayToSayNo(tree uiact.Tree) (uiact.Element, bool) {
	for _, el := range tree.Elements {
		if !el.Notice || !el.Enabled || !el.Pressable() {
			continue
		}
		if saysNo.MatchString(strings.TrimSpace(el.Label())) {
			return el, true
		}
	}
	return uiact.Element{}, false
}

func (e *Engine) closeTheNotice(ctx context.Context, app string) (string, bool) {
	tree, err := e.UIScreen.Capture(ctx, app)
	if err != nil || len(tree.Elements) == 0 {
		return "", false
	}
	el, ok := wayToSayNo(tree)
	if !ok {
		return "", false
	}

	name := strings.TrimSpace(el.Label())
	step := uiact.Step{Do: "click", Target: tree.SelectorFor(el), Describe: fmt.Sprintf("Close the cookie notice with %q", name)}
	program := uiact.Program{App: tree.App, Steps: []uiact.Step{step}}
	if err := uiact.Validate(program, tree); err != nil {
		return "", false
	}
	if err := e.showBeforeActing(ctx, tree, step, 0, step.Describe); err != nil {
		return "", false
	}
	if err := e.UIScreen.Act(ctx, program, tree); err != nil {
		e.logf("could not close the cookie notice: %v", err)
		return "", false
	}
	e.logf("closed the cookie notice with %q, without asking the model", name)
	return fmt.Sprintf("Closed the cookie notice by clicking %q \u2014 only what the site needs was allowed", name), true
}
