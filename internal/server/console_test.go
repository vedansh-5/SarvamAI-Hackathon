package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

const consoleKey = "console-key-for-tests"

func consoleServer(t *testing.T) (*httptest.Server, *Memory, *http.Client) {
	t.Helper()
	now := func() time.Time { return time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC) }
	ledger := NewMemory([]Tester{{Name: "Riya", Code: riya, Daily: 10}})
	srv := httptest.NewTLSServer(&Server{
		Model:   func(*synth.Usage) synth.Model { return &standIn{} },
		Ledger:  ledger,
		Now:     now,
		Console: &Console{Books: ledger, Key: consoleKey, Now: now, Limits: map[string]int{"groq": 1000}},
	})
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Jar, _ = cookiejar.New(nil)
	return srv, ledger, client
}

func page(t *testing.T, client *http.Client, method, target string, form url.Values) (int, string) {
	t.Helper()
	var resp *http.Response
	var err error
	if method == http.MethodPost {
		resp, err = client.PostForm(target, form)
	} else {
		resp, err = client.Get(target)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func signIn(t *testing.T, srv *httptest.Server, client *http.Client) {
	t.Helper()
	if status, _ := page(t, client, http.MethodPost, srv.URL+"/console/login", url.Values{"key": {consoleKey}}); status != http.StatusOK {
		t.Fatalf("signing in answered %d", status)
	}
}

func TestTheConsoleIsShutWithoutTheKey(t *testing.T) {
	srv, ledger, client := consoleServer(t)

	for _, path := range []string{"/console", "/console/testers", "/console/activity", "/console/feedback"} {
		status, body := page(t, client, http.MethodGet, srv.URL+path, nil)
		if status != http.StatusUnauthorized || strings.Contains(body, riya) || !strings.Contains(body, "Console key") {
			t.Errorf("%s answered %d without the key", path, status)
		}
	}
	if status, _ := page(t, client, http.MethodPost, srv.URL+"/console/testers/off", url.Values{"code": {riya}}); status != http.StatusUnauthorized {
		t.Errorf("switching a code off without the key answered %d", status)
	}
	if got, _, _ := ledger.Tester(context.Background(), riya); got.Off {
		t.Error("a code was switched off without the key")
	}
	if status, body := page(t, client, http.MethodPost, srv.URL+"/console/login", url.Values{"key": {"wrong"}}); status != http.StatusUnauthorized || !strings.Contains(body, "not the console key") {
		t.Errorf("a wrong key answered %d", status)
	}
}

func TestTheConsoleRunsTheTesters(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	signIn(t, srv, client)
	ctx := context.Background()

	_, body := page(t, client, http.MethodPost, srv.URL+"/console/testers/add", url.Values{"name": {"Arjun"}, "daily": {"50"}})
	code := regexp.MustCompile(`vans-[0-9a-f]{20}`).FindString(body)
	if code == "" || !strings.Contains(body, "Arjun can use VANS") {
		t.Fatalf("no code shown after adding a tester:\n%s", body)
	}
	if got, found, _ := ledger.Tester(ctx, code); !found || got.Name != "Arjun" || got.Daily != 50 {
		t.Errorf("tester = %+v, %v", got, found)
	}

	page(t, client, http.MethodPost, srv.URL+"/console/testers/daily", url.Values{"code": {code}, "daily": {"80"}})
	page(t, client, http.MethodPost, srv.URL+"/console/testers/off", url.Values{"code": {code}})
	if got, _, _ := ledger.Tester(ctx, code); got.Daily != 80 || !got.Off {
		t.Errorf("after the changes, tester = %+v", got)
	}
	page(t, client, http.MethodPost, srv.URL+"/console/testers/on", url.Values{"code": {code}})
	if got, _, _ := ledger.Tester(ctx, code); got.Off {
		t.Error("switching back on did not take")
	}
	if status, _ := page(t, client, http.MethodPost, srv.URL+"/console/testers/add", url.Values{"name": {""}, "daily": {"5"}}); status != http.StatusOK {
		t.Errorf("a tester with no name answered %d", status)
	}
}

func TestTheConsoleShowsWhatTestersAskedAndSent(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	remote := synth.NewRemote(srv.URL, riya)
	remote.HTTP = srv.Client()
	ctx := context.Background()

	if _, err := remote.WritePlan(ctx, synth.PlanRequest{Task: "turn on dark mode"}); err != nil {
		t.Fatal(err)
	}
	if _, err := remote.WhichApp(ctx, synth.AppChoiceRequest{Task: "dark mode", InFront: "Safari"}); err != nil {
		t.Fatal(err)
	}
	if err := remote.SendFeedback(ctx, synth.Feedback{Kind: synth.FeedbackNote, Text: "The pointer was late."}); err != nil {
		t.Fatal(err)
	}
	if err := remote.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	rows, _ := ledger.Activity(ctx, "2026-10-14")
	if len(rows) != 2 {
		t.Fatalf("recorded = %+v; want the plan and the choice, not the feedback or the ping", rows)
	}
	if plan := rows[1]; plan.Words != "turn on dark mode" || plan.Kind != synth.KindPlan || plan.From != "Riya" || plan.Fault != "" {
		t.Errorf("plan recorded as %+v", plan)
	}
	if choice := rows[0]; choice.App != "System Settings" {
		t.Errorf("the choice was recorded against %q, want the application chosen, not the one in front", choice.App)
	}

	signIn(t, srv, client)
	for path, want := range map[string]string{
		"/console":                        "requests today",
		"/console/activity":               "turn on dark mode",
		"/console/activity?failed=1":      "Nothing matches",
		"/console/activity?tester=Nobody": "Nothing matches",
		"/console/feedback":               "The pointer was late.",
		"/console/testers":                riya,
	} {
		if status, body := page(t, client, http.MethodGet, srv.URL+path, nil); status != http.StatusOK || !strings.Contains(body, want) {
			t.Errorf("%s answered %d without %q", path, status, want)
		}
	}

	kept := ledger.Kept()
	page(t, client, http.MethodPost, srv.URL+"/console/feedback/handled", url.Values{"id": {kept[0].ID}})
	if !ledger.Kept()[0].Handled {
		t.Error("the feedback was not marked handled")
	}
}

func TestOnlyTheTypedWordsAndTheApplicationAreKept(t *testing.T) {
	for raw, want := range map[string][2]string{
		`{"Instruction":"what does the build line say","Shape":"what does the {x}"}`:      {"what does the build line say", ""},
		`{"Task":"dark mode","InFront":"Safari","Installed":["Safari"]}`:                  {"dark mode", "Safari"},
		`{"task":"crop","app":"Photos","tree":"secret window contents"}`:                  {"crop", "Photos"},
		`{"Ask":"make it shorter","Text":"a private letter the person selected"}`:         {"make it shorter", ""},
		`{"Question":"what is the total","App":"Numbers","Text":"the numbers on screen"}`: {"what is the total", "Numbers"},
		`not json`: {"", ""},
	} {
		words, app := said(json.RawMessage(raw))
		if words != want[0] || app != want[1] {
			t.Errorf("%s: kept %q, %q; want %q, %q", raw, words, app, want[0], want[1])
		}
	}
	long := strings.Repeat("a", MostWords+50)
	if words, _ := said(json.RawMessage(`{"Task":"` + long + `"}`)); len([]rune(words)) != MostWords+1 {
		t.Errorf("a long request kept %d characters", len([]rune(words)))
	}
}
