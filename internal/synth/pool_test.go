package synth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type provider struct {
	asked  atomic.Int32
	status int
	body   string
	answer string
}

func (p *provider) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.asked.Add(1)
		if p.status != 0 && p.status != http.StatusOK {
			w.WriteHeader(p.status)
			_, _ = w.Write([]byte(p.body))
			return
		}
		answerWith(t, w, p.answer)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const settingsChoice = `{"app":"System Settings","why":"appearance","nowhere":false}`

func seat(t *testing.T, name string, p *provider, tier int, takes ...Class) Seat {
	t.Helper()
	m := map[Class]bool{}
	for _, c := range takes {
		m[c] = true
	}
	return Seat{
		Provider: name,
		Client:   &OpenAICompatible{BaseURL: p.serve(t).URL, APIKey: "k", Model: name + "-model", SchemaMode: SchemaObject, Retries: -1},
		Takes:    m,
		Tier:     tier,
	}
}

func choose(t *testing.T, pool *Pool) error {
	t.Helper()
	_, err := pool.With(&Usage{}).WhichApp(context.Background(), AppChoiceRequest{Task: "dark mode", Installed: []string{"System Settings"}})
	return err
}

func TestAQuickQuestionGoesToTheFastest(t *testing.T) {
	slow, fast := &provider{answer: settingsChoice}, &provider{answer: settingsChoice}
	pool := &Pool{Seats: []Seat{seat(t, "slow", slow, 1, Quick), seat(t, "fast", fast, 2, Quick)}}
	pool.answered(pool.Seats[0], Quick, 9*time.Second)
	pool.answered(pool.Seats[1], Quick, time.Second)

	if err := choose(t, pool); err != nil {
		t.Fatal(err)
	}
	if slow.asked.Load() != 0 || fast.asked.Load() != 1 {
		t.Errorf("slow asked %d, fast asked %d; want the fast one", slow.asked.Load(), fast.asked.Load())
	}
}

func TestWritingGoesToTheMostAccurateTierFirst(t *testing.T) {
	pool := &Pool{Seats: []Seat{
		{Provider: "quick-but-weaker", Client: &OpenAICompatible{Model: "a"}, Takes: map[Class]bool{Write: true}, Tier: 2},
		{Provider: "accurate", Client: &OpenAICompatible{Model: "b"}, Takes: map[Class]bool{Write: true}, Tier: 1},
	}}
	pool.answered(pool.Seats[0], Write, time.Second)
	pool.answered(pool.Seats[1], Write, 9*time.Second)

	order := pool.Order(Write)
	if len(order) != 2 || order[0].Provider != "accurate" {
		t.Errorf("order = %v, want the tier-1 seat first however slow", order)
	}
}

func TestABusyProviderIsSkippedUntilItIsLikelyBack(t *testing.T) {
	busy := &provider{status: http.StatusServiceUnavailable, body: `{"error":{"message":"overloaded"}}`}
	spare := &provider{answer: settingsChoice}
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	pool := &Pool{Seats: []Seat{seat(t, "busy", busy, 1, Quick), seat(t, "spare", spare, 1, Quick)}, Now: func() time.Time { return now }}
	pool.answered(pool.Seats[0], Quick, time.Second)
	pool.answered(pool.Seats[1], Quick, 2*time.Second)

	if err := choose(t, pool); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := choose(t, pool); err != nil {
		t.Fatalf("second: %v", err)
	}
	if busy.asked.Load() != 1 {
		t.Errorf("the busy provider was asked %d times, want once and then left to rest", busy.asked.Load())
	}
	if spare.asked.Load() != 2 {
		t.Errorf("the spare was asked %d times, want 2", spare.asked.Load())
	}

	now = now.Add(busyRest + time.Second)
	if order := pool.Order(Quick); len(order) != 2 {
		t.Errorf("after its rest, order = %v, want both back", order)
	}
}

func TestAProviderOutOfItsDailyAllowanceRestsUntilTheDayTurns(t *testing.T) {
	spent := &provider{status: http.StatusTooManyRequests, body: `{"error":{"message":"Rate limit reached on requests per day (RPD): Limit 1000"}}`}
	spare := &provider{answer: settingsChoice}
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	pool := &Pool{Seats: []Seat{seat(t, "spent", spent, 1, Quick), seat(t, "spare", spare, 1, Quick)}, Now: func() time.Time { return now }}
	pool.answered(pool.Seats[0], Quick, time.Second)

	if err := choose(t, pool); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Hour)
	if order := pool.Order(Quick); len(order) != 1 || order[0].Provider != "spare" {
		t.Errorf("ten hours later, order = %v, want the spent one still resting", order)
	}
	now = time.Date(2026, 10, 15, 0, 0, 1, 0, time.UTC)
	if order := pool.Order(Quick); len(order) != 2 {
		t.Errorf("after midnight UTC, order = %v, want it back", order)
	}
}

func TestARefusedRequestIsNotSentOn(t *testing.T) {
	refusing := &provider{status: http.StatusBadRequest, body: `{"error":{"message":"bad"}}`}
	spare := &provider{answer: settingsChoice}
	pool := &Pool{Seats: []Seat{seat(t, "refusing", refusing, 1, Quick), seat(t, "spare", spare, 1, Quick)}}
	pool.answered(pool.Seats[0], Quick, time.Second)

	if err := choose(t, pool); err == nil {
		t.Fatal("want the refusal")
	}
	if spare.asked.Load() != 0 {
		t.Error("a request the first provider refused as wrong was sent to another")
	}
}

func TestLookingGoesOnlyToSeatsThatTakePictures(t *testing.T) {
	pool := &Pool{Seats: []Seat{{Provider: "text-only", Client: &OpenAICompatible{Model: "a"}, Takes: map[Class]bool{Quick: true, Write: true}}}}
	_, err := pool.With(&Usage{}).SeeScreen(context.Background(), SeeRequest{App: "Resolve", JPEG: []byte{1}})
	if !errors.Is(err, ErrNoSeat) {
		t.Errorf("err = %v, want no seat", err)
	}
}

func TestTheKindsOfQuestionAreSorted(t *testing.T) {
	for kind, want := range map[string]Class{
		KindRoute: Quick, KindWhichApp: Quick, KindSee: See,
		KindPlan: Write, KindStep: Write, KindText: Write, KindRead: Write, KindAnswer: Write,
	} {
		if got := ClassOf(kind); got != want {
			t.Errorf("%s is %s, want %s", kind, got, want)
		}
	}
}

func TestProvidersAreReadFromTheEnvironment(t *testing.T) {
	env := map[string]string{
		EnvProviders:                  "gemini, groq",
		"GEMINI_KEY":                  "g-key",
		"VANS_PROVIDER_GEMINI_URL":    "https://g.example/v1/",
		"VANS_PROVIDER_GEMINI_KEY":    "$GEMINI_KEY",
		"VANS_PROVIDER_GEMINI_MODELS": "flash-lite, flash",
		"VANS_PROVIDER_GEMINI_SCHEMA": "json_schema",
		"VANS_PROVIDER_GEMINI_TAKES":  "quick,write,see",
		"VANS_PROVIDER_GROQ_URL":      "https://q.example/v1",
		"VANS_PROVIDER_GROQ_KEY":      "q-key",
		"VANS_PROVIDER_GROQ_MODELS":   "oss",
		"VANS_PROVIDER_GROQ_TIER":     "2",
	}
	pool, err := PoolFrom(func(k string) string { return env[k] }, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pool.Seats) != 3 {
		t.Fatalf("seats = %d, want 3", len(pool.Seats))
	}
	g := pool.Seats[0]
	if g.Client.APIKey != "g-key" || g.Client.BaseURL != "https://g.example/v1" || g.Client.SchemaMode != SchemaJSON || !g.Takes[See] {
		t.Errorf("gemini seat = %+v", g)
	}
	q := pool.Seats[2]
	if q.Tier != 2 || q.Client.SchemaMode != SchemaObject || !q.Takes[Quick] || !q.Takes[Write] || q.Takes[See] {
		t.Errorf("groq seat = %+v (tier %d)", q.Client, q.Tier)
	}

	delete(env, "VANS_PROVIDER_GROQ_MODELS")
	if _, err := PoolFrom(func(k string) string { return env[k] }, 0); err == nil {
		t.Error("a provider with no models was accepted")
	}
	if p, err := PoolFrom(func(string) string { return "" }, 0); p != nil || err != nil {
		t.Errorf("no providers = %v, %v; want nothing", p, err)
	}
}
