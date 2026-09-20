# Multi-Step Coaching Design

## 1. Overview
Multi-step coaching guides a user through complex tasks across changing application states on screen.
The Go engine acts as the authoritative state machine (`stepThroughTask`), streaming step coordinates to Tauri's coach window, waiting for user progression (either via accessibility tree change detection or an explicit "Next" HUD button), and terminating cleanly when the task is complete or cancelled ("Stop").

## 2. Architectural Decisions & Corrections

### 2.1 Session-Scoped Advance Channel
`RunGuide` accepts an `advance <-chan struct{}` parameter alongside `show func(GuideStep)`:
```go
func (e *Engine) RunGuide(ctx context.Context, plan *Plan, show func(GuideStep), advance <-chan struct{}) (int, error)
```
The channel is owned by `rpc.Server` for the duration of the guide session.
At the start of each `waitForTheUserToAct` iteration, any buffered advance signal is drained via non-blocking receive (`select { case <-advance: default: }`) to prevent stale clicks during synthesis from skipping steps.

### 2.2 Consecutive Failure Guard for Window Closure
Instead of immediately aborting on a single `err != nil || len(after.Elements) == 0`, `waitForTheUserToAct` requires 3 consecutive failures (approx. 1.5 seconds) before concluding the application window is no longer open. This tolerates transient window redraws, animations, and momentary app switches.

### 2.3 `uiact.Changes` Focus & Window Title Tracking
- Detect `!prev.Focused && el.Focused`: Clicking a text field or search bar now confirms the action, treating focus movement as verified interaction.
- Detect `before.Window != "" && after.Window != "" && before.Window != after.Window`: Window title transitions are recorded as changes.
- Update `docs/decisions.md` with record D-073 (focus change confirmation).

### 2.4 Server Guide Session Lifecycle & Cancellation
In `rpc.Server`:
- Protected under `s.mu`:
  - `guideCancel context.CancelFunc`
  - `guideAdvance chan struct{}`
- Lifecycle:
  - When `guide` RPC is invoked, allocate `guideAdvance = make(chan struct{}, 1)`, derive cancellable context, store `guideCancel`.
  - `defer` clearing `guideCancel` and `guideAdvance` upon completion under `s.mu`.
  - `"stop_guide"` RPC: calls `guideCancel()` if non-nil, returns `{ "stopped": true }`.
  - `"advance_guide"` RPC: performs non-blocking send to `guideAdvance`, returns `{ "advanced": true }`.
  - `guide` RPC handles `context.Canceled` cleanly by returning `{ "steps": steps, "stopped": true }` without an error.

### 2.5 UI & Tauri Shell Integration
- `ui/src-tauri/src/lib.rs`: Expose `stop_guide` and `advance_guide` Tauri commands that call engine RPC.
- `ui/index.html`: Add "Next" button next to "Stop" in the guiding HUD banner.
- `ui/src/main.ts`:
  - Wire `guiding-stop` to invoke `stop_guide` and `clear_pointer`.
  - Wire `guiding-next` to invoke `advance_guide`.
  - Wire Escape key to trigger `stop_guide` when in `guiding` state.

## 3. Verification Plan
- Unit tests:
  - `internal/uiact/changes_test.go`: Focus and window title changes.
  - `internal/engine/guide_test.go`: Multi-step guide loop execution, advance channel signaling, and consecutive failure tolerance.
  - `internal/rpc/server_test.go`: `stop_guide` and `advance_guide` RPC lifecycle and cancellation.
- Build & Verification Gate:
  - `go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...`
  - `gofmt -l internal/ cmd/`
  - `cd ui && npx tsc --noEmit && cd src-tauri && cargo check`
- Live Verification:
  - Rebuild engine (`go build -o ui/vans-engine ./cmd/vans-engine && cd ui && npm run build`)
  - Run app (`cd ui && npm run tauri dev`)
  - Verify multi-step coaching and "Next" / "Stop" behavior.
