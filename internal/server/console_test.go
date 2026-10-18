package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/capability"
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
	if !strings.Contains(body, DownloadLink()) {
		t.Error("the download link is not shown with the new code")
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
		`{"Instruction":"what does the build line say","Shape":"what does the {x}"}`:                             {"what does the build line say", ""},
		`{"Task":"dark mode","InFront":"Safari","Installed":["Safari"]}`:                                         {"dark mode", "Safari"},
		`{"task":"crop","app":"Photos","tree":"secret window contents"}`:                                         {"crop", "Photos"},
		`{"Ask":"make it shorter","Text":"a private letter the person selected"}`:                                {"make it shorter", ""},
		`{"Question":"what is the total","App":"Numbers","Text":"the numbers on screen"}`:                        {"what is the total", "Numbers"},
		`{"Action":"make it purple","Surface":{"App":"Notes","Things":["a note"]}}`:                              {"make it purple", "Notes"},
		`{"Task":"open settings","Tree":{"app":"Antigravity","nodes":[{"name":"private"}]}}`:                     {"open settings", "Antigravity"},
		`{"Intent":"highlight Pending","URL":"https://www.github.com/vedansh-5/VANS/pulls?q=x","Structure":"."}`: {"highlight Pending", "github.com"},
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

func TestBehindAGateTheConsoleIsInvisibleWithoutIt(t *testing.T) {
	ledger := NewMemory([]Tester{{Name: "Riya", Code: riya, Daily: 10}})
	console := &Console{Books: ledger, Key: consoleKey, Gate: "gate-secret"}

	for _, header := range []string{"", "wrong"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/console", nil)
		if header != "" {
			req.Header.Set(GateHeader, header)
		}
		console.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "Console key") {
			t.Errorf("gate %q: answered %d", header, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/console/login", strings.NewReader(url.Values{"key": {consoleKey}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(GateHeader, "gate-secret")
	console.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("signing in through the gate answered %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	cookie := rec.Result().Cookies()[0]
	if cookie.Path != "/" {
		t.Errorf("cookie path = %q", cookie.Path)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/console/testers", nil)
	req.Header.Set(GateHeader, "gate-secret")
	req.AddCookie(cookie)
	console.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `href="/activity"`) || strings.Contains(body, "/console/") {
		t.Errorf("through the gate the links should be plain: %d", rec.Code)
	}
}

func TestATesterHasTheirOwnPage(t *testing.T) {
	srv, ledger, client := consoleServer(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	other := "vans-another-tester-code"
	_ = ledger.AddTester(ctx, Tester{Name: "Kabir", Code: other, Daily: 5})
	_, _, _ = ledger.Spend(ctx, riya, Day(at))
	_ = ledger.Record(ctx, Activity{At: at, From: "Riya", Kind: "app_guide", Words: "how do I add a layer", App: "Figma", Took: 2 * time.Second, By: "groq/a"})
	_ = ledger.Record(ctx, Activity{At: at.Add(time.Minute), From: "Riya", Kind: "app_action", Words: "make a note", App: "Notes", Fault: "not_found"})
	_ = ledger.Record(ctx, Activity{At: at, From: "Kabir", Kind: "app_guide", Words: "something only Kabir asked", App: "Mail"})
	habits, _ := json.Marshal([]synth.LearnedHabit{{What: "Start in Clockify", When: "after you open Mail", Days: 4, Times: 3, Yes: 2, Always: true}})
	_ = ledger.KeepLearned(ctx, "Riya", at, string(habits))
	signIn(t, srv, client)

	_, list := page(t, client, http.MethodGet, srv.URL+"/console/testers", nil)
	if !strings.Contains(list, `href="/console/testers/Riya"`) {
		t.Error("the testers list does not link to Riya's page")
	}

	status, body := page(t, client, http.MethodGet, srv.URL+"/console/testers/Riya", nil)
	if status != http.StatusOK {
		t.Fatalf("Riya's page answered %d", status)
	}
	for _, want := range []string{"how do I add a layer", "make a note", "Figma", "not found", "Start in Clockify", "after you open Mail", "always", "of 10"} {
		if !strings.Contains(body, want) {
			t.Errorf("Riya's page is missing %q", want)
		}
	}
	if strings.Contains(body, "something only Kabir asked") {
		t.Error("Riya's page shows another tester's request")
	}

	if status, _ := page(t, client, http.MethodPost, srv.URL+"/console/testers/daily", url.Values{"code": {riya}, "daily": {"25"}, "back": {"Riya"}}); status != http.StatusOK {
		t.Fatalf("changing the allowance from Riya's page answered %d", status)
	}
	if got, _, _ := ledger.Tester(ctx, riya); got.Daily != 25 {
		t.Errorf("allowance = %d, want 25", got.Daily)
	}

	if status, _ := page(t, client, http.MethodGet, srv.URL+"/console/testers/Nobody", nil); status != http.StatusNotFound {
		t.Errorf("a tester who does not exist answered %d", status)
	}
}

func TestATesterPageNeedsTheKey(t *testing.T) {
	srv, _, client := consoleServer(t)
	status, body := page(t, client, http.MethodGet, srv.URL+"/console/testers/Riya", nil)
	if status != http.StatusUnauthorized || strings.Contains(body, riya) {
		t.Errorf("Riya's page answered %d without the key", status)
	}
}

func TestAFailureKeepsTheAppAndTheModelsReason(t *testing.T) {
	srv, ledger := feedbackServer(t, 10, &postbox{})
	_, err := synth.NewRemote(srv.URL, riya).WriteAction(context.Background(), synth.AppActRequest{Action: "make it purple", Surface: capability.Surface{App: "Notes"}})
	var cannot *synth.UnwritableError
	if !errors.As(err, &cannot) {
		t.Fatalf("got %v", err)
	}
	got, _ := ledger.Activity(context.Background(), "2026-10-14")
	if len(got) != 1 || got[0].App != "Notes" || got[0].Words != "make it purple" || got[0].Fault != synth.FaultUnwritable || got[0].Why != "Notes has no way to do that" {
		t.Errorf("kept %+v", got)
	}
}
