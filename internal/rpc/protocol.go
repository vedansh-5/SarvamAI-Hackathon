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

	// CodeSpent means a plan was already committed. Writes are never retried.
	CodeSpent = "plan_spent"
)

// Error is a structured failure.
type Error struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// --- method payloads ---

// PrepareParams asks for an extraction, without writing anything.
type PrepareParams struct {
	URL    string `json:"url"`
	Intent string `json:"intent"`
	Title  string `json:"title"`
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
