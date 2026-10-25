package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/names"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

type Rulebook interface {
	Rules(ctx context.Context) (store.Rules, error)
}

type NotAllowedError struct {
	Where string
	Rule  string
}

func (e *NotAllowedError) Error() string {
	return fmt.Sprintf("engine: not allowed in %s: %s", e.Where, e.Rule)
}

func (e *Engine) rules(ctx context.Context) store.Rules {
	if e.Rulebook == nil {
		return store.Rules{}
	}
	r, err := e.Rulebook.Rules(ctx)
	if err != nil {
		e.logf("could not read your rules, so none are applied: %v", err)
		return store.Rules{}
	}
	return r
}

func forbidden(r store.Rules, places ...string) (string, bool) {
	for _, place := range places {
		place = strings.TrimSpace(place)
		if place == "" {
			continue
		}
		for _, never := range r.Never {
			if names.Same(never, place) || sameSite(place, never) {
				return place, true
			}
		}
	}
	return "", false
}

func sameSite(place, never string) bool {
	host := strings.ToLower(strings.TrimSpace(place))
	rule := strings.ToLower(strings.TrimSpace(never))
	rule = strings.TrimPrefix(strings.TrimPrefix(rule, "https://"), "http://")
	rule = strings.TrimSuffix(strings.TrimPrefix(rule, "www."), "/")
	if rule == "" || strings.ContainsAny(host, " /") || !strings.Contains(host, ".") {
		return false
	}
	if strings.Contains(rule, ".") {
		return host == rule || strings.HasSuffix(host, "."+rule)
	}
	for _, label := range strings.Split(host, ".") {
		if label == rule {
			return true
		}
	}
	return false
}

func (e *Engine) asksFirstAt(ctx context.Context, places ...string) string {
	r := e.rules(ctx)
	if len(r.AskFirst) == 0 {
		return ""
	}
	where, yes := forbidden(store.Rules{Never: r.AskFirst}, places...)
	if !yes {
		return ""
	}
	return "your rules say to ask before every step in " + where
}

func placesOf(plan *Plan) []string {
	switch {
	case plan.Drive != nil:
		return []string{plan.Drive.App, plan.Drive.Browser, hostOf(plan.Drive.Open)}
	case plan.UI != nil:
		return []string{plan.UI.App}
	case plan.Files != nil:
		return []string{whereFilesLive, "Files"}
	case plan.Setting != nil:
		return []string{"System Settings", "Settings"}
	case plan.Action != nil:
		return []string{plan.Action.App}
	case plan.Web != nil:
		return []string{hostOf(plan.Web.URL)}
	}
	return nil
}

func (e *Engine) allowedToAct(ctx context.Context, plan *Plan) error {
	if plan == nil {
		return nil
	}
	places := placesOf(plan)
	if len(places) == 0 {
		return nil
	}
	if where, no := forbidden(e.rules(ctx), places...); no {
		return &NotAllowedError{Where: where, Rule: "you have told Vspk never to act there. It can still show you how"}
	}
	return nil
}

func (e *Engine) overTheLimit(ctx context.Context, tree uiact.Tree, step uiact.Step) error {
	limit := e.rules(ctx).SpendLimit
	if limit <= 0 {
		return nil
	}
	el, why := tree.Resolve(step.Target)
	if why != "" || !uiact.Spends(step, el) {
		return nil
	}
	most := 0.0
	for _, amount := range uiact.Amounts(tree) {
		if amount > most {
			most = amount
		}
	}
	if most <= limit {
		return nil
	}
	return &NotAllowedError{
		Where: tree.App,
		Rule:  fmt.Sprintf("the screen shows %s, which is over your spending limit of %s, so this step was not taken", plainAmount(most), plainAmount(limit)),
	}
}

func plainAmount(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf("%d", int64(v))
	}
	return fmt.Sprintf("%.2f", v)
}
