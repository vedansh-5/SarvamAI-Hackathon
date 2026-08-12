package synth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// flakyServer fails the first failures requests with status, then succeeds.
func flakyServer(t *testing.T, failures int, status int, retryAfter string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if int(calls.Add(1)) <= failures {
			if retryAfter != "" {
				w.Header().Set("Retry-After", retryAfter)
			}
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"rate-limited upstream"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": goodProgramJSON}, "finish_reason": "stop"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	return srv, &calls
}

func fastRetryClient(srv *httptest.Server) *OpenAICompatible {
	c := clientFor(srv)
	c.RetryWait = time.Millisecond
	return c
}

// A 429 is transient by definition. Free tiers return them constantly, and
// failing the task on the first one would make the runtime unusable on them.
func TestRetriesRateLimit(t *testing.T) {
	srv, calls := flakyServer(t, 2, http.StatusTooManyRequests, "")

	got, err := fastRetryClient(srv).Synthesize(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if got.Root == "" {
		t.Error("empty program after retry")
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("server saw %d calls, want 3", n)
	}
}

func TestRetriesServerErrors(t *testing.T) {
	srv, calls := flakyServer(t, 1, http.StatusBadGateway, "")

	if _, err := fastRetryClient(srv).Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("server saw %d calls, want 2", n)
	}
}

// A 400 means the request itself is wrong. Retrying only burns quota.
func TestDoesNotRetryClientErrors(t *testing.T) {
	srv, calls := flakyServer(t, 5, http.StatusBadRequest, "")

	if _, err := fastRetryClient(srv).Synthesize(context.Background(), sampleRequest()); err == nil {
		t.Fatal("want an error")
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("server saw %d calls, want 1 — a 400 must not be retried", n)
	}
}

func TestGivesUpAfterRetryBudget(t *testing.T) {
	srv, calls := flakyServer(t, 99, http.StatusTooManyRequests, "")

	c := fastRetryClient(srv)
	c.Retries = 2

	_, err := c.Synthesize(context.Background(), sampleRequest())
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "gave up after 3 attempts") {
		t.Errorf("error did not report the attempt count: %v", err)
	}
	if n := calls.Load(); n != 3 {
		t.Errorf("server saw %d calls, want 3 (1 + 2 retries)", n)
	}
	// The provider's explanation has to survive the retry wrapper, or the
	// user is told "gave up" with no reason.
	if !strings.Contains(err.Error(), "rate-limited upstream") {
		t.Errorf("error lost the provider's explanation: %v", err)
	}
}

func TestHonorsRetryAfterHeader(t *testing.T) {
	srv, _ := flakyServer(t, 1, http.StatusTooManyRequests, "1")

	c := clientFor(srv)
	c.RetryWait = time.Hour // would never finish if the header were ignored

	start := time.Now()
	if _, err := c.Synthesize(context.Background(), sampleRequest()); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("waited %v — Retry-After was ignored", elapsed)
	}
}

func TestBackoffRespectsContext(t *testing.T) {
	srv, _ := flakyServer(t, 99, http.StatusTooManyRequests, "")

	c := clientFor(srv)
	c.RetryWait = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := c.Synthesize(ctx, sampleRequest()); err == nil {
		t.Fatal("want an error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("backoff ignored context cancellation (waited %v)", elapsed)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{
		"":                              0,
		"0":                             0,
		"5":                             5 * time.Second,
		" 12 ":                          12 * time.Second,
		"-3":                            0,
		"not-a-number":                  0,
		"Wed, 21 Oct 2026 07:28:00 GMT": 0,                // HTTP-date form: fall back to our own backoff
		"99999":                         60 * time.Second, // clamped
	}
	for in, want := range cases {
		if got := parseRetryAfter(in); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}
