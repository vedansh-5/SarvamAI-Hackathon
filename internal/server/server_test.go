package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type standIn struct {
	synth.Model
	plans  int
	failed error
}

func (s *standIn) WritePlan(_ context.Context, req synth.PlanRequest) (synth.PlanAnswer, error) {
	s.plans++
	if s.failed != nil {
		return synth.PlanAnswer{}, s.failed
	}
	return synth.PlanAnswer{Steps: []uiact.Step{{Do: "press", Describe: "Open " + req.Task}}}, nil
}

func (s *standIn) Ping(context.Context) error { return nil }

const riya = "riya-code-0123456789"

func serving(t *testing.T, model *standIn, testers []Tester, total int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(&Server{
		Model:  func(*synth.Usage) synth.Model { return model },
		Ledger: NewMemory(testers),
		Total:  total,
		Now:    func() time.Time { return time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC) },
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestAQuestionGoesThroughTheServer(t *testing.T) {
	model := &standIn{}
	srv := serving(t, model, []Tester{{Name: "Riya", Code: riya, Daily: 10}}, 0)

	got, err := synth.NewRemote(srv.URL, riya).WritePlan(context.Background(), synth.PlanRequest{Task: "Settings"})
	if err != nil {
		t.Fatalf("WritePlan: %v", err)
	}
	if len(got.Steps) != 1 || got.Steps[0].Describe != "Open Settings" {
		t.Errorf("answer = %+v", got)
	}
}

func TestAnUnknownCodeIsTurnedDown(t *testing.T) {
	model := &standIn{}
	srv := serving(t, model, []Tester{{Name: "Riya", Code: riya, Daily: 10}}, 0)

	for _, code := range []string{"", "not-a-code-at-all"} {
		_, err := synth.NewRemote(srv.URL, code).WritePlan(context.Background(), synth.PlanRequest{})
		if !errors.Is(err, synth.ErrRefused) {
			t.Errorf("code %q: err = %v, want refused", code, err)
		}
	}
	if model.plans != 0 {
		t.Errorf("the model was asked %d times", model.plans)
	}
}

func TestASwitchedOffCodeIsTurnedDown(t *testing.T) {
	srv := serving(t, &standIn{}, []Tester{{Name: "Riya", Code: riya, Daily: 10, Off: true}}, 0)

	_, err := synth.NewRemote(srv.URL, riya).WritePlan(context.Background(), synth.PlanRequest{})
	if !errors.Is(err, synth.ErrRefused) {
		t.Errorf("err = %v, want refused", err)
	}
}

func TestATesterStopsAtTheirDailyAllowance(t *testing.T) {
	model := &standIn{}
	srv := serving(t, model, []Tester{{Name: "Riya", Code: riya, Daily: 2}}, 0)
	remote := synth.NewRemote(srv.URL, riya)

	for i := 0; i < 2; i++ {
		if _, err := remote.WritePlan(context.Background(), synth.PlanRequest{}); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	if _, err := remote.WritePlan(context.Background(), synth.PlanRequest{}); !errors.Is(err, synth.ErrOverCap) {
		t.Errorf("third request: err = %v, want over the allowance", err)
	}
	if model.plans != 2 {
		t.Errorf("the model was asked %d times, want 2", model.plans)
	}
}

func TestEveryoneStopsAtTheTotal(t *testing.T) {
	other := "arjun-code-0123456789"
	srv := serving(t, &standIn{}, []Tester{{Name: "Riya", Code: riya, Daily: 10}, {Name: "Arjun", Code: other, Daily: 10}}, 1)

	if _, err := synth.NewRemote(srv.URL, riya).WritePlan(context.Background(), synth.PlanRequest{}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := synth.NewRemote(srv.URL, other).WritePlan(context.Background(), synth.PlanRequest{}); !errors.Is(err, synth.ErrOverCap) {
		t.Errorf("second tester: err = %v, want over the total", err)
	}
}

func TestATypedFailureArrivesTyped(t *testing.T) {
	srv := serving(t, &standIn{failed: &synth.UnwritableError{App: "Notes", Why: "no command"}}, []Tester{{Name: "Riya", Code: riya, Daily: 10}}, 0)

	_, err := synth.NewRemote(srv.URL, riya).WritePlan(context.Background(), synth.PlanRequest{})
	var unwritable *synth.UnwritableError
	if !errors.As(err, &unwritable) || unwritable.App != "Notes" {
		t.Errorf("err = %v, want unwritable for Notes", err)
	}
}

func TestAPingIsNotCounted(t *testing.T) {
	srv := serving(t, &standIn{}, []Tester{{Name: "Riya", Code: riya, Daily: 1}}, 0)
	remote := synth.NewRemote(srv.URL, riya)

	for i := 0; i < 3; i++ {
		if err := remote.Ping(context.Background()); err != nil {
			t.Fatalf("ping %d: %v", i+1, err)
		}
	}
	if _, err := remote.WritePlan(context.Background(), synth.PlanRequest{}); err != nil {
		t.Errorf("after pings: %v", err)
	}
}

func TestHealthAnswersWithoutACode(t *testing.T) {
	srv := serving(t, &standIn{}, nil, 0)
	resp, err := http.Get(srv.URL + HealthPath)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health = %d", resp.StatusCode)
	}
}

func TestTestersAreReadFromOneLine(t *testing.T) {
	got, err := ParseTesters("Riya:riya-code-0123456789:30; Arjun:arjun-code-0123456789:20")
	if err != nil || len(got) != 2 || got[1].Name != "Arjun" || got[1].Daily != 20 {
		t.Errorf("got %+v, %v", got, err)
	}
	for _, bad := range []string{"Riya:short:30", "Riya:riya-code-0123456789", "Riya:riya-code-0123456789:x", "A:same-code-0123456789:1;B:same-code-0123456789:1"} {
		if _, err := ParseTesters(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}
