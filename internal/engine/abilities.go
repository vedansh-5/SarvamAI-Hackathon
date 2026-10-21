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
	trialsFresh  = 5 * time.Minute
	fetchRetry   = time.Minute
	fetchWait    = 2 * time.Second
)

type kept[T any] struct {
	fetch func(ctx context.Context) (T, error)
	fresh time.Duration
	mu    sync.Mutex
	got   T
	at    time.Time
	tried time.Time
}

func (k *kept[T]) get(ctx context.Context) T {
	k.mu.Lock()
	defer k.mu.Unlock()
	now := time.Now()
	if (!k.at.IsZero() && now.Sub(k.at) < k.fresh) || now.Sub(k.tried) < fetchRetry {
		return k.got
	}
	k.tried = now
	ctx, cancel := context.WithTimeout(ctx, fetchWait)
	defer cancel()
	got, err := k.fetch(ctx)
	if err != nil {
		return k.got
	}
	k.got, k.at = got, now
	return got
}

type cachedContent struct{ k *kept[synth.Content] }

func (c cachedContent) Content(ctx context.Context) (synth.Content, error) { return c.k.get(ctx), nil }

func Cached(from ContentSource) ContentSource {
	if from == nil {
		return nil
	}
	return cachedContent{&kept[synth.Content]{fetch: from.Content, fresh: contentFresh}}
}

type TrialSource interface {
	Trials(ctx context.Context) (synth.Trials, error)
}

type cachedTrials struct{ k *kept[synth.Trials] }

func (c cachedTrials) Trials(ctx context.Context) (synth.Trials, error) { return c.k.get(ctx), nil }

func CachedTrials(from TrialSource) TrialSource {
	if from == nil {
		return nil
	}
	return cachedTrials{&kept[synth.Trials]{fetch: from.Trials, fresh: trialsFresh}}
}

func (e *Engine) TrialsNow(ctx context.Context) synth.Trials {
	if e.Trials == nil {
		return synth.TrialsFor(nil)
	}
	got, err := e.Trials.Trials(ctx)
	if err != nil {
		return synth.TrialsFor(nil)
	}
	return synth.TrialsFor(got)
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
		synth.AbilityVoice:    true,
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
