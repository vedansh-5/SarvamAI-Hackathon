package synth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
	"github.com/vedansh/vans/internal/webact"
)

type Class string

const (
	Quick Class = "quick"
	Write Class = "write"
	See   Class = "see"
)

func ClassOf(kind string) Class {
	switch kind {
	case KindWhichApp, KindPing:
		return Quick
	case KindSee:
		return See
	}
	return Write
}

type Seat struct {
	Provider string
	Client   *OpenAICompatible
	Takes    map[Class]bool
	Tier     int
}

func (s Seat) Name() string { return s.Provider + "/" + s.Client.Model }

const (
	busyRest     = 30 * time.Second
	silentRest   = 60 * time.Second
	missingRest  = time.Hour
	unknownSpeed = 3 * time.Second
	speedWeight  = 0.3
)

type seatState struct {
	speed   map[Class]time.Duration
	resting time.Time
	why     string
}

type Pool struct {
	Seats []Seat
	Now   func() time.Time

	mu    sync.Mutex
	state map[string]*seatState
}

func (p *Pool) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Pool) stateOf(name string) *seatState {
	if p.state == nil {
		p.state = map[string]*seatState{}
	}
	st := p.state[name]
	if st == nil {
		st = &seatState{speed: map[Class]time.Duration{}}
		p.state[name] = st
	}
	return st
}

func (p *Pool) Order(class Class) []Seat {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	type ranked struct {
		seat  Seat
		speed time.Duration
	}
	var awake []ranked
	for _, s := range p.Seats {
		if !s.Takes[class] {
			continue
		}
		st := p.stateOf(s.Name())
		if now.Before(st.resting) {
			continue
		}
		speed := st.speed[class]
		if speed == 0 {
			speed = unknownSpeed
		}
		awake = append(awake, ranked{s, speed})
	}
	sort.SliceStable(awake, func(i, j int) bool {
		if class == Write && awake[i].seat.Tier != awake[j].seat.Tier {
			return awake[i].seat.Tier < awake[j].seat.Tier
		}
		return awake[i].speed < awake[j].speed
	})
	out := make([]Seat, len(awake))
	for i, r := range awake {
		out[i] = r.seat
	}
	return out
}

func (p *Pool) answered(s Seat, class Class, took time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.stateOf(s.Name())
	if old := st.speed[class]; old > 0 {
		took = time.Duration(float64(old)*(1-speedWeight) + float64(took)*speedWeight)
	}
	st.speed[class] = took
}

func (p *Pool) failed(s Seat, err error) {
	rest, why := restFor(err, p.now())
	if rest <= 0 {
		return
	}
	p.mu.Lock()
	st := p.stateOf(s.Name())
	st.resting, st.why = p.now().Add(rest), why
	p.mu.Unlock()
	Notice("%s rests for %s: %s", s.Name(), readableWait(rest), why)
}

func restFor(err error, now time.Time) (time.Duration, string) {
	var busy *transientError
	if errors.As(err, &busy) {
		if busy.status == http.StatusTooManyRequests && spentForTheDay(busy.body) {
			midnight := now.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
			return midnight.Sub(now), "its allowance for the day is used up"
		}
		if busy.retryAfter > 0 {
			return busy.retryAfter, fmt.Sprintf("busy (status %d)", busy.status)
		}
		return busyRest, fmt.Sprintf("busy (status %d)", busy.status)
	}
	if errors.Is(err, ErrNoAnswer) {
		return silentRest, "it did not answer"
	}
	var refused *providerRefusal
	if errors.As(err, &refused) && refused.status == http.StatusNotFound {
		return missingRest, "it does not have this model"
	}
	return 0, ""
}

func spentForTheDay(body string) bool {
	b := strings.ToLower(body)
	return strings.Contains(b, "per day") || strings.Contains(b, "perday") || strings.Contains(b, "daily")
}

var ErrNoSeat = errors.New("synth: no model is free to answer this right now")

type poolView struct {
	pool *Pool
	used *Usage
}

func (p *Pool) With(used *Usage) Model { return &poolView{pool: p, used: used} }

func through[A any](ctx context.Context, v *poolView, kind string, ask func(c *OpenAICompatible) (A, error)) (A, error) {
	var none A
	class := ClassOf(kind)
	seats := v.pool.Order(class)
	if len(seats) == 0 {
		return none, ErrNoSeat
	}
	var failures []error
	for _, s := range seats {
		c := *s.Client
		c.Usage = v.used
		started := time.Now()
		got, err := ask(&c)
		if err == nil {
			took := time.Since(started)
			v.pool.answered(s, class, took)
			Notice("%s answered %s in %s", s.Name(), kind, readableWait(took))
			return got, nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", s.Name(), err))
		v.pool.failed(s, err)
		if ctx.Err() != nil || !anotherModelMightAnswer(err) {
			break
		}
	}
	return none, errors.Join(failures...)
}

func (v *poolView) Synthesize(ctx context.Context, req Request) (extract.Program, error) {
	return through(ctx, v, KindProgram, func(c *OpenAICompatible) (extract.Program, error) { return c.Synthesize(ctx, req) })
}

func (v *poolView) Decide(ctx context.Context, req route.Request) (route.Decision, error) {
	return through(ctx, v, KindRoute, func(c *OpenAICompatible) (route.Decision, error) { return c.Decide(ctx, req) })
}

func (v *poolView) Answer(ctx context.Context, req AnswerRequest) (answer.Answer, error) {
	return through(ctx, v, KindAnswer, func(c *OpenAICompatible) (answer.Answer, error) { return c.Answer(ctx, req) })
}

func (v *poolView) WriteAction(ctx context.Context, req AppActRequest) (appact.Program, error) {
	return through(ctx, v, KindAppAction, func(c *OpenAICompatible) (appact.Program, error) { return c.WriteAction(ctx, req) })
}

func (v *poolView) WriteWebAction(ctx context.Context, req WebActRequest) (webact.Program, error) {
	return through(ctx, v, KindWebAction, func(c *OpenAICompatible) (webact.Program, error) { return c.WriteWebAction(ctx, req) })
}

func (v *poolView) WriteUIAction(ctx context.Context, req UIActRequest) (uiact.Program, error) {
	return through(ctx, v, KindUIAction, func(c *OpenAICompatible) (uiact.Program, error) { return c.WriteUIAction(ctx, req) })
}

func (v *poolView) WriteStep(ctx context.Context, req StepRequest) (StepAnswer, error) {
	return through(ctx, v, KindStep, func(c *OpenAICompatible) (StepAnswer, error) { return c.WriteStep(ctx, req) })
}

func (v *poolView) WritePlan(ctx context.Context, req PlanRequest) (PlanAnswer, error) {
	return through(ctx, v, KindPlan, func(c *OpenAICompatible) (PlanAnswer, error) { return c.WritePlan(ctx, req) })
}

func (v *poolView) WriteStyle(ctx context.Context, req StyleRequest) (StyleAnswer, error) {
	return through(ctx, v, KindStyle, func(c *OpenAICompatible) (StyleAnswer, error) { return c.WriteStyle(ctx, req) })
}

func (v *poolView) WhichApp(ctx context.Context, req AppChoiceRequest) (AppChoice, error) {
	return through(ctx, v, KindWhichApp, func(c *OpenAICompatible) (AppChoice, error) { return c.WhichApp(ctx, req) })
}

func (v *poolView) WorkOnText(ctx context.Context, req TextRequest) (TextAnswer, error) {
	return through(ctx, v, KindText, func(c *OpenAICompatible) (TextAnswer, error) { return c.WorkOnText(ctx, req) })
}

func (v *poolView) WriteFileOps(ctx context.Context, req FilesRequest) (FilesAnswer, error) {
	return through(ctx, v, KindFiles, func(c *OpenAICompatible) (FilesAnswer, error) { return c.WriteFileOps(ctx, req) })
}

func (v *poolView) SeeScreen(ctx context.Context, req SeeRequest) (SeeAnswer, error) {
	return through(ctx, v, KindSee, func(c *OpenAICompatible) (SeeAnswer, error) { return c.SeeScreen(ctx, req) })
}

func (v *poolView) ReadScreen(ctx context.Context, req ReadRequest) (ReadAnswer, error) {
	return through(ctx, v, KindRead, func(c *OpenAICompatible) (ReadAnswer, error) { return c.ReadScreen(ctx, req) })
}

func (v *poolView) Ping(ctx context.Context) error {
	_, err := through(ctx, v, KindPing, func(c *OpenAICompatible) (struct{}, error) { return struct{}{}, c.Ping(ctx) })
	return err
}

const EnvProviders = "VANS_PROVIDERS"

func PoolFrom(env func(string) string, wait time.Duration) (*Pool, error) {
	names := strings.Split(env(EnvProviders), ",")
	pool := &Pool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		upper := strings.ToUpper(name)
		get := func(field string) string {
			v := strings.TrimSpace(env("VANS_PROVIDER_" + upper + "_" + field))
			if strings.HasPrefix(v, "$") {
				v = strings.TrimSpace(env(strings.TrimPrefix(v, "$")))
			}
			return v
		}
		base, key := get("URL"), get("KEY")
		if base == "" || key == "" {
			return nil, fmt.Errorf("synth: provider %s needs VANS_PROVIDER_%s_URL and _KEY", name, upper)
		}
		mode := SchemaObject
		if spelled := get("SCHEMA"); spelled != "" {
			m, err := ParseSchemaMode(spelled)
			if err != nil {
				return nil, fmt.Errorf("synth: provider %s: %w", name, err)
			}
			mode = m
		}
		takes := map[Class]bool{}
		for _, c := range strings.Split(get("TAKES"), ",") {
			switch Class(strings.TrimSpace(c)) {
			case Quick, Write, See:
				takes[Class(strings.TrimSpace(c))] = true
			case "":
			default:
				return nil, fmt.Errorf("synth: provider %s takes %q, want quick, write or see", name, c)
			}
		}
		if len(takes) == 0 {
			takes = map[Class]bool{Quick: true, Write: true}
		}
		tier := 1
		if spelled := get("TIER"); spelled != "" {
			t, err := strconv.Atoi(spelled)
			if err != nil || t < 1 {
				return nil, fmt.Errorf("synth: provider %s: tier must be 1 or more", name)
			}
			tier = t
		}
		models := strings.Split(get("MODELS"), ",")
		added := 0
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			pool.Seats = append(pool.Seats, Seat{
				Provider: name,
				Client: &OpenAICompatible{
					BaseURL: strings.TrimSuffix(base, "/"), APIKey: key, Model: model,
					SchemaMode: mode, AnswerWait: wait, Retries: -1,
				},
				Takes: takes,
				Tier:  tier,
			})
			added++
		}
		if added == 0 {
			return nil, fmt.Errorf("synth: provider %s names no models in VANS_PROVIDER_%s_MODELS", name, upper)
		}
	}
	if len(pool.Seats) == 0 {
		return nil, nil
	}
	return pool, nil
}
