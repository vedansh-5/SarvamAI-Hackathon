package synth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// heldOpen serves a handler that is allowed to never answer.
//
// The release channel is not a convenience: a client that gives up closes its
// end, and the server does not learn of it until it tries to write, so a handler
// waiting only on the request context would keep httptest.Server.Close waiting
// forever. Cleanups run in reverse order of registration, which is why the
// release is registered after the close.
func heldOpen(t *testing.T, handle func(http.ResponseWriter, *http.Request, <-chan struct{})) *httptest.Server {
	t.Helper()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handle(w, r, release)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	return srv
}

// silentServer takes the request and never answers it, which is what an
// overloaded model did on 2026-09-22.
func silentServer(t *testing.T, seen *atomic.Int32) *httptest.Server {
	t.Helper()
	return heldOpen(t, func(_ http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		if seen != nil {
			seen.Add(1)
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
}

func answerWith(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{
			{"message": map[string]string{"content": body}, "finish_reason": "stop"},
		},
	}); err != nil {
		t.Errorf("encode reply: %v", err)
	}
}

func modelAsked(t *testing.T, r *http.Request) string {
	t.Helper()
	var body chatRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Errorf("decode request: %v", err)
		return ""
	}
	return body.Model
}

// The failure this exists for: waiting out the old three minutes is
// indistinguishable from the app having hung.
func TestGivesUpOnASilentProvider(t *testing.T) {
	c := clientFor(silentServer(t, nil))
	c.AnswerWait = 150 * time.Millisecond

	start := time.Now()
	_, err := c.Synthesize(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("want an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("waited %v — the answer timeout did not apply", elapsed)
	}
	if !errors.Is(err, ErrNoAnswer) {
		t.Errorf("not reported as an unanswered request: %v", err)
	}
	if !strings.Contains(err.Error(), "test/model") {
		t.Errorf("the error does not name the model: %v", err)
	}
}

// Silence is not a transient failure: retrying spends the whole wait again for
// nothing.
func TestDoesNotRetrySilence(t *testing.T) {
	var calls atomic.Int32

	c := clientFor(silentServer(t, &calls))
	c.AnswerWait = 100 * time.Millisecond

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want an error")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("server saw %d calls, want 1", n)
	}
}

// Someone pressing Stop is not the model going quiet, and blaming the model for
// it sends them looking on the wrong side of the network.
func TestCancellationIsNotBlamedOnTheModel(t *testing.T) {
	c := clientFor(silentServer(t, nil))
	c.AnswerWait = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Synthesize(ctx, sampleRequest())
	if err == nil {
		t.Fatal("want an error")
	}
	if errors.Is(err, ErrNoAnswer) {
		t.Errorf("the caller giving up was reported as the model not answering: %v", err)
	}
}

// One model overloaded while another on the same key answers in a second and a
// half is measured, not hypothetical (2026-09-22).
func TestFallsBackWhenTheFirstModelIsSilent(t *testing.T) {
	var asked []string

	srv := heldOpen(t, func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		model := modelAsked(t, r)
		asked = append(asked, model)
		if model == "test/model" {
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		answerWith(t, w, goodProgramJSON)
	})

	c := clientFor(srv)
	c.AnswerWait = 150 * time.Millisecond
	c.Fallback = "test/second"

	got, err := c.Synthesize(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got.Root == "" {
		t.Error("empty program from the fallback model")
	}
	if len(asked) != 2 || asked[0] != "test/model" || asked[1] != "test/second" {
		t.Errorf("models asked = %v, want the first then the fallback", asked)
	}
}

// A model name the provider does not have is the other way a run dies with a
// working key, and a second name is exactly the fix for it.
func TestFallsBackWhenTheModelDoesNotExist(t *testing.T) {
	var asked []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := modelAsked(t, r)
		asked = append(asked, model)
		if model == "test/model" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
			return
		}
		answerWith(t, w, goodProgramJSON)
	}))
	t.Cleanup(srv.Close)

	c := clientFor(srv)
	c.Fallback = "test/second"

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(asked) != 2 || asked[1] != "test/second" {
		t.Errorf("models asked = %v, want the fallback tried", asked)
	}
}

// A 400 is our own request being wrong. Every model rejects it the same way, so
// spending the fallback on it only doubles the wait before the same failure.
func TestDoesNotSpendTheFallbackOnABadRequest(t *testing.T) {
	srv, calls := flakyServer(t, 99, http.StatusBadRequest, "")

	c := fastRetryClient(srv)
	c.Fallback = "test/second"

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want an error")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("server saw %d calls, want 1", n)
	}
}

func TestFallbackNamingThePrimaryIsNotTriedTwice(t *testing.T) {
	srv, calls := flakyServer(t, 99, http.StatusNotFound, "")

	c := fastRetryClient(srv)
	c.Fallback = "test/model"

	if _, err := c.Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want an error")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("server saw %d calls, want 1 — the fallback names the same model", n)
	}
}

func TestParseAnswerWait(t *testing.T) {
	for _, in := range []string{"45s", "2m", "500ms"} {
		if _, err := parseAnswerWait(in); err != nil {
			t.Errorf("parseAnswerWait(%q): %v", in, err)
		}
	}
	if got, err := parseAnswerWait("  "); err != nil || got != 0 {
		t.Errorf("empty = (%v, %v), want (0, nil) so the default applies", got, err)
	}
	for _, in := range []string{"45", "soon", "0s", "-5s"} {
		if _, err := parseAnswerWait(in); err == nil {
			t.Errorf("parseAnswerWait(%q) accepted a value it cannot honour", in)
		}
	}
}
