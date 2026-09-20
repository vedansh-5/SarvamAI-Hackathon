# App Agent Workflow Implementation Plan

**Goal:** Complete Milestone 9.4 by enabling live progress streaming during plan commit, returning verified receipts upon task completion, adding failure feedback recovery to `stepThroughTask`, and providing an opt-in live drive integration test.

**Architecture:** The Go engine executes multi-step task loops via `stepThroughTask`. In `rpc.Server:commit`, progress is wired to `s.notify` so step transitions stream directly to Tauri's UI. `runDrive` marks receipts verified when all steps finish cleanly. In `stepThroughTask`, step dispatch failures append to `failures` and feed into attempt 2.

**Tech Stack:** Go (engine & RPC), TypeScript / Tauri v2 (shell UI), macOS Accessibility API.

## Global Constraints
- Minimal and scoped changes.
- Minimal comments (G1): no doc comments, no block comments explaining rationale. Functions named functionally.
- Signed-off commits (`git commit -s`).
- Do NOT merge without user approval.

---

### Task 1: Server Commit Progress Logging

**Files:**
- Modify: `internal/rpc/server.go:426-448`
- Test: `internal/rpc/server_test.go`

**Interfaces:**
- `s.notify(id int, format string, args ...any)`
- `s.Engine.Commit(ctx context.Context, plan *engine.Plan) (engine.Receipt, error)`

- [ ] **Step 1: Write the failing test in `internal/rpc/server_test.go`**
Test that `commit` emits progress notifications when `eng.Log` is called during commit.

- [ ] **Step 2: Run test to confirm failure**
Run `go test ./internal/rpc -run TestCommitEmitsProgress` to confirm failure.

- [ ] **Step 3: Implement progress logger wiring in `internal/rpc/server.go`**
In `commit()`, copy `eng := *s.Engine`, set `eng.Log = func(format string, args ...any) { s.notify(req.ID, format, args...) }`, and call `eng.Commit(ctx, plan)`.

- [ ] **Step 4: Run test to confirm success**
Run `go test ./internal/rpc` to verify all tests pass.

- [ ] **Step 5: Commit**
`git commit -s` with clean commit message.

---

### Task 2: Drive Receipt Verification & Step Failure Feedback

**Files:**
- Modify: `internal/engine/drive.go`
- Test: `internal/engine/drive_test.go`

**Interfaces:**
- `runDrive(ctx context.Context, plan *Plan) (Receipt, error)`
- `stepThroughTask(ctx context.Context, app, task string, limit int, handle stepHandler) (int, error)`

- [ ] **Step 1: Write failing tests in `internal/engine/drive_test.go`**
Add tests:
- `TestDriveReturnsVerifiedReceiptWhenDone`: confirms `Receipt.Verified == true` when task completes with `answer.Done`.
- `TestDriveRetriesWhenStepExecutionFails`: confirms that if `handle(...)` returns an error on attempt 1, the failure reason is recorded in `failures` and passed to attempt 2.

- [ ] **Step 2: Run tests to confirm failure**
Run `go test ./internal/engine -run 'TestDriveReturnsVerified|TestDriveRetries'` to verify failures.

- [ ] **Step 3: Implement receipt verification and retry in `internal/engine/drive.go`**
In `runDrive`, set `Verified: true` on receipt when `stepThroughTask` returns `err == nil`.
In `stepThroughTask`, catch actionable `handle(...)` errors, append to `failures`, and continue attempt loop.

- [ ] **Step 4: Run tests to confirm success**
Run `go test ./internal/engine` to verify all tests pass.

- [ ] **Step 5: Commit**
`git commit -s` with clean commit message.

---

### Task 3: UI Progress Update During Commit

**Files:**
- Modify: `ui/src/main.ts:1137-1145`

**Interfaces:**
- Tauri event: `engine://progress`

- [ ] **Step 1: Update `ui/src/main.ts`**
In the `engine://progress` listener, if the `#commit` button is disabled (action running), update its `textContent` with the incoming progress text so the user sees real-time progress.

- [ ] **Step 2: Verify TypeScript compilation**
Run `cd ui && npx tsc --noEmit`.

- [ ] **Step 3: Commit**
`git commit -s` with clean commit message.

---

### Task 4: Live Opt-In Integration Test

**Files:**
- Modify: `internal/engine/live_test.go`

- [ ] **Step 1: Add `TestLiveDriveTask` in `internal/engine/live_test.go`**
Add test behind `//go:build integration` and `VANS_LIVE_DRIVE_OK=1`.

- [ ] **Step 2: Verify integration test compilation**
Run `go test -tags integration ./internal/engine`.

- [ ] **Step 3: Commit**
`git commit -s` with clean commit message.

---

### Task 5: Full Verification Gate & Runtime Restart

- [ ] **Step 1: Run full verification gate**
```bash
go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...
gofmt -l internal/ cmd/
cd ui && npx tsc --noEmit && cd src-tauri && cargo check
```

- [ ] **Step 2: Rebuild binary & UI assets**
```bash
go build -o ui/vans-engine ./cmd/vans-engine && cd ui && npm run build
```

- [ ] **Step 3: Restart dev server daemon**
Kill previous task and launch `npm run tauri dev`.
