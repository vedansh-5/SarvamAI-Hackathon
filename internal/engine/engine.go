// Package engine orchestrates a task from instruction to verified side effect.
//
// It is the only component that coordinates others, and it holds no domain
// logic of its own: navigation belongs to the adapter, structure to extract,
// judgement to runner, memory to store, and the single model call to synth.
// What lives here is the order those happen in, the failure ladder, and the
// gate in front of anything that touches the world.
//
// The engine never launches an application or a browser. It attaches to what
// is already running and fails with an explanation when nothing is. A runtime
// that opens apps on the user's behalf to satisfy its own plan is doing
// something the user did not ask for, and doing it before any confirmation
// gate has been reached.
package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/capability"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/runner"
	"github.com/vedansh/vans/internal/store"
	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

// Browser is the page surface the engine drives.
//
// An implementation is responsible for its own reconnection: the engine
// retries an operation that reports itself transient, and expects the next
// call to have re-established whatever broke.
type Browser interface {
	Navigate(ctx context.Context, pageURL string) error
	URL(ctx context.Context) (string, error)
	OuterHTML(ctx context.Context) (string, error)
}

// Document is what gets written.
type Document struct {
	Title  string
	Fields []string
	Rows   []runner.Row
}

// Receipt is evidence a write landed.
type Receipt struct {
	ID    string
	Title string

	// Verified reports whether the sink read the write back after making it
	// (D-013). A false here is not a failure — it means the tier could not
	// confirm, and the caller should say so rather than claim success.
	Verified bool
}

// Task is a user instruction.
type Task struct {
	// URL is the page to extract from.
	URL string

	// Intent is the plain-language description of what to extract.
	Intent string

	// Title names the document that will be written.
	Title string

	// Destination is where the answer goes. Empty means the panel, which
	// writes nothing — so a task that says nothing about its destination
	// cannot accidentally acquire a side effect.
	Destination string
}

// Engine runs tasks.
type Engine struct {
	Browser Browser
	Store   *store.Store
	Synth   synth.Synthesizer

	// Router turns a free-text instruction into a task. Optional: Prepare
	// takes an already-resolved task and does not need one. Ask does.
	Router *route.Router

	// Sinks are the destinations that write somewhere, by name. The panel is
	// not among them — it is the absence of a write, not a kind of one.
	Sinks map[string]Sink

	// Apps, Writer, Actions and Runner are what it takes to act inside an
	// application. All optional and all four required together: a build with
	// some of them declines app.act rather than failing partway through one.
	// Answers replies to a question from rows already extracted. Optional: a
	// build without one shows the rows and nothing above them.
	Answers Answerer

	Apps   *capability.Catalog
	Writer ActionWriter

	// WebWriter drives a page the user has open, for the applications that
	// ship no scripting dictionary. Optional.
	WebWriter WebActionWriter
	Actions   ActionCache
	Runner    appact.Runner

	UIWriter  UIActionWriter
	UIRunner  uiact.Runner
	UIActions UIActionCache
	Steps     StepWriter

	// Model labels which backend wrote a program, stored as provenance.
	Model string

	// MaxTransientRetries bounds rung 0. Defaults to DefaultTransientRetries.
	MaxTransientRetries int

	// MaxSynthesis bounds how many model calls one task may cost. Defaults to
	// DefaultMaxSynthesis: the first attempt, plus one retry informed by why
	// the first was rejected.
	MaxSynthesis int

	// Log receives progress lines. Optional.
	Log func(format string, args ...any)
}

// Ladder budgets (D-020).
const (
	DefaultTransientRetries = 2
	DefaultMaxSynthesis     = 2
)

func (e *Engine) logf(format string, args ...any) {
	if e.Log != nil {
		e.Log(format, args...)
	}
}

func (e *Engine) transientRetries() int {
	if e.MaxTransientRetries > 0 {
		return e.MaxTransientRetries
	}
	return DefaultTransientRetries
}

func (e *Engine) maxSynthesis() int {
	if e.MaxSynthesis > 0 {
		return e.MaxSynthesis
	}
	return DefaultMaxSynthesis
}

// Attempt records one program the engine tried and why it was rejected.
type Attempt struct {
	// Source is "cache" or "synthesis".
	Source  string
	Program extract.Program
	Reasons []string
}

// UnresolvedError is rung 3: every rung below it has been spent.
//
// It carries what was tried rather than a summary, because the point of rung 3
// is that the runtime has stopped guessing and is handing the problem to a
// person. A person needs the candidates that were found and the predicate that
// failed, not "extraction failed".
type UnresolvedError struct {
	PageURL    string
	Signature  string
	Attempts   []Attempt
	Candidates []extract.Candidate
}

func (e *UnresolvedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "engine: could not extract from %s after %d attempts", e.PageURL, len(e.Attempts))
	for i, a := range e.Attempts {
		fmt.Fprintf(&b, "\n  attempt %d (%s, root %q): %s",
			i+1, a.Source, a.Program.Root, strings.Join(a.Reasons, "; "))
	}
	if len(e.Attempts) == 0 {
		fmt.Fprintf(&b, "\n  %d candidate regions were found on the page", len(e.Candidates))
	}
	return b.String()
}

// Plan is a prepared task: everything has been extracted and verified, and
// nothing has touched the world yet.
type Plan struct {
	Task      Task
	PageURL   string
	Signature string

	Fields []string
	Rows   []runner.Row

	// SynthesisCalls is how many times the model was asked during Prepare.
	// Zero is the expected value on a warm run and the number worth watching.
	SynthesisCalls int

	// FromCache reports whether the program that produced these rows came out
	// of the store rather than being written for this run.
	FromCache bool

	// Destination is where the answer goes. Empty means the panel, which
	// writes nothing.
	Destination string

	// Route is how the instruction was understood, when one was routed.
	// Nil for a task that arrived already resolved.
	Route *route.Route

	// DecisionCalls is how many model calls the routing cost. Zero on a warm
	// shape, the same way SynthesisCalls is zero on a warm page.
	DecisionCalls int

	// Answer is a reply to the question, grounded in the rows. Nil when the
	// task was not a question, or when nothing survived being checked against
	// the results.
	Answer *answer.Answer

	// AnswerCalls is what the answer cost. Unlike everything else it is never
	// zero on a warm run: routing and extraction are about a shape and cache,
	// an answer is about this content and cannot.
	AnswerCalls int

	// Web is set when this plan acts inside a page rather than an
	// application. Mutually exclusive with Action.
	Web *WebPlan

	// Action is set when this plan acts inside an application rather than
	// extracting from a page. Rows are empty in that case: there is nothing
	// to show as a table, and the preview is what gets confirmed.
	Action *ActionPlan

	UI *UIPlan

	Drive *DrivePlan

	key       store.Key
	programID int64
	committed bool
}

// Prepare navigates, extracts, and verifies. It never writes anything.
//
// The split between Prepare and Commit is the confirmation gate (D-004). It is
// deliberately the shape of the API rather than a flag on a single call: a
// caller cannot forget to check a boolean it was never given, and the only way
// to reach a side effect is to have received a Plan, shown it to someone, and
// called Commit on purpose.
func (e *Engine) Prepare(ctx context.Context, task Task) (*Plan, error) {
	if strings.TrimSpace(task.URL) == "" {
		return nil, fmt.Errorf("engine: task has no URL")
	}
	if strings.TrimSpace(task.Intent) == "" {
		return nil, fmt.Errorf("engine: task has no intent")
	}

	src, pageURL, err := e.snapshot(ctx, task.URL)
	if err != nil {
		return nil, err
	}

	sig, err := extract.SignatureOf(src, extract.NewSigOptions())
	if err != nil {
		return nil, fmt.Errorf("engine: sign page: %w", err)
	}
	key, err := store.NewKey(pageURL, sig, task.Intent)
	if err != nil {
		return nil, err
	}
	e.logf("page %s signature %s", pageURL, sig)

	return e.extractFrom(ctx, src, pageURL, sig, key, task)
}

// extractFrom is everything after a page is in hand: the cache, the ladder,
// the model, the verifier.
//
// Split out so that a page the runtime navigated to and a page the user
// already had open go through exactly the same path. Two copies of the ladder
// would drift, and the one that drifted would be the one nobody was watching.
func (e *Engine) extractFrom(
	ctx context.Context,
	src, pageURL, sig string,
	key store.Key,
	task Task,
) (*Plan, error) {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, fmt.Errorf("engine: parse page URL: %w", err)
	}
	opts := runner.Options{Base: base}

	var attempts []Attempt

	// Rung 1 territory. Every cached program is tried before the model is
	// considered — see D-029: a program that failed once may be right for a
	// variant the signature collapsed, so the cache is exhausted first.
	cached, err := e.Store.Lookup(ctx, key)
	if err != nil {
		return nil, err
	}
	for _, entry := range cached {
		res, runErr := runner.Execute(src, entry.Program, opts)
		if runErr == nil {
			if err := e.Store.RecordRun(ctx, entry.ID, true); err != nil {
				return nil, err
			}
			e.logf("cache hit: %d rows, no model call", len(res.Rows))
			return &Plan{
				Task: task, PageURL: pageURL, Signature: sig,
				Fields: res.Fields, Rows: res.Rows,
				FromCache: true, key: key, programID: entry.ID,
				Destination: task.Destination,
			}, nil
		}

		if err := e.Store.RecordRun(ctx, entry.ID, false); err != nil {
			return nil, err
		}
		reasons := runner.Failures(runErr)
		if reasons == nil {
			reasons = []string{runErr.Error()}
		}
		attempts = append(attempts, Attempt{Source: "cache", Program: entry.Program, Reasons: reasons})
		e.logf("cached program rejected: %s", strings.Join(reasons, "; "))
	}

	// Rungs 1 and 2 converge here. A cache miss and an invalidated cache take
	// the same synthesis path, so there is one implementation of it, reached
	// from both directions.
	candidates, err := extract.FindCandidates(src, extract.Options{})
	if err != nil {
		return nil, fmt.Errorf("engine: find candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil, &UnresolvedError{PageURL: pageURL, Signature: sig, Attempts: attempts}
	}

	plan := &Plan{Task: task, PageURL: pageURL, Signature: sig, key: key, Destination: task.Destination}

	for plan.SynthesisCalls < e.maxSynthesis() {
		req := synth.Request{
			Intent:     task.Intent,
			URL:        pageURL,
			Candidates: candidates,
			Failures:   reasonsFrom(attempts),
		}

		program, err := e.Synth.Synthesize(ctx, req)
		plan.SynthesisCalls++
		if err != nil {
			return nil, fmt.Errorf("engine: synthesis failed: %w", err)
		}

		res, runErr := runner.Execute(src, program, opts)
		if runErr != nil {
			reasons := runner.Failures(runErr)
			if reasons == nil {
				reasons = []string{runErr.Error()}
			}
			attempts = append(attempts, Attempt{Source: "synthesis", Program: program, Reasons: reasons})
			e.logf("synthesized program rejected: %s", strings.Join(reasons, "; "))
			continue
		}

		// Only a program that verified is cached. Storing one that failed
		// would put a known-bad program at the front of a future lookup.
		id, err := e.Store.Put(ctx, key, program, task.Intent, e.Model)
		if err != nil {
			return nil, err
		}
		if err := e.Store.RecordRun(ctx, id, true); err != nil {
			return nil, err
		}

		plan.Fields, plan.Rows, plan.programID = res.Fields, res.Rows, id
		e.logf("synthesized a working program: %d rows, %d model call(s)", len(res.Rows), plan.SynthesisCalls)
		return plan, nil
	}

	return nil, &UnresolvedError{
		PageURL: pageURL, Signature: sig,
		Attempts: attempts, Candidates: candidates,
	}
}

// snapshot navigates and captures the DOM, retrying transient failures.
//
// This is rung 0: a dropped CDP socket or a relaunched app is a reconnect, not
// a task failure, and it costs no model call.
func (e *Engine) snapshot(ctx context.Context, target string) (src, pageURL string, err error) {
	// One browser drives one tab, and navigating is a state change on shared
	// state. Two requests in flight at once send the tab to different pages
	// and then each read whichever won — surfacing as one failing with
	// net::ERR_ABORTED and, worse because it is silent, the other reading a
	// page it never asked for and signing it as its own.
	//
	// The lock lives on the browser rather than here because that is what it
	// protects, and because the RPC server hands each request its own copy of
	// the Engine to attach a logger to: a lock held in this struct would be
	// copied along with it and guard nothing.
	//
	// Held across navigate, read-back and capture, since those three are one
	// operation. A mutex rather than a retry — retrying an aborted navigation
	// races the same way. A tab per request is the other answer, and the one
	// to reach for when concurrency is worth having rather than survivable.
	if l, ok := e.Browser.(sync.Locker); ok {
		l.Lock()
		defer l.Unlock()
	}

	attempts := e.transientRetries() + 1

	for i := 0; i < attempts; i++ {
		if i > 0 {
			e.logf("transient failure, retrying (%d/%d): %v", i, e.transientRetries(), err)
			select {
			case <-ctx.Done():
				return "", "", ctx.Err()
			case <-time.After(time.Duration(i) * 500 * time.Millisecond):
			}
		}

		if err = e.Browser.Navigate(ctx, target); err != nil {
			if IsTransient(err) {
				continue
			}
			return "", "", fmt.Errorf("engine: navigate to %s: %w", target, err)
		}

		// The URL is read back rather than assumed. Navigation is a state
		// change with an observable result (D-004), and a redirect to a login
		// wall is the case that matters: extracting from it would produce a
		// confidently wrong answer.
		if pageURL, err = e.Browser.URL(ctx); err != nil {
			if IsTransient(err) {
				continue
			}
			return "", "", fmt.Errorf("engine: read page URL: %w", err)
		}

		if src, err = e.Browser.OuterHTML(ctx); err != nil {
			if IsTransient(err) {
				continue
			}
			return "", "", fmt.Errorf("engine: capture DOM: %w", err)
		}
		return src, pageURL, nil
	}

	return "", "", fmt.Errorf("engine: %s did not respond after %d attempts: %w", target, attempts, err)
}

// Commit performs the write. It is the only method that touches the world.
//
// A plan is committed at most once, ever. D-020's hard rule is that an
// executed write is never retried — verification ambiguity is not grounds for
// writing twice — so a second call is refused rather than being allowed to
// create a duplicate note the user has no reason to expect.
func (e *Engine) Commit(ctx context.Context, plan *Plan) (Receipt, error) {
	if plan == nil {
		return Receipt{}, fmt.Errorf("engine: no plan to commit")
	}
	if plan.committed {
		return Receipt{}, fmt.Errorf("engine: this plan has already been committed; writes are never retried")
	}
	if plan.Web != nil {
		plan.committed = true
		return e.runWeb(ctx, plan)
	}

	if plan.UI != nil {
		plan.committed = true
		return e.runUI(ctx, plan)
	}

	if plan.Drive != nil {
		plan.committed = true
		return e.runDrive(ctx, plan)
	}

	if plan.Action != nil {
		// Set before the call, not after: if the program panics or the
		// process dies mid-flight the plan is still spent, because the change
		// may have happened.
		plan.committed = true
		return e.runAction(ctx, plan)
	}

	if len(plan.Rows) == 0 {
		return Receipt{}, fmt.Errorf("engine: refusing to write an empty document")
	}

	// Resolved before the plan is spent, so a plan is not burned by a typo in
	// a destination name.
	sink, err := e.sinkFor(plan.Destination)
	if err != nil {
		return Receipt{}, err
	}

	title := plan.Task.Title
	if strings.TrimSpace(title) == "" {
		title = defaultTitle(plan)
	}

	// Set before the call, not after. If the write panics or the process dies
	// mid-flight the plan is still spent, because the note may exist.
	plan.committed = true

	receipt, err := sink.Write(ctx, Document{
		Title:  title,
		Fields: plan.Fields,
		Rows:   plan.Rows,
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("engine: write failed: %w", err)
	}

	e.logf("wrote %q (%d rows), verified=%v", receipt.Title, len(plan.Rows), receipt.Verified)
	return receipt, nil
}

// Committed reports whether this plan has been spent.
func (p *Plan) Committed() bool { return p.committed }

// NeedsCommit reports whether this plan still has to touch the world.
//
// False for the panel: the rows are already in hand, so the answer has been
// delivered by the time Prepare returns. The gate is shown on the strength of
// this rather than unconditionally — a confirmation in front of something with
// no side effect teaches people to dismiss confirmations.
func (p *Plan) NeedsCommit() bool {
	// An action asks the dictionary. A program that only reads has nothing to
	// confirm, and a confirmation in front of an action with no side effect
	// teaches people to dismiss confirmations.
	if p.Action != nil {
		return p.Action.Writes
	}

	// A page declares nothing, so there is no dictionary to ask whether a
	// click sends a message or opens a menu. Every page program is confirmed.
	if p.Web != nil {
		return true
	}

	if p.UI != nil {
		return true
	}

	if p.Drive != nil {
		return true
	}
	return p.Destination != "" && p.Destination != PanelDestination
}

func defaultTitle(plan *Plan) string {
	host := plan.PageURL
	if u, err := url.Parse(plan.PageURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return fmt.Sprintf("%s — %s", strings.TrimSpace(plan.Task.Intent), host)
}

// reasonsFrom flattens every rejection so far into synthesis input.
//
// All of them, not just the last: a model that only hears about the most
// recent failure can re-propose something it already had rejected, and each
// round trip is a model call the runtime exists to avoid.
func reasonsFrom(attempts []Attempt) []string {
	var out []string
	for _, a := range attempts {
		for _, r := range a.Reasons {
			out = append(out, fmt.Sprintf("%s (program with root %q): %s", a.Source, a.Program.Root, r))
		}
	}
	return out
}

// transient marks an error worth retrying without any model involvement.
type transient interface{ Transient() bool }

// IsTransient reports whether err is a reconnect rather than a task failure.
//
// A cancelled context is never transient. It means the caller gave up or a
// deadline passed, and retrying past that is how a runtime ignores the person
// using it.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var t transient
	return errors.As(err, &t) && t.Transient()
}

// TransientError wraps an error as retryable. Adapters use it to tell the
// engine that a failure is theirs to recover from.
type TransientError struct{ Err error }

func (e *TransientError) Error() string   { return e.Err.Error() }
func (e *TransientError) Unwrap() error   { return e.Err }
func (e *TransientError) Transient() bool { return true }
