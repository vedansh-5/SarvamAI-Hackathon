package engine

import (
	"context"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/pagemark"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/synth"
)

type Ability struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Says     string   `json:"says"`
	Examples []string `json:"examples,omitempty"`
}

type ContentSource interface {
	Content(ctx context.Context) (synth.Content, error)
}

const (
	contentFresh = 15 * time.Minute
	contentRetry = time.Minute
	contentWait  = 2 * time.Second
)

type cachedContent struct {
	from  ContentSource
	mu    sync.Mutex
	got   synth.Content
	at    time.Time
	tried time.Time
}

func Cached(from ContentSource) ContentSource {
	if from == nil {
		return nil
	}
	return &cachedContent{from: from}
}

func (c *cachedContent) Content(ctx context.Context) (synth.Content, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if (!c.at.IsZero() && now.Sub(c.at) < contentFresh) || now.Sub(c.tried) < contentRetry {
		return c.got, nil
	}
	c.tried = now
	ctx, cancel := context.WithTimeout(ctx, contentWait)
	defer cancel()
	got, err := c.from.Content(ctx)
	if err != nil {
		return c.got, nil
	}
	c.got, c.at = got, now
	return got, nil
}

func (e *Engine) wording(ctx context.Context) synth.Content {
	if e.Content == nil {
		return synth.Content{}
	}
	got, err := e.Content.Content(ctx)
	if err != nil {
		return synth.Content{}
	}
	return got
}

func (e *Engine) Welcome(ctx context.Context) synth.Welcome {
	return e.wording(ctx).WelcomeOver(synth.BuiltInWelcome())
}

func (e *Engine) Abilities(ctx context.Context) []Ability {
	_, marks := e.pageAsker()
	can := map[string]bool{
		synth.AbilityGuide:    e.canRun(route.KindAppGuide),
		synth.AbilityDo:       e.canRun(route.KindAppAction),
		synth.AbilityRead:     e.canRun(route.KindScreenRead),
		synth.AbilityPage:     e.canRun(route.KindPageHere),
		synth.AbilityMarks:    marks || e.canRun(route.KindPageStyle),
		synth.AbilitySearch:   e.canRun(route.KindWebSearch),
		synth.AbilityOnText:   e.canRun(route.KindText),
		synth.AbilityFiles:    e.canRun(route.KindFiles),
		synth.AbilitySetting:  e.canRun(route.KindSetting),
		synth.AbilityMemory:   e.Memory != nil,
		synth.AbilityRoutines: e.Routines != nil,
		synth.AbilityLater:    e.Schedules != nil,
		synth.AbilityLearning: e.Clicks != nil,
	}
	var base []synth.AbilityText
	builtIn := map[string][]string{}
	for _, a := range synth.BuiltInAbilities() {
		if can[a.ID] {
			base = append(base, a)
			builtIn[a.ID] = a.Examples
		}
	}
	var out []Ability
	for _, a := range e.wording(ctx).AbilitiesOver(base) {
		if a.ID == synth.AbilityMarks && !allUnderstood(a.Examples) {
			a.Examples = builtIn[a.ID]
		}
		out = append(out, Ability{ID: a.ID, Title: a.Title, Says: a.Says, Examples: a.Examples})
	}
	return out
}

func allUnderstood(examples []string) bool {
	for _, example := range examples {
		if _, ok := pagemark.Understand(example); !ok {
			return false
		}
	}
	return true
}
