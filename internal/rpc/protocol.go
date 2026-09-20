// Package rpc exposes the engine to a desktop shell over a private stdio
// channel.
//
// The transport is newline-delimited JSON on stdin and stdout, and the choice
// of transport is a security decision rather than a convenience one. Commit
// performs irreversible side effects. Serving it over a localhost port — the
// usual way a desktop shell talks to a local backend — would let any process
// on the machine trigger a write: any Electron app, any package install
// script, anything that can open a socket. A pipe is reachable only by the
// process that spawned it, so the confirmation gate cannot be walked around by
// something the user never started.
package rpc

import "encoding/json"

// Request is one call from the shell.
type Request struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response is the reply to a Request. Exactly one of Result and Error is set.
type Response struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Notification is an unsolicited message, used for progress. It carries the
// request it belongs to so a shell running several calls can route it.
type Notification struct {
	Method string          `json:"method"`
	For    int             `json:"for"`
	Params json.RawMessage `json:"params"`
}

// Error codes. These are matched by the shell to decide which view to show,
// so they are part of the contract and not just diagnostic text.
const (
	CodeBadRequest = "bad_request"
	CodeInternal   = "internal"

	// CodeUnresolved is rung 3: extraction was attempted and failed. Data
	// carries UnresolvedData so the shell can show what was tried.
	CodeUnresolved = "unresolved"

	// CodeNoBrowser means no Chromium debugging endpoint answered. Distinct
	// from a generic failure because the fix is a command the user runs, and
	// the runtime will not run it for them (D-032).
	CodeNoBrowser = "no_browser"

	// CodePermission means macOS denied automation access.
	CodePermission = "permission_denied"

	// CodeBrowserScripting means Chrome is open and reachable but will not
	// run JavaScript sent by Apple Events. Its own code because it is the one
	// browser problem whose fix is a menu item rather than a relaunch, and
	// telling someone to start a second browser when this is all that is
	// wrong sends them to do work they do not need to do.
	CodeBrowserScripting = "browser_scripting_off"

	// CodeSpent means a plan was already committed. Writes are never retried.
	CodeSpent = "plan_spent"

	// CodeUnroutable means the instruction was not understood, or names
	// something VANS can recognise but cannot yet carry out. Distinct from
	// CodeUnresolved, which is a request that *was* understood and then
	// failed — the two need entirely different things said to the user.
	CodeUnroutable = "unroutable"
)

// Error is a structured failure.
type Error struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// --- method payloads ---

// PrepareParams asks for whatever the instruction turns out to mean, without
// writing anything.
//
// URL, Intent and Title remain for an already-resolved request — the CLI, and
// any caller that knows exactly what it wants. When Instruction is set it wins,
// and the runtime works the rest out.
type PrepareParams struct {
	// Instruction is free text: what the user typed or said.
	Instruction string `json:"instruction"`

	// Destination names where the answer should go. Empty means the panel,
	// which writes nothing — so a request that says nothing about its
	// destination cannot acquire a side effect by accident.
	Destination string `json:"destination"`

	URL    string `json:"url"`
	Intent string `json:"intent"`
	Title  string `json:"title"`
}

// GuideParams names the guide to run; GuideStepParams is one thing to point at.
type GuideParams struct {
	PlanID string `json:"plan_id"`
}

type GuideResult struct {
	Steps   int  `json:"steps"`
	Stopped bool `json:"stopped,omitempty"`
}

type StopGuideResult struct {
	Stopped bool `json:"stopped"`
}

type AdvanceGuideResult struct {
	Advanced bool `json:"advanced"`
}

type GuideStepParams struct {
	Number int    `json:"number"`
	Label  string `json:"label"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	W      int    `json:"w"`
	H      int    `json:"h"`
}

// ClearStyleResult reports that the page was put back.
type ClearStyleResult struct {
	Cleared bool `json:"cleared"`
}

// PrepareResult is a plan awaiting confirmation.
type PrepareResult struct {
	// PlanID is what Commit takes. The rows are not sent back on commit —
	// the shell confirms the plan it was shown, not data it could alter.
	PlanID string `json:"plan_id"`

	PageURL   string `json:"page_url"`
	Signature string `json:"signature"`
	Title     string `json:"title"`

	Fields []string            `json:"fields"`
	Rows   []map[string]string `json:"rows"`

	SynthesisCalls int   `json:"synthesis_calls"`
	FromCache      bool  `json:"from_cache"`
	ElapsedMS      int64 `json:"elapsed_ms"`

	// Destination is where this plan will go if committed, and NeedsCommit
	// says whether committing is a thing that exists for it. The panel writes
	// nothing, so the window shows no gate — a confirmation in front of an
	// action with no side effect teaches people to dismiss confirmations.
	Destination string `json:"destination"`
	NeedsCommit bool   `json:"needs_commit"`

	// Kind is how the instruction was understood, and DecisionCalls what that
	// understanding cost. Zero on a shape seen before.
	Kind          string `json:"kind"`
	DecisionCalls int    `json:"decision_calls"`

	// Answer is a reply to the question, and Cites the one-based results it
	// rests on. Empty when the task was not a question or nothing survived
	// being checked against the rows.
	Answer      string `json:"answer"`
	AnswerCites []int  `json:"answer_cites"`
	AnswerCalls int    `json:"answer_calls"`

	// Action is what will happen inside an application, in that
	// application's own words. Empty for a plan that reads a page: those are
	// confirmed against the rows themselves.
	Action string `json:"action"`

	App string `json:"app"`
}

// Destination is somewhere an answer can be sent.
type Suggestion struct {
	Icon        string `json:"icon"`
	Text        string `json:"text"`
	Destination string `json:"destination,omitempty"`
}

// SuggestionsParams carries what the user is looking at, so the list can lead
// with what is useful there. Optional: the panel works before the shell can
// read the screen.
type SuggestionsParams struct {
	AppName string `json:"app_name"`
}

type Destination struct {
	Name         string `json:"name"`
	Describe     string `json:"describe"`
	TouchesWorld bool   `json:"touches_world"`
}

// UnroutableData is the runtime not understanding an instruction, with what it
// can do — a refusal that names no alternative is a dead end.
type UnroutableData struct {
	Instruction string   `json:"instruction"`
	Why         string   `json:"why"`
	Can         []string `json:"can"`
}

// CommitParams confirms a plan. This is the only method with a side effect.
type CommitParams struct {
	PlanID string `json:"plan_id"`
}

// CommitResult is the receipt.
type CommitResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Verified bool   `json:"verified"`
}

// UnresolvedData is rung 3 rendered for a person.
type UnresolvedData struct {
	PageURL    string          `json:"page_url"`
	Signature  string          `json:"signature"`
	Attempts   []AttemptData   `json:"attempts"`
	Candidates []CandidateData `json:"candidates"`
}

// AttemptData is one program that was tried and rejected.
type AttemptData struct {
	Source  string   `json:"source"`
	Root    string   `json:"root"`
	Span    int      `json:"span"`
	Reasons []string `json:"reasons"`
}

// CandidateData is a repeating region that was found on the page.
type CandidateData struct {
	Selector    string   `json:"selector"`
	Span        int      `json:"span"`
	RecordCount int      `json:"record_count"`
	Samples     []string `json:"samples"`
}

// Check is one preflight result.
type Check struct {
	// Name identifies the check to the shell.
	Name string `json:"name"`

	// Label is what a person reads.
	Label string `json:"label"`

	OK     bool   `json:"ok"`
	Detail string `json:"detail"`

	// Fix is the action the user takes, when there is one. It is instruction
	// text, not something the runtime performs.
	Fix string `json:"fix,omitempty"`

	// Required reports whether a task can run at all without this.
	Required bool `json:"required"`
}

// PreflightResult is the readiness of the environment.
type PreflightResult struct {
	Ready  bool    `json:"ready"`
	Checks []Check `json:"checks"`
}

// LogParams is a progress line.
type LogParams struct {
	Message string `json:"message"`
}

// ScreenContextParams contains the frontmost app metadata and optional screen frame.
type ScreenContextParams struct {
	AppName     string  `json:"app_name"`
	BundleID    string  `json:"bundle_id"`
	WindowTitle string  `json:"window_title"`
	PID         int32   `json:"pid"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	FramePath   string  `json:"frame_path,omitempty"`
	ImageBase64 string  `json:"image_base64,omitempty"`
}

// ScreenContextResult is returned by the engine after processing screen context.
type ScreenContextResult struct {
	Handled bool   `json:"handled"`
	Target  string `json:"target,omitempty"`
}
