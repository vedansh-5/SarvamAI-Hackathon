# App Agent Workflow Design (§9.4)

## 1. Overview
The App Agent workflow (`drive`) allows VANS to carry out multi-step procedural tasks across native applications on screen, one step at a time.
With input posting and read-back both operational (#63), the drive loop can now be fully exercised end-to-end:
- Each step is planned against the current window state.
- The action is posted via `RunScreen` with target verification.
- Read-back via `uiact.Changes` captures what changed and feeds `"nothing in the window changed"` or the change summary back into the subsequent step's prompt.
- Progress is streamed live to the user interface during execution.

## 2. Architecture & Technical Design

### 2.1 Progress Streaming During `Commit`
In `internal/rpc/server.go`:
- Currently, `server.go:commit()` calls `s.Engine.Commit()` directly with no progress notification handler attached, leaving the UI silent while the drive loop executes multiple steps.
- In `server.go:commit()`, copy `eng := *s.Engine` and wire:
  ```go
  eng.Log = func(format string, args ...any) { s.notify(req.ID, format, args...) }
  ```
- In `ui/src/main.ts`:
  - When `engine://progress` notifications arrive while the commit action is in flight, update the disabled commit button text (or status display) to reflect the active step (e.g. `"working out step 1"`, `"step 1: Click Search"`, `"finished after 2 step(s)"`).

### 2.2 Verified Receipt On Clean Completion
In `internal/engine/drive.go`:
- `runDrive` previously returned `Receipt{Title: d.App, Verified: false}`.
- When `stepThroughTask` completes without error (having reached `answer.Done == true`), return `Receipt{Title: d.App, Verified: true}`.

### 2.3 Failure Feedback and Step Retry
In `internal/engine/drive.go`:
- In `stepThroughTask`, `handle(ctx, tree, program, i, describe)` dispatches the step action.
- If `handle` returns an error (e.g. target control occluded or verification mismatch), append the error message to `failures` and continue the inner attempt loop (`attempt <= 2`).
- The subsequent synthesis request receives `YOUR LAST STEP WAS REJECTED` with the exact reason, allowing the model to choose an alternative action or recovery step rather than crashing the entire drive task immediately.

### 2.4 Live Opt-in Integration Test
In `internal/engine/live_test.go`:
- Tagged with `//go:build integration`.
- Guarded by `VANS_LIVE=1` and `VANS_LIVE_DRIVE_OK=1`, with `VANS_LIVE_APP` (and optional `VANS_LIVE_TASK`).
- Exercises the end-to-end driving path: brings target application forward, drives a live 1-to-2 step task, reads back window state, and validates the receipt.

## 3. Verification Plan
- Unit tests:
  - `internal/engine/drive_test.go`:
    - Test that `runDrive` returns `Verified: true` when steps complete successfully.
    - Test that `stepThroughTask` retries on step execution failure and passes failure reasoning to the next attempt.
  - `internal/rpc/server_test.go`:
    - Test that `commit` emits progress notifications over the RPC channel during plan execution.
- Build & Verification Gate:
  - `go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...`
  - `gofmt -l internal/ cmd/`
  - `cd ui && npx tsc --noEmit && cd src-tauri && cargo check`
- Live Verification:
  - Rebuild engine binary (`go build -o ui/vans-engine ./cmd/vans-engine && cd ui && npm run build`)
  - Run `npm run tauri dev` and test drive tasks live.
