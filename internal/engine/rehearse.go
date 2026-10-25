package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

var asksForARehearsal = regexp.MustCompile(`(?i)[\s,:\x{2014}-]*\b(as a dry run|dry run|dry-run|rehearse|rehearsal|without (actually )?doing (it|anything)|do ?n[o\x{2019}']t (actually )?do (it|anything)( yet)?|what would you do (to|if i asked you to)|just (tell|show) me (the|your) (plan|steps)( to| for)?)\b[\s,:\x{2014}-]*`)

func withoutTheRehearsal(instruction string) (string, bool) {
	if !asksForARehearsal.MatchString(instruction) {
		return instruction, false
	}
	rest := strings.Trim(asksForARehearsal.ReplaceAllString(instruction, " "), " ,.;:\u2014-")
	rest = strings.Join(strings.Fields(rest), " ")
	if rest == "" {
		return instruction, false
	}
	return rest, true
}

func (e *Engine) rehearse(ctx context.Context, in Ask) (*Plan, error) {
	const title = "Dry run \u2014 nothing was done"

	plan, err := e.ask(ctx, in)
	if err != nil {
		var no *NotAllowedError
		if errors.As(err, &no) {
			return saying(title, fmt.Sprintf("Your rules would refuse this in %s: %s.", no.Where, no.Rule)), nil
		}
		return nil, err
	}
	if err := e.allowedToAct(ctx, plan); err != nil {
		var no *NotAllowedError
		if errors.As(err, &no) {
			return saying(title, fmt.Sprintf("Your rules would refuse this in %s: %s.", no.Where, no.Rule)), nil
		}
	}

	return e.rehearsalOf(ctx, plan), nil
}

func (e *Engine) rehearsalOf(ctx context.Context, plan *Plan) *Plan {
	const title = "Dry run \u2014 nothing was done"

	if plan.Drive == nil {
		what := strings.TrimSpace(previewOf(plan))
		if what == "" {
			what = "Nothing would be changed."
		}
		return saying(title, "Vspk would:\n\n"+what)
	}

	d := plan.Drive
	var b strings.Builder
	where := d.App
	if d.Open != "" {
		fmt.Fprintf(&b, "Vspk would open %s, then on that page:\n\n", hostOf(d.Open))
	} else {
		fmt.Fprintf(&b, "In %s, Vspk would:\n\n", where)
	}

	steps := d.Replay
	fromMemory := d.Kept
	if !fromMemory && e.Plans != nil {
		tree := uiact.Tree{App: d.App}
		screen := e.UIScreen
		if d.Web {
			screen = e.PageScreen
		}
		if d.Open == "" && screen != nil {
			if seen, err := screen.Capture(ctx, d.App); err == nil {
				tree = seen
			}
		}
		if whole, _ := e.planOn(ctx, synth.PlanRequest{Task: d.Task, Tree: tree, Most: MaxWebSteps, Rehearse: true}); len(whole.Steps) > 0 {
			steps = whole.Steps
		}
	}

	if len(steps) == 0 {
		b.WriteString("  (the steps cannot be told without starting; it would work them out one at a time)\n")
	}
	stops := 0
	for i, step := range steps {
		line := strings.TrimSpace(step.Describe)
		if line == "" {
			line = uiact.Preview(uiact.Program{App: d.App, Steps: []uiact.Step{step}})
		}
		fmt.Fprintf(&b, "  %d. %s", i+1, line)
		if final, _ := uiact.Commits(step, uiact.Element{Role: step.Target.Role, Title: step.Target.Title}); final {
			b.WriteString("   \u2190 would stop and ask you first")
			stops++
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	switch {
	case fromMemory:
		b.WriteString("These are the steps that worked last time.")
	case len(steps) > 1:
		b.WriteString("Only the first step has been checked against the screen. The rest are what Vspk expects, and each is checked when its turn comes.")
	}
	if limit := e.rules(ctx).SpendLimit; limit > 0 {
		fmt.Fprintf(&b, "\nA step that pays more than %s would not be taken.", plainAmount(limit))
	}
	return saying(title, strings.TrimSpace(b.String()))
}
