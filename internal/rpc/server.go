package rpc

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/adapter/appleevents"
	"github.com/vedansh/vans/internal/engine"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/runner"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// ShownDeeds is how many changes the panel is given. The store keeps more; a
// list nobody scrolls is not more accountable for being longer.
const ShownDeeds = 50

// DeedReader reads back what VANS has done. Implemented by store.
type DeedReader interface {
	RecentDeeds(ctx context.Context, limit int) ([]store.Deed, error)
}

// Prober reports whether an environment prerequisite is satisfied. Preflight
// is built from these so the checks can be exercised without a browser or a
// permissions prompt.
type Prober interface {
	// BrowserReachable reports whether a Chromium debugging endpoint answers.
	BrowserReachable(ctx context.Context) error

	// LLMConfigured reports whether a synthesis backend can be built.
	LLMConfigured() error

	// AccessibilityGranted reports whether VANS may read and drive other
	// applications' windows. Asking does not raise a prompt, so unlike Notes
	// this one is safe in an automatic preflight.
	AccessibilityGranted() error

	// NotesAccessible reports whether Notes can be driven. This one launches
	// Notes.app and can raise a permission prompt, so it is only called when
	// the user asks for it — never during an automatic preflight.
	NotesAccessible(ctx context.Context) error
}

// Server answers requests from the desktop shell.
type Server struct {
	Engine *engine.Engine
	Probe  Prober

	// HostPort is reported in fix instructions so the message names the port
	// actually in use.
	HostPort string

	// Trace, when set, receives every line the runtime reports on its way to
	// the window. The window shows one line at a time and keeps none of them,
	// which is the right thing for someone watching and useless for working out
	// afterwards why a run did what it did.
	Trace io.Writer

	// Deeds is the record of what VANS has done, for the panel to show back.
	// Optional: a build without one answers with an empty list rather than
	// refusing, which is the truth for a build that remembers nothing.
	Deeds DeedReader

	out   *json.Encoder
	outMu sync.Mutex

	mu           sync.Mutex
	plans        map[string]*engine.Plan
	running      map[int]context.CancelFunc
	guideCancel  context.CancelFunc
	guideAdvance chan struct{}

	// actCancel stops a run that is acting on screen. Kept beside guideCancel and
	// stopped by the same request, because from where the person sits they are
	// the same thing: VANS doing something in their window, and Escape to stop it.
	actCancel context.CancelFunc
	actResume chan struct{}
}

// Serve reads requests from in and writes responses to out until in ends.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = json.NewEncoder(out)
	s.plans = map[string]*engine.Plan{}
	s.running = map[int]context.CancelFunc{}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)

	var wg sync.WaitGroup
	defer wg.Wait()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.reply(Response{Error: &Error{Code: CodeBadRequest, Message: err.Error()}})
			continue
		}

		// Cancel is handled inline: dispatching it would queue it behind the
		// very call it is meant to interrupt.
		if req.Method == "cancel" {
			s.cancel(req)
			continue
		}

		wg.Add(1)
		go func(req Request) {
			defer wg.Done()
			s.dispatch(ctx, req)
		}(req)
	}
	return scanner.Err()
}

func (s *Server) dispatch(ctx context.Context, req Request) {
	ctx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.running[req.ID] = cancel
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.running, req.ID)
		s.mu.Unlock()
	}()

	var (
		result any
		err    error
	)
	switch req.Method {
	case "preflight":
		result, err = s.preflight(ctx)
	case "check_notes":
		result, err = s.checkNotes(ctx)
	case "prepare":
		result, err = s.prepare(ctx, req)
	case "commit":
		result, err = s.commit(ctx, req)
	case "destinations":
		result = s.destinations()
	case "suggestions":
		result, err = s.suggestions(ctx, req)
	case "screen_context":
		result, err = s.screenContext(ctx, req)
	case "clear_style":
		result, err = s.clearStyle(ctx)
	case "history":
		result, err = s.history(ctx)
	case "undo":
		result, err = s.undo(ctx, req)
	case "guide":
		result, err = s.guide(ctx, req)
	case "stop_guide":
		result, err = s.stopGuide()
	case "advance_guide":
		result, err = s.advanceGuide()
	case "resume_drive":
		result, err = s.resumeDrive()
	default:
		err = &Error{Code: CodeBadRequest, Message: fmt.Sprintf("unknown method %q", req.Method)}
	}

	if err != nil {
		s.reply(Response{ID: req.ID, Error: asError(err)})
		return
	}

	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		s.reply(Response{ID: req.ID, Error: &Error{Code: CodeInternal, Message: marshalErr.Error()}})
		return
	}
	s.reply(Response{ID: req.ID, Result: raw})
}

func (s *Server) cancel(req Request) {
	var p struct {
		Target int `json:"target"`
	}
	_ = json.Unmarshal(req.Params, &p)

	s.mu.Lock()
	stop := s.running[p.Target]
	s.mu.Unlock()

	if stop != nil {
		stop()
	}
	s.reply(Response{ID: req.ID, Result: json.RawMessage(`{"cancelled":true}`)})
}

func (s *Server) reply(r Response) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_ = s.out.Encode(r)
}

// notify pushes a progress line for a request already in flight.
func (s *Server) notify(id int, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if s.Trace != nil {
		fmt.Fprintf(s.Trace, "[%d] %s\n", id, message)
	}
	s.push(id, "log", LogParams{Message: message})
}

func (s *Server) push(id int, method string, params any) {
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_ = s.out.Encode(Notification{Method: method, For: id, Params: raw})
}

// --- methods ---

func (s *Server) preflight(ctx context.Context) (PreflightResult, error) {
	checks := []Check{
		{
			Name:     "llm",
			Label:    "Synthesis backend",
			Required: true,
		},
		{
			Name:     "browser",
			Label:    "Chrome debugging endpoint",
			Required: true,
		},
		{
			Name:  "accessibility",
			Label: "Accessibility access",
		},
	}

	if err := s.Probe.LLMConfigured(); err != nil {
		checks[0].Detail = err.Error()
		checks[0].Fix = "Set VANS_LLM_API_KEY to an OpenRouter key, or VANS_LLM_PROVIDER=anthropic with ANTHROPIC_API_KEY."
	} else {
		checks[0].OK = true
		checks[0].Detail = "Configured."
	}

	if err := s.Probe.BrowserReachable(ctx); err != nil {
		checks[1].Detail = err.Error()
		checks[1].Fix = s.browserFix()
	} else {
		checks[1].OK = true
		checks[1].Detail = "Connected."
	}

	if err := s.Probe.AccessibilityGranted(); err != nil {
		checks[2].Detail = err.Error()
		checks[2].Fix = uiact.Fix(err)
	} else {
		checks[2].OK = true
		checks[2].Detail = "Granted."
	}

	// Notes is deliberately absent. Probing it launches the app and can raise
	// a permission prompt, and neither should happen because a window opened
	// (D-032). check_notes exists for when the user asks.
	ready := true
	for _, c := range checks {
		if c.Required && !c.OK {
			ready = false
		}
	}
	return PreflightResult{Ready: ready, Checks: checks}, nil
}

func (s *Server) browserFix() string {
	port := "9222"
	if i := strings.LastIndex(s.HostPort, ":"); i >= 0 && i+1 < len(s.HostPort) {
		port = s.HostPort[i+1:]
	}
	return "Start Chrome yourself with:\n\n" +
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome \\\n" +
		"  --remote-debugging-port=" + port + " \\\n" +
		"  --user-data-dir=/tmp/vans-chrome\n\n" +
		"The separate profile matters: without it Chrome attaches to your running " +
		"instance and ignores the flag. VANS does not launch applications for you."
}

func (s *Server) checkNotes(ctx context.Context) (Check, error) {
	c := Check{
		Name:     "notes",
		Label:    "Notes automation access",
		Required: false,
	}
	if err := s.Probe.NotesAccessible(ctx); err != nil {
		c.Detail = err.Error()
		if appleevents.IsPermissionDenied(err) {
			c.Fix = "Allow access in System Settings › Privacy & Security › Automation, then check again."
		}
		return c, nil
	}
	c.OK = true
	c.Detail = "Granted."
	return c, nil
}

// destinations lists everywhere an answer can go.
//
// Sent to the shell rather than hardcoded there, so adding a sink to the
// runtime makes it appear in the window without a matching change in the
// frontend — and so the window cannot offer a destination that does not exist.
func (s *Server) suggestions(ctx context.Context, req Request) ([]Suggestion, error) {
	var params SuggestionsParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &Error{
				Code:    CodeBadRequest,
				Message: fmt.Sprintf("invalid suggestions params: %v", err),
			}
		}
	}

	out := []Suggestion{}
	for _, sg := range s.Engine.Suggestions(ctx, params.AppName) {
		out = append(out, Suggestion{Icon: sg.Icon, Text: sg.Text, Destination: sg.Destination})
	}
	return out, nil
}

func (s *Server) destinations() []Destination {
	var out []Destination
	for _, d := range s.Engine.Destinations() {
		out = append(out, Destination{
			Name:         d.Name,
			Describe:     d.Describe,
			TouchesWorld: d.TouchesWorld,
		})
	}
	return out
}

func (s *Server) prepare(ctx context.Context, req Request) (PrepareResult, error) {
	var p PrepareParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return PrepareResult{}, &Error{Code: CodeBadRequest, Message: err.Error()}
	}

	// Progress is routed back to the shell rather than a log file. A cold run
	// takes ten seconds and a silent window looks broken.
	eng := *s.Engine
	eng.Log = func(format string, args ...any) { s.notify(req.ID, format, args...) }

	started := time.Now()

	// Free text when there is any, the resolved form otherwise. The CLI and
	// anything else that already knows what it wants skips the router
	// entirely, and so skips its model call.
	var plan *engine.Plan
	var err error
	if strings.TrimSpace(p.Instruction) != "" {
		plan, err = eng.Ask(ctx, engine.Ask{
			Instruction: p.Instruction,
			Destination: p.Destination,
			Context:     p.Context,
		})
	} else {
		plan, err = eng.Prepare(ctx, engine.Task{
			URL: p.URL, Intent: p.Intent, Title: p.Title,
			Destination: p.Destination,
		})
	}
	if err != nil {
		return PrepareResult{}, err
	}

	id, err := newID()
	if err != nil {
		return PrepareResult{}, err
	}
	s.mu.Lock()
	s.plans[id] = plan
	s.mu.Unlock()

	rows := make([]map[string]string, len(plan.Rows))
	for i, r := range plan.Rows {
		rows[i] = r
	}

	title := plan.Task.Title
	if strings.TrimSpace(title) == "" {
		title = fmt.Sprintf("%s — %s", plan.Task.Intent, plan.PageURL)
	}

	kind := ""
	if plan.Route != nil {
		kind = string(plan.Route.Kind)
	}

	action := ""
	app := ""
	switch {
	case plan.Action != nil:
		action = plan.Action.Preview
		app = plan.Action.App
	case plan.Web != nil:
		action = plan.Web.Preview
	case plan.UI != nil:
		action = plan.UI.Preview
		app = plan.UI.App
	case plan.Drive != nil:
		action = plan.Drive.Preview
		app = plan.Drive.App
		if plan.Drive.Browser != "" {
			app = plan.Drive.Browser
		}
	case plan.Style != nil:
		action = plan.Style.Preview
	case plan.Guide != nil:
		action = plan.Guide.Preview
		app = plan.Guide.App
		if plan.Guide.Browser != "" {
			app = plan.Guide.Browser
		}
	}

	answerText := ""
	var cites []int
	if plan.Answer != nil {
		answerText = plan.Answer.Text
		cites = plan.Answer.Cites
	}

	return PrepareResult{
		PlanID:         id,
		Action:         action,
		Answer:         answerText,
		AnswerCites:    cites,
		AnswerCalls:    plan.AnswerCalls,
		PageURL:        plan.PageURL,
		Signature:      plan.Signature,
		Title:          title,
		Fields:         plan.Fields,
		Rows:           rows,
		SynthesisCalls: plan.SynthesisCalls,
		FromCache:      plan.FromCache,
		ElapsedMS:      time.Since(started).Milliseconds(),
		Destination:    plan.Destination,
		NeedsCommit:    plan.NeedsCommit(),
		Kind:           kind,
		App:            app,
		DecisionCalls:  plan.DecisionCalls,
	}, nil
}

func (s *Server) commit(ctx context.Context, req Request) (CommitResult, error) {
	var p CommitParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return CommitResult{}, &Error{Code: CodeBadRequest, Message: err.Error()}
	}

	s.mu.Lock()
	plan := s.plans[p.PlanID]
	s.mu.Unlock()

	if plan == nil {
		return CommitResult{}, &Error{
			Code:    CodeBadRequest,
			Message: "no such plan; prepare it again before confirming",
		}
	}

	eng := *s.Engine
	eng.Log = func(format string, args ...any) { s.notify(req.ID, format, args...) }

	// Each step of a run that acts on screen is announced just before it happens,
	// so the shell can draw the ghost cursor gliding to where it is about to be.
	eng.ShowStep = func(g engine.GuideStep) {
		s.push(req.ID, "drive_step", GuideStepParams{
			Number: g.Number, Label: g.Label,
			X: g.X, Y: g.Y, W: g.W, H: g.H,
			Do: g.Do, Detail: g.Detail,
		})
	}

	resume := make(chan struct{}, 1)
	eng.Resume = resume
	eng.Paused = func(g engine.GuideStep) {
		s.push(req.ID, "drive_paused", GuideStepParams{
			Number: g.Number, Label: g.Label,
			X: g.X, Y: g.Y, W: g.W, H: g.H,
			Do: g.Do, Detail: g.Detail, Ask: g.Ask,
		})
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.actCancel = cancel
	s.actResume = resume
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.actCancel = nil
		s.actResume = nil
		s.mu.Unlock()
		cancel()
	}()

	receipt, err := eng.Commit(runCtx, plan)
	if err != nil {
		// Stopped on purpose, not broken. Said as a stop in the application it
		// was working in, the same way a coached run that is stopped is.
		if runCtx.Err() != nil && ctx.Err() == nil {
			return CommitResult{}, &Error{
				Code:    CodeCoachStopped,
				Message: "stopped before the next step",
				Data:    mustData(CoachStoppedData{App: plan.Destination}),
			}
		}
		return CommitResult{}, err
	}
	out := CommitResult{ID: receipt.ID, Title: receipt.Title, Verified: receipt.Verified}
	if receipt.Work != nil {
		out.Work = &CommitWork{
			Steps:    receipt.Work.Steps,
			Replayed: receipt.Work.Replayed,
			Calls:    receipt.Work.Calls,
		}
	}
	return out, nil
}

func newID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("rpc: generate plan id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// asError classifies an engine failure for the shell.
//
// The shell shows a different view per code, so a failure that loses its
// identity here becomes a generic error dialog with no next step in it.
func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}

	var unresolved *engine.UnresolvedError
	if errors.As(err, &unresolved) {
		return unresolvedError(unresolved)
	}

	// Kept separate from unresolved on purpose. "I did not understand you" and
	// "I understood you and could not do it" call for different things on
	// screen: one wants a rephrase and a list of what is possible, the other
	// wants to show what was tried on the page.
	var unroutable *engine.UnroutableError
	if errors.As(err, &unroutable) {
		return &Error{
			Code:    CodeUnroutable,
			Message: unroutable.Error(),
			Data: mustData(UnroutableData{
				Instruction: unroutable.Instruction,
				Why:         unroutable.Why,
				Can:         unroutable.Can,
			}),
		}
	}

	if appleevents.IsPermissionDenied(err) {
		return &Error{
			Code:    CodePermission,
			Message: err.Error(),
			Data:    fixData("Allow access in System Settings › Privacy & Security › Automation."),
		}
	}

	// Matched on the error rather than on its text: the sentinel is what the
	// adapter promises, and the wording is not.
	if errors.Is(err, appleevents.ErrScriptingOff) {
		return &Error{
			Code:    CodeBrowserScripting,
			Message: err.Error(),
			Data:    fixData(appleevents.Fix(err)),
		}
	}
	if errors.Is(err, engine.ErrCannotBeUndone) || errors.Is(err, engine.ErrNothingToUndo) {
		return &Error{Code: CodeCannotUndo, Message: err.Error()}
	}

	if errors.Is(err, engine.ErrCoachStopped) {
		return &Error{
			Code:    CodeCoachStopped,
			Message: err.Error(),
			Data:    mustData(CoachStoppedData{App: engine.WhereItStopped(err)}),
		}
	}

	// The same thing to say as an application that is not running: VANS will not
	// open it, and the one thing to do about it is done by the person.
	var notRunning *uiact.NotRunningError
	if errors.As(err, &notRunning) {
		return &Error{
			Code:    CodeAppNotOpen,
			Message: err.Error(),
			Data: mustData(AppNotOpenData{
				App: notRunning.App,
				Why: "it is not open, and VANS does not open applications",
			}),
		}
	}

	var cannotRaise *uiact.CannotRaiseError
	if errors.As(err, &cannotRaise) {
		return &Error{
			Code:    CodeAppNotOpen,
			Message: err.Error(),
			Data: mustData(AppNotOpenData{
				App: cannotRaise.App,
				Why: "it would not come to the front, and VANS does not open applications",
			}),
		}
	}

	var notOpen *engine.AppNotOpenError
	if errors.As(err, &notOpen) {
		return &Error{
			Code:    CodeAppNotOpen,
			Message: err.Error(),
			Data: mustData(AppNotOpenData{
				App: notOpen.App,
				Why: notOpen.Why,
			}),
		}
	}
	if errors.Is(err, engine.ErrNoControlsToPointAt) {
		return &Error{
			Code:    CodeNoCoachHere,
			Message: err.Error(),
			Data:    fixData("Ask about the page itself and VANS works on it directly — reading it, filling it in, or restyling it."),
		}
	}
	var cannot *synth.UnwritableError
	if errors.As(err, &cannot) {
		return &Error{Code: CodeCannotDoThere, Message: cannot.Why}
	}
	if errors.Is(err, synth.ErrNoAnswer) {
		return &Error{
			Code:    CodeModelSilent,
			Message: err.Error(),
			Data:    fixData("Ask again. If it keeps happening the model is overloaded — set VANS_LLM_FALLBACK_MODEL to a second one."),
		}
	}
	if errors.Is(err, uiact.ErrNoAssistiveAccess) {
		return &Error{Code: CodePermission, Message: err.Error(), Data: fixData(uiact.Fix(err))}
	}

	if errors.Is(err, appleevents.ErrNotRunning) {
		return &Error{Code: CodeNoBrowser, Message: err.Error(), Data: fixData(appleevents.Fix(err))}
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "no Chromium debugging endpoint"),
		strings.Contains(msg, "Chrome has no window open"),
		strings.Contains(msg, "no page target"):
		return &Error{Code: CodeNoBrowser, Message: msg}
	case strings.Contains(msg, "already been committed"):
		return &Error{Code: CodeSpent, Message: msg}
	}
	return &Error{Code: CodeInternal, Message: msg}
}

func unresolvedError(u *engine.UnresolvedError) *Error {
	data := UnresolvedData{PageURL: u.PageURL, Signature: u.Signature}

	for _, a := range u.Attempts {
		data.Attempts = append(data.Attempts, AttemptData{
			Source:  a.Source,
			Root:    a.Program.Root,
			Span:    a.Program.Span(),
			Reasons: a.Reasons,
		})
	}
	for _, c := range u.Candidates {
		data.Candidates = append(data.Candidates, CandidateData{
			Selector:    c.Selector,
			Span:        spanOf(c),
			RecordCount: c.RecordCount,
			Samples:     c.Samples,
		})
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return &Error{Code: CodeInternal, Message: err.Error()}
	}
	return &Error{Code: CodeUnresolved, Message: u.Error(), Data: raw}
}

func spanOf(c extract.Candidate) int {
	if c.Span < 1 {
		return 1
	}
	return c.Span
}

// mustData encodes error payloads, dropping them if they cannot be encoded.
//
// Losing the detail is survivable; losing the error is not. The code and
// message always reach the shell, so the worst case is a less specific screen
// rather than a silent failure.
func mustData(v any) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return raw
}

func fixData(fix string) json.RawMessage {
	raw, err := json.Marshal(map[string]string{"fix": fix})
	if err != nil {
		return nil
	}
	return raw
}

func (s *Server) guide(ctx context.Context, req Request) (GuideResult, error) {
	var p GuideParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return GuideResult{}, &Error{Code: CodeBadRequest, Message: err.Error()}
	}

	s.mu.Lock()
	plan := s.plans[p.PlanID]
	s.mu.Unlock()

	if plan == nil || plan.Guide == nil {
		return GuideResult{}, &Error{
			Code:    CodeBadRequest,
			Message: "no such guide; ask again",
		}
	}

	guideCtx, cancel := context.WithCancel(ctx)
	advance := make(chan struct{}, 1)

	s.mu.Lock()
	s.guideCancel = cancel
	s.guideAdvance = advance
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		// Only if this guide is still the current one. A second guide can start
		// before this one has finished unwinding, and clearing unconditionally
		// takes Stop and Next away from the guide that is actually running.
		if s.guideAdvance == advance {
			s.guideCancel = nil
			s.guideAdvance = nil
		}
		s.mu.Unlock()
	}()

	// Progress goes back to the window, the same way prepare does it. Without
	// this the coach runs with no diagnostics at all: a guide that stops with
	// no steps says nothing about why.
	eng := *s.Engine
	eng.Log = func(format string, args ...any) { s.notify(req.ID, format, args...) }

	work, err := eng.RunGuideCounted(guideCtx, plan, func(g engine.GuideStep) {
		s.push(req.ID, "guide", GuideStepParams{
			Number: g.Number, Label: g.Label,
			X: g.X, Y: g.Y, W: g.W, H: g.H,
			Do: g.Do, Detail: g.Detail,
		})
	}, advance)
	if err != nil {
		if errors.Is(err, context.Canceled) || guideCtx.Err() != nil {
			return GuideResult{Steps: work.Steps, Stopped: true}, nil
		}
		return GuideResult{}, err
	}

	return GuideResult{
		Steps: work.Steps,
		Work:  &CommitWork{Steps: work.Steps, Replayed: work.Replayed, Calls: work.Calls},
	}, nil
}

func (s *Server) stopGuide() (StopGuideResult, error) {
	s.mu.Lock()
	guide, act := s.guideCancel, s.actCancel
	s.mu.Unlock()

	if guide != nil {
		guide()
	}
	if act != nil {
		act()
	}
	return StopGuideResult{Stopped: true}, nil
}

func (s *Server) advanceGuide() (AdvanceGuideResult, error) {
	s.mu.Lock()
	advance := s.guideAdvance
	s.mu.Unlock()

	if advance != nil {
		select {
		case advance <- struct{}{}:
		default:
		}
	}
	return AdvanceGuideResult{Advanced: true}, nil
}

func (s *Server) resumeDrive() (ResumeDriveResult, error) {
	s.mu.Lock()
	resume := s.actResume
	s.mu.Unlock()

	if resume == nil {
		return ResumeDriveResult{}, nil
	}
	select {
	case resume <- struct{}{}:
	default:
	}
	return ResumeDriveResult{Resumed: true}, nil
}

func (s *Server) clearStyle(ctx context.Context) (ClearStyleResult, error) {
	if err := s.Engine.ClearStyle(ctx); err != nil {
		return ClearStyleResult{}, err
	}
	return ClearStyleResult{Cleared: true}, nil
}

// history is what VANS has done to this machine.
//
// Read-only and ungated. A person is entitled to know what a program acting on
// their behalf has changed, without asking it nicely.
func (s *Server) history(ctx context.Context) (HistoryResult, error) {
	if s.Deeds == nil {
		return HistoryResult{}, nil
	}

	deeds, err := s.Deeds.RecentDeeds(ctx, ShownDeeds)
	if err != nil {
		return HistoryResult{}, err
	}

	out := make([]Deed, 0, len(deeds))
	for _, d := range deeds {
		out = append(out, Deed{
			ID: d.ID, At: d.At.Unix(), Kind: d.Kind, Where: d.Where,
			Asked: d.Asked, Did: d.Did, Changed: d.Changed,
			Verified: d.Verified, Undoable: d.Undoable(), Undone: !d.UndoneAt.IsZero(),
		})
	}
	return HistoryResult{Deeds: out}, nil
}

// undo puts one recorded change back.
//
// No confirmation gate. Undoing is not a side effect anyone needs protecting
// from, and asking twice before putting something back teaches people to dismiss
// the asking.
func (s *Server) undo(ctx context.Context, req Request) (UndoResult, error) {
	var params UndoParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return UndoResult{}, &Error{
			Code:    CodeBadRequest,
			Message: fmt.Sprintf("rpc: undo takes an id: %v", err),
		}
	}
	if params.ID == 0 {
		return UndoResult{}, &Error{Code: CodeBadRequest, Message: "rpc: undo takes an id"}
	}

	deed, err := s.Engine.Undo(ctx, params.ID)
	if err != nil {
		return UndoResult{}, err
	}
	return UndoResult{Undone: true, Did: deed.Did}, nil
}

func (s *Server) screenContext(ctx context.Context, req Request) (ScreenContextResult, error) {
	var params ScreenContextParams
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return ScreenContextResult{}, &Error{
				Code:    CodeBadRequest,
				Message: fmt.Sprintf("invalid screen_context params: %v", err),
			}
		}
	}

	target := params.AppName
	if target == "" {
		target = params.BundleID
	}

	return ScreenContextResult{
		Handled: true,
		Target:  target,
	}, nil
}

// compile-time assurance that runner rows serialize as plain maps.
var _ = func() bool {
	var r runner.Row
	_ = map[string]string(r)
	return true
}()
