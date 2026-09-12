package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

const page = `<!doctype html><html><body><div id="main"><div class="results">
  <article class="card"><h3><a href="/p/1">Widget</a></h3><span class="price">$10</span></article>
  <article class="card"><h3><a href="/p/2">Gadget</a></h3><span class="price">$20</span></article>
  <article class="card"><h3><a href="/p/3">Thing</a></h3><span class="price">$30</span></article>
</div></div></body></html>`

func program() extract.Program {
	return extract.Program{
		Root: "div.results > article.card",
		Fields: []extract.Field{
			{Name: "name", Selector: "h3 a", Attr: "text"},
			{Name: "price", Selector: "span.price", Attr: "text"},
		},
		Expect: extract.Expectation{MinRows: 1, Required: []string{"name", "price"}},
	}
}

// --- fakes ---

type fakeBrowser struct{ err error }

func (b *fakeBrowser) Navigate(context.Context, string) error { return b.err }
func (b *fakeBrowser) URL(context.Context) (string, error) {
	return "https://shop.example/search", nil
}
func (b *fakeBrowser) OuterHTML(context.Context) (string, error) { return page, nil }

type fakeSink struct {
	writes atomic.Int32
	err    error
}

func (s *fakeSink) Destination() engine.Destination {
	return engine.Destination{Name: "notes", Describe: "test sink", TouchesWorld: true}
}

func (s *fakeSink) Write(_ context.Context, doc engine.Document) (engine.Receipt, error) {
	s.writes.Add(1)
	if s.err != nil {
		return engine.Receipt{}, s.err
	}
	return engine.Receipt{ID: "note-1", Title: doc.Title, Verified: true}, nil
}

type fakeSynth struct{ p extract.Program }

func (f fakeSynth) Synthesize(context.Context, synth.Request) (extract.Program, error) {
	return f.p, nil
}

type fakeProbe struct {
	browserErr error
	llmErr     error
	notesErr   error
	accessErr  error
	notesCalls atomic.Int32
}

func (p *fakeProbe) BrowserReachable(context.Context) error { return p.browserErr }
func (p *fakeProbe) LLMConfigured() error                   { return p.llmErr }
func (p *fakeProbe) AccessibilityGranted() error            { return p.accessErr }
func (p *fakeProbe) NotesAccessible(context.Context) error {
	p.notesCalls.Add(1)
	return p.notesErr
}

// --- harness ---

type session struct {
	t     *testing.T
	sink  *fakeSink
	probe *fakeProbe

	responses map[int]Response
	logs      map[int][]string
}

// run feeds requests through a Server and collects everything it wrote.
func run(t *testing.T, configure func(*Server, *session), reqs ...Request) *session {
	t.Helper()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	s := &session{
		t:         t,
		sink:      &fakeSink{},
		probe:     &fakeProbe{},
		responses: map[int]Response{},
		logs:      map[int][]string{},
	}

	srv := &Server{
		Engine: &engine.Engine{
			Browser: &fakeBrowser{},
			Store:   st,
			Synth:   fakeSynth{program()},
			Sinks:   map[string]engine.Sink{"notes": s.sink},
			Model:   "test",
		},
		Probe:    s.probe,
		HostPort: "127.0.0.1:9222",
	}
	if configure != nil {
		configure(srv, s)
	}

	var in bytes.Buffer
	enc := json.NewEncoder(&in)
	for _, r := range reqs {
		if err := enc.Encode(r); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := srv.Serve(context.Background(), &in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var probe struct {
			Method string `json:"method"`
			For    int    `json:"for"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("undecodable output line %q: %v", line, err)
		}
		if probe.Method == "log" {
			var n Notification
			_ = json.Unmarshal([]byte(line), &n)
			var lp LogParams
			_ = json.Unmarshal(n.Params, &lp)
			s.logs[probe.For] = append(s.logs[probe.For], lp.Message)
			continue
		}
		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("undecodable response %q: %v", line, err)
		}
		s.responses[resp.ID] = resp
	}
	return s
}

func req(id int, method string, params any) Request {
	raw, err := json.Marshal(params)
	if err != nil {
		panic(err)
	}
	return Request{ID: id, Method: method, Params: raw}
}

func (s *session) ok(id int) json.RawMessage {
	s.t.Helper()
	r, found := s.responses[id]
	if !found {
		s.t.Fatalf("no response for request %d", id)
	}
	if r.Error != nil {
		s.t.Fatalf("request %d failed: %s: %s", id, r.Error.Code, r.Error.Message)
	}
	return r.Result
}

func (s *session) failure(id int) *Error {
	s.t.Helper()
	r, found := s.responses[id]
	if !found {
		s.t.Fatalf("no response for request %d", id)
	}
	if r.Error == nil {
		s.t.Fatalf("request %d unexpectedly succeeded: %s", id, r.Result)
	}
	return r.Error
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode: %v (from %s)", err, raw)
	}
	return v
}

func prepareParams() PrepareParams {
	// A destination is named explicitly. Without one the plan goes to the
	// panel, which writes nothing — so a commit test that omitted it would
	// pass or fail for reasons unrelated to what it is checking.
	return PrepareParams{
		URL: "https://shop.example/search", Intent: "names and prices",
		Title: "Widgets", Destination: "notes",
	}
}

// --- the gate ---

// Prepare must be inert. The shell shows what it returned and the person
// decides; nothing before that decision may touch the world.
func TestPrepareWritesNothing(t *testing.T) {
	s := run(t, nil, req(1, "prepare", prepareParams()))

	res := decode[PrepareResult](t, s.ok(1))
	if len(res.Rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(res.Rows))
	}
	if n := s.sink.writes.Load(); n != 0 {
		t.Errorf("prepare performed %d writes", n)
	}
	if res.PlanID == "" {
		t.Error("no plan id was issued, so the plan cannot be confirmed")
	}
}

// live drives a Server over a pipe, so a request can depend on the answer to
// the one before it — which the whole gate does: Commit takes an id only
// Prepare can issue.
type live struct {
	t    *testing.T
	in   *io.PipeWriter
	dec  *json.Decoder
	sink *fakeSink
	done chan struct{}
}

func newLive(t *testing.T, configure func(*Server)) *live {
	t.Helper()

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	sink := &fakeSink{}
	srv := &Server{
		Engine: &engine.Engine{
			Browser: &fakeBrowser{}, Store: st,
			Synth: fakeSynth{program()}, Sinks: map[string]engine.Sink{"notes": sink}, Model: "test",
		},
		Probe: &fakeProbe{}, HostPort: "127.0.0.1:9222",
	}
	if configure != nil {
		configure(srv)
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	l := &live{t: t, in: inW, dec: json.NewDecoder(outR), sink: sink, done: make(chan struct{})}

	go func() {
		defer close(l.done)
		_ = srv.Serve(context.Background(), inR, outW)
		outW.Close()
	}()
	t.Cleanup(func() {
		inW.Close()
		<-l.done
	})
	return l
}

// call sends a request and returns the response, skipping progress lines.
func (l *live) call(id int, method string, params any) Response {
	l.t.Helper()

	if err := json.NewEncoder(l.in).Encode(req(id, method, params)); err != nil {
		l.t.Fatalf("send: %v", err)
	}
	for {
		var raw json.RawMessage
		if err := l.dec.Decode(&raw); err != nil {
			l.t.Fatalf("receive: %v", err)
		}
		var probe struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(raw, &probe)
		if probe.Method == "log" {
			continue
		}
		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			l.t.Fatalf("decode response: %v", err)
		}
		return resp
	}
}

// The full gate, over the wire: prepare returns a plan, nothing is written,
// and only the explicit confirm produces the side effect.
func TestCommitWritesOnlyAfterPrepare(t *testing.T) {
	l := newLive(t, nil)

	resp := l.call(1, "prepare", prepareParams())
	if resp.Error != nil {
		t.Fatalf("prepare: %v", resp.Error)
	}
	plan := decode[PrepareResult](t, resp.Result)

	if n := l.sink.writes.Load(); n != 0 {
		t.Fatalf("prepare wrote %d times", n)
	}

	resp = l.call(2, "commit", CommitParams{PlanID: plan.PlanID})
	if resp.Error != nil {
		t.Fatalf("commit: %v", resp.Error)
	}

	receipt := decode[CommitResult](t, resp.Result)
	if receipt.ID == "" || !receipt.Verified {
		t.Errorf("receipt = %+v, want a verified write", receipt)
	}
	if n := l.sink.writes.Load(); n != 1 {
		t.Errorf("sink saw %d writes", n)
	}
}

// The second confirm must be refused across the wire too, not only in-process.
func TestCommitTwiceOverTheWire(t *testing.T) {
	l := newLive(t, nil)

	plan := decode[PrepareResult](t, l.call(1, "prepare", prepareParams()).Result)
	if resp := l.call(2, "commit", CommitParams{PlanID: plan.PlanID}); resp.Error != nil {
		t.Fatalf("commit: %v", resp.Error)
	}

	resp := l.call(3, "commit", CommitParams{PlanID: plan.PlanID})
	if resp.Error == nil {
		t.Fatal("a second commit succeeded")
	}
	if resp.Error.Code != CodeSpent {
		t.Errorf("code = %q, want %q", resp.Error.Code, CodeSpent)
	}
	if n := l.sink.writes.Load(); n != 1 {
		t.Errorf("sink saw %d writes", n)
	}
}

// The second run of the same task must not call the model.
func TestWarmRunOverTheWire(t *testing.T) {
	l := newLive(t, nil)

	first := decode[PrepareResult](t, l.call(1, "prepare", prepareParams()).Result)
	if first.SynthesisCalls != 1 || first.FromCache {
		t.Fatalf("cold run: calls=%d fromCache=%v", first.SynthesisCalls, first.FromCache)
	}

	second := decode[PrepareResult](t, l.call(2, "prepare", prepareParams()).Result)
	if second.SynthesisCalls != 0 || !second.FromCache {
		t.Errorf("warm run: calls=%d fromCache=%v, want 0 and true",
			second.SynthesisCalls, second.FromCache)
	}
	if len(second.Rows) != 3 {
		t.Errorf("warm run returned %d rows", len(second.Rows))
	}
}

// A plan is confirmed once. D-020: an executed write is never retried.
func TestCommitIsOneShot(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	sink := &fakeSink{}
	srv := &Server{
		Engine: &engine.Engine{
			Browser: &fakeBrowser{}, Store: st,
			Synth: fakeSynth{program()}, Sinks: map[string]engine.Sink{"notes": sink}, Model: "test",
		},
		Probe: &fakeProbe{}, HostPort: "127.0.0.1:9222",
	}

	var out bytes.Buffer
	srv.out = json.NewEncoder(&out)
	srv.plans = map[string]*engine.Plan{}
	srv.running = map[int]context.CancelFunc{}

	ctx := context.Background()
	res, err := srv.prepare(ctx, req(1, "prepare", prepareParams()))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if _, err := srv.commit(ctx, req(2, "commit", CommitParams{PlanID: res.PlanID})); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if n := sink.writes.Load(); n != 1 {
		t.Fatalf("sink saw %d writes", n)
	}

	_, err = srv.commit(ctx, req(3, "commit", CommitParams{PlanID: res.PlanID}))
	if err == nil {
		t.Fatal("a second commit succeeded and would have duplicated the note")
	}
	if code := asError(err).Code; code != CodeSpent {
		t.Errorf("code = %q, want %q so the shell can explain it", code, CodeSpent)
	}
	if n := sink.writes.Load(); n != 1 {
		t.Errorf("sink saw %d writes after the retry", n)
	}
}

func TestCommitRejectsAnUnknownPlan(t *testing.T) {
	s := run(t, nil, req(1, "commit", CommitParams{PlanID: "does-not-exist"}))

	if e := s.failure(1); e.Code != CodeBadRequest {
		t.Errorf("code = %q", e.Code)
	}
	if n := s.sink.writes.Load(); n != 0 {
		t.Errorf("an unknown plan produced %d writes", n)
	}
}

// --- preflight ---

// Probing Notes launches it and can raise a permission prompt. Neither should
// happen merely because a window opened (D-032).
func TestPreflightDoesNotTouchNotes(t *testing.T) {
	s := run(t, nil, req(1, "preflight", nil))

	res := decode[PreflightResult](t, s.ok(1))
	if !res.Ready {
		t.Errorf("preflight not ready with everything healthy: %+v", res.Checks)
	}
	if n := s.probe.notesCalls.Load(); n != 0 {
		t.Errorf("preflight probed Notes %d times; it must wait to be asked", n)
	}
	for _, c := range res.Checks {
		if c.Name == "notes" {
			t.Error("Notes appeared in the automatic preflight")
		}
	}
}

func TestCheckNotesOnlyRunsWhenAsked(t *testing.T) {
	s := run(t, nil, req(1, "check_notes", nil))

	c := decode[Check](t, s.ok(1))
	if !c.OK {
		t.Errorf("check = %+v", c)
	}
	if n := s.probe.notesCalls.Load(); n != 1 {
		t.Errorf("Notes probed %d times, want 1", n)
	}
}

func TestCheckNotesReportsADenialWithItsFix(t *testing.T) {
	// A real ScriptError, wrapped the way NotesSink wraps it, so the test
	// exercises the unwrapping that classification depends on rather than a
	// convenient stand-in.
	denial := fmt.Errorf("notes: write failed: %w", &appleevents.ScriptError{
		Err:    errors.New("exit status 1"),
		Stderr: "execution error: Not authorized to send Apple events to Notes. (-1743)",
	})

	s := run(t, func(_ *Server, s *session) {
		s.probe.notesErr = denial
	}, req(1, "check_notes", nil))

	c := decode[Check](t, s.ok(1))
	if c.OK {
		t.Fatal("a denial was reported as granted")
	}
	if !strings.Contains(c.Fix, "System Settings") {
		t.Errorf("no actionable fix: %+v", c)
	}
}

// A missing browser is not a generic error — the fix is a command the user
// runs, and the runtime will not run it for them.
func TestPreflightExplainsAMissingBrowser(t *testing.T) {
	s := run(t, func(_ *Server, s *session) {
		s.probe.browserErr = errors.New("connection refused")
	}, req(1, "preflight", nil))

	res := decode[PreflightResult](t, s.ok(1))
	if res.Ready {
		t.Error("ready with no browser")
	}

	var browser Check
	for _, c := range res.Checks {
		if c.Name == "browser" {
			browser = c
		}
	}
	if browser.OK {
		t.Fatal("browser check passed")
	}
	for _, want := range []string{"--remote-debugging-port=9222", "--user-data-dir", "does not launch"} {
		if !strings.Contains(browser.Fix, want) {
			t.Errorf("fix is missing %q:\n%s", want, browser.Fix)
		}
	}
}

func TestPreflightReportsAMissingKey(t *testing.T) {
	s := run(t, func(_ *Server, s *session) {
		s.probe.llmErr = errors.New("VANS_LLM_API_KEY is not set")
	}, req(1, "preflight", nil))

	res := decode[PreflightResult](t, s.ok(1))
	if res.Ready {
		t.Error("ready with no synthesis backend")
	}
	if !strings.Contains(res.Checks[0].Fix, "VANS_LLM_API_KEY") {
		t.Errorf("fix = %q", res.Checks[0].Fix)
	}
}

// --- failures ---

// Rung 3 has to survive the transport, or the shell shows "extraction failed"
// with no next step in it.
func TestUnresolvedCarriesItsDetail(t *testing.T) {
	broken := program()
	broken.Fields[1].Selector = "span.nope"

	s := run(t, func(srv *Server, _ *session) {
		srv.Engine.Synth = fakeSynth{broken}
	}, req(1, "prepare", prepareParams()))

	e := s.failure(1)
	if e.Code != CodeUnresolved {
		t.Fatalf("code = %q, want %q", e.Code, CodeUnresolved)
	}

	data := decode[UnresolvedData](t, e.Data)
	if len(data.Attempts) == 0 {
		t.Fatal("no attempts were carried")
	}
	if data.Attempts[0].Root == "" || len(data.Attempts[0].Reasons) == 0 {
		t.Errorf("attempt is empty: %+v", data.Attempts[0])
	}
	if !strings.Contains(strings.Join(data.Attempts[0].Reasons, " "), "price") {
		t.Errorf("the failing field is not named: %v", data.Attempts[0].Reasons)
	}
	if len(data.Candidates) == 0 {
		t.Error("the candidates found were not carried")
	}
	if data.Signature == "" {
		t.Error("the page signature was not carried")
	}
}

func TestNoBrowserIsItsOwnCode(t *testing.T) {
	s := run(t, func(srv *Server, _ *session) {
		srv.Engine.Browser = &fakeBrowser{
			err: &engine.TransientError{Err: errors.New("no Chromium debugging endpoint at 127.0.0.1:9222")},
		}
		srv.Engine.MaxTransientRetries = 1
	}, req(1, "prepare", prepareParams()))

	if e := s.failure(1); e.Code != CodeNoBrowser {
		t.Errorf("code = %q, want %q", e.Code, CodeNoBrowser)
	}
}

func TestBrowserWithNoTabIsNoBrowser(t *testing.T) {
	s := run(t, func(srv *Server, _ *session) {
		srv.Engine.Browser = &fakeBrowser{
			err: &engine.TransientError{Err: errors.New("cdp: no page target among 0 targets")},
		}
		srv.Engine.MaxTransientRetries = 1
	}, req(1, "prepare", prepareParams()))

	if e := s.failure(1); e.Code != CodeNoBrowser {
		t.Errorf("code = %q, want %q", e.Code, CodeNoBrowser)
	}
}

func TestUnknownMethod(t *testing.T) {
	s := run(t, nil, req(1, "eval", nil))
	if e := s.failure(1); e.Code != CodeBadRequest {
		t.Errorf("code = %q", e.Code)
	}
}

func TestMalformedLineDoesNotKillTheServer(t *testing.T) {
	var in bytes.Buffer
	in.WriteString("this is not json\n")
	raw, _ := json.Marshal(req(7, "preflight", nil))
	in.Write(raw)
	in.WriteString("\n")

	st, _ := store.Open(":memory:")
	defer st.Close()

	srv := &Server{
		Engine: &engine.Engine{Browser: &fakeBrowser{}, Store: st, Synth: fakeSynth{program()}, Sinks: map[string]engine.Sink{"notes": &fakeSink{}}},
		Probe:  &fakeProbe{}, HostPort: "127.0.0.1:9222",
	}

	var out bytes.Buffer
	if err := srv.Serve(context.Background(), &in, &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !strings.Contains(out.String(), `"id":7`) {
		t.Errorf("the request after the bad line was never answered:\n%s", out.String())
	}
}

// A cold run takes ten seconds. A silent window looks broken.
func TestProgressIsReported(t *testing.T) {
	s := run(t, nil, req(1, "prepare", prepareParams()))
	s.ok(1)

	if len(s.logs[1]) == 0 {
		t.Fatal("no progress was reported during prepare")
	}
	joined := strings.Join(s.logs[1], " ")
	if !strings.Contains(joined, "signature") {
		t.Errorf("progress did not mention the page: %v", s.logs[1])
	}
}

func TestWarmPrepareReportsZeroModelCalls(t *testing.T) {
	s := run(t, nil,
		req(1, "prepare", prepareParams()),
	)
	first := decode[PrepareResult](t, s.ok(1))
	if first.SynthesisCalls != 1 || first.FromCache {
		t.Errorf("cold run: calls=%d fromCache=%v", first.SynthesisCalls, first.FromCache)
	}
}

func TestScreenContextHandlesFrontmostApp(t *testing.T) {
	params := ScreenContextParams{
		AppName:     "Google Chrome",
		BundleID:    "com.google.Chrome",
		WindowTitle: "Hacker News",
		PID:         5432,
		X:           0,
		Y:           0,
		Width:       1920,
		Height:      1080,
	}
	s := run(t, nil, req(1, "screen_context", params))
	res := decode[ScreenContextResult](t, s.ok(1))
	if !res.Handled {
		t.Errorf("expected screen context to be handled, got %+v", res)
	}
	if res.Target != "Google Chrome" {
		t.Errorf("expected target 'Google Chrome', got %q", res.Target)
	}
}

func TestPreflightReportsAccessibilityWithItsFix(t *testing.T) {
	s := run(t, func(_ *Server, sess *session) {
		sess.probe.accessErr = uiact.ErrNoAssistiveAccess
	}, req(1, "preflight", nil))

	res := decode[PreflightResult](t, s.ok(1))

	var found *Check
	for i := range res.Checks {
		if res.Checks[i].Name == "accessibility" {
			found = &res.Checks[i]
		}
	}
	if found == nil {
		t.Fatal("preflight says nothing about accessibility")
	}
	if found.OK {
		t.Error("reported granted while the probe refused")
	}
	if found.Fix == "" {
		t.Error("no fix offered for a permission the user can grant")
	}
	if found.Required {
		t.Error("accessibility is one tier of several, not a prerequisite for starting")
	}
	if !res.Ready {
		t.Error("a missing optional permission blocked startup")
	}
}

func TestPreflightReportsAccessibilityWhenGranted(t *testing.T) {
	s := run(t, nil, req(1, "preflight", nil))

	res := decode[PreflightResult](t, s.ok(1))
	for _, c := range res.Checks {
		if c.Name == "accessibility" && !c.OK {
			t.Errorf("reported missing while the probe granted it: %+v", c)
		}
	}
}
