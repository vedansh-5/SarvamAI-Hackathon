package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/uiact"
)

type AppMap interface {
	LearnEdge(ctx context.Context, app string, e store.MapEdge) error
	KnownEdges(ctx context.Context, app string) ([]store.MapEdge, error)
}

type TaskLog interface {
	RecordTask(ctx context.Context, r store.Run) error
}

const (
	mostLeadsKept   = 12
	longestMapName  = 60
	mostKnownNearby = 14
	mostKnownBeyond = 10
)

func mapName(el uiact.Element) (string, bool) {
	name := strings.TrimSpace(el.Label())
	if name == "" || strings.Contains(name, "\n") || len([]rune(name)) > longestMapName {
		return "", false
	}
	return name, true
}

func asLead(el uiact.Element) string {
	return fmt.Sprintf("%s %q", el.Role, strings.TrimSpace(el.Label()))
}

func (e *Engine) learnFrom(ctx context.Context, before, after uiact.Tree, step uiact.Step) {
	if e.Map == nil || strings.EqualFold(strings.TrimSpace(step.Do), "press") {
		return
	}
	el, why := before.Resolve(step.Target)
	if why != "" || !uiact.IsControl(el) {
		return
	}
	name, ok := mapName(el)
	if !ok {
		return
	}

	var leads []string
	for _, appeared := range uiact.NewControls(before, after) {
		if _, ok := mapName(appeared); !ok {
			continue
		}
		leads = append(leads, asLead(appeared))
		if len(leads) == mostLeadsKept {
			break
		}
	}
	if len(leads) == 0 {
		return
	}
	if err := e.Map.LearnEdge(ctx, before.App, store.MapEdge{Role: el.Role, Title: name, Leads: leads}); err != nil {
		e.logf("could not note what %s led to: %v", name, err)
	}

	list := before.ListAround(el)
	if list == "" {
		return
	}
	var shared []string
	plain := strings.ToLower(name)
	for _, lead := range leads {
		if !strings.Contains(strings.ToLower(lead), plain) {
			shared = append(shared, lead)
		}
	}
	if len(shared) == 0 {
		return
	}
	if err := e.Map.LearnEdge(ctx, before.App, store.MapEdge{Role: el.Role, Title: anyIn(list), Leads: shared}); err != nil {
		e.logf("could not note what the items in %s lead to: %v", list, err)
	}
}

const anyInPrefix = "any in: "

func anyIn(list string) string { return anyInPrefix + list }

func (e *Engine) knownAbout(ctx context.Context, tree uiact.Tree) []string {
	if e.Map == nil || tree.App == "" {
		return nil
	}
	edges, err := e.Map.KnownEdges(ctx, tree.App)
	if err != nil || len(edges) == 0 {
		return nil
	}

	onScreen := map[string]bool{}
	for _, el := range tree.Elements {
		if uiact.IsControl(el) && el.Label() != "" {
			onScreen[strings.ToLower(asLead(el))] = true
		}
	}

	line := func(edge store.MapEdge) string {
		return fmt.Sprintf("%s %q leads to: %s", edge.Role, edge.Title, strings.Join(edge.Leads, ", "))
	}
	key := func(edge store.MapEdge) string {
		return strings.ToLower(fmt.Sprintf("%s %q", edge.Role, edge.Title))
	}

	var out []string
	reachable := map[string]bool{}
	used := map[string]bool{}

	lists := map[string]string{}
	for i, list := range tree.Lists() {
		if list != "" {
			lists[strings.ToLower(tree.Elements[i].Role+"\x00"+anyIn(list))] = list
		}
	}
	for _, edge := range edges {
		if !strings.HasPrefix(edge.Title, anyInPrefix) {
			continue
		}
		used[key(edge)] = true
		list, here := lists[strings.ToLower(edge.Role+"\x00"+edge.Title)]
		if !here || len(out) >= mostKnownNearby {
			continue
		}
		out = append(out, fmt.Sprintf("on screen now: any %s in %q leads to: %s", edge.Role, list, strings.Join(edge.Leads, ", ")))
		for _, lead := range edge.Leads {
			reachable[strings.ToLower(lead)] = true
		}
	}
	for _, edge := range edges {
		if !onScreen[key(edge)] || len(out) >= mostKnownNearby {
			continue
		}
		used[key(edge)] = true
		out = append(out, "on screen now: "+line(edge))
		for _, lead := range edge.Leads {
			reachable[strings.ToLower(lead)] = true
		}
	}
	beyond := 0
	for _, edge := range edges {
		if used[key(edge)] || !reachable[key(edge)] || beyond >= mostKnownBeyond {
			continue
		}
		beyond++
		out = append(out, "one step further: "+line(edge))
	}
	return out
}

func (e *Engine) noteTheTask(ctx context.Context, app, shape, mode string, run taskRun, planCalls int, failed error) {
	if e.Tasks == nil {
		return
	}
	why := ""
	switch {
	case failed == nil:
	case ctx.Err() != nil:
		why = "stopped by the person"
	default:
		why = failed.Error()
	}
	err := e.Tasks.RecordTask(context.WithoutCancel(ctx), store.Run{
		App: app, Shape: shape, Mode: mode,
		Steps: run.Steps(), Replayed: run.Replayed, Calls: run.Calls + planCalls, Finished: run.Finished,
		Failed: why,
	})
	if err != nil {
		e.logf("could not note what this task cost: %v", err)
	}
}
