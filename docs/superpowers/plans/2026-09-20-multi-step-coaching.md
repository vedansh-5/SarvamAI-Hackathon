# Multi-Step Coaching Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement robust multi-step coaching with automatic change detection (focus, value, window title), manual step advance ("Next"), clean cancellation lifecycle ("Stop"), and consecutive-failure window guards.

**Architecture:** The Go engine retains single-source-of-truth task progression via `stepThroughTask` and `RunGuide`. The session-scoped advance channel and cancellation func are owned by `rpc.Server` under `s.mu`. Tauri shell exposes `advance_guide` and `stop_guide` commands to the HUD.

**Tech Stack:** Go 1.24, Tauri v2 (Rust), TypeScript, Vanilla HTML/CSS.

## Global Constraints
- Minimal and scoped changes with minimal comments; name functions functionally so they explain themselves.
- Signed-off commits (`git commit -s`).
- Never commit directly to `main`; work on branch `multi-step-coaching`.
- Do NOT merge without explicit user command.
- Verification gate before shipping:
  ```bash
  go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...
  gofmt -l internal/ cmd/
  cd ui && npx tsc --noEmit && cd src-tauri && cargo check
  ```
- Rebuild engine and app after changes:
  ```bash
  go build -o ui/vans-engine ./cmd/vans-engine && cd ui && npm run build
  cd ui && npm run tauri dev
  ```

---

### Task 1: Focus & Window Title Tracking in `uiact.Changes` and Decision D-073

**Files:**
- Modify: `internal/uiact/changes.go`
- Modify: `internal/uiact/changes_test.go`
- Modify: `docs/decisions.md`

**Interfaces:**
- Produces: `uiact.Changes(before, after Tree) []string` returning `"the <role> <title> is now focused"` and `"window title changed to <title>"`.

- [ ] **Step 1: Write failing tests in `changes_test.go`**

Add tests for focus change and window title change.
```go
func TestChangesReportsFocusChange(t *testing.T) {
	before := treeOf(Element{Role: "AXTextField", Title: "Search", Focused: false})
	after := treeOf(Element{Role: "AXTextField", Title: "Search", Focused: true})

	got := Changes(before, after)
	if len(got) != 1 || !strings.Contains(got[0], "now focused") {
		t.Errorf("changes = %v, want focus change", got)
	}
}

func TestChangesReportsWindowTitleChange(t *testing.T) {
	before := Tree{App: "Settings", Window: "General"}
	after := Tree{App: "Settings", Window: "Displays"}

	got := Changes(before, after)
	if len(got) != 1 || !strings.Contains(got[0], `window title changed to "Displays"`) {
		t.Errorf("changes = %v, want window title change", got)
	}
}
```

- [ ] **Step 2: Run test to verify**

Run: `go test ./internal/uiact/...`

- [ ] **Step 3: Implement minimal code in `changes.go` and add D-073 to `docs/decisions.md`**

In `internal/uiact/changes.go`:
```go
	if before.Window != "" && after.Window != "" && before.Window != after.Window {
		out = append(out, fmt.Sprintf("window title changed to %q", after.Window))
	}
```
And inside `for k, el := range now`:
```go
		if !prev.Focused && el.Focused {
			out = append(out, fmt.Sprintf("the %s %q is now focused", el.Role, el.Title))
		}
```
In `docs/decisions.md`: add decision record D-073 explaining focus movement as confirmation of user action.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/uiact/...`

- [ ] **Step 5: Commit**

```bash
git add internal/uiact/ docs/decisions.md
git commit -s -m "uiact: detect focus transitions and window title changes"
```

---

### Task 2: Engine Multi-Step Guide Loop with Session Advance & Consecutive-Failure Guard

**Files:**
- Modify: `internal/engine/guide.go`
- Modify: `internal/engine/guide_test.go`

**Interfaces:**
- Consumes: `advance <-chan struct{}`
- Produces: `RunGuide(ctx context.Context, plan *Plan, show func(GuideStep), advance <-chan struct{}) (int, error)`

- [ ] **Step 1: Write failing tests in `internal/engine/guide_test.go`**

Add tests for:
- Advancing via window change across 2 steps.
- Advancing via `advance` channel.
- Tolerating transient capture errors up to 2 times before failing on the 3rd.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/... -run TestGuide`

- [ ] **Step 3: Implement minimal code in `internal/engine/guide.go`**

- Update `RunGuide` signature to accept `advance <-chan struct{}`.
- In `waitForTheUserToAct`:
  - Non-blocking drain of `advance`: `select { case <-advance: default: }`
  - Track `consecutiveFailures int`
  - In loop, select on `ctx.Done()`, `advance`, `time.After(guidePoll)`
  - If `advance` receives: return `nil`
  - If `Capture` errors or returns 0 elements: increment `consecutiveFailures`; if `>= 3`, return error. Else reset `consecutiveFailures = 0` on valid tree.
  - If `len(uiact.Changes(before, after)) > 0`, return `nil`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/... -run TestGuide`

- [ ] **Step 5: Commit**

```bash
git add internal/engine/guide.go internal/engine/guide_test.go
git commit -s -m "engine: support session advance channel and consecutive failure guard in guide"
```

---

### Task 3: RPC Server Guide Session Lifecycle & Methods (`stop_guide`, `advance_guide`)

**Files:**
- Modify: `internal/rpc/server.go`
- Modify: `internal/rpc/server_test.go`

**Interfaces:**
- Produces:
  - RPC method `"stop_guide"`: cancels active guide context, returns `{"stopped": true}`
  - RPC method `"advance_guide"`: sends to `guideAdvance`, returns `{"advanced": true}`
  - RPC method `"guide"`: allocates `guideAdvance`, passes to `RunGuide`, handles `context.Canceled` cleanly.

- [ ] **Step 1: Write failing tests in `internal/rpc/server_test.go`**

Add test verifying `advance_guide` signals active guide, and `stop_guide` terminates guide cleanly without returning error to client.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rpc/... -run TestGuide`

- [ ] **Step 3: Implement minimal code in `internal/rpc/server.go`**

- Add `guideCancel context.CancelFunc` and `guideAdvance chan struct{}` to `Server`.
- In `handle`: register `"stop_guide"` and `"advance_guide"`.
- In `guide`:
  - Under `s.mu`, create `guideAdvance = make(chan struct{}, 1)`, derive `guideCtx, guideCancel = context.WithCancel(ctx)`, store on `s`.
  - Defer clearing under `s.mu`.
  - Pass `guideAdvance` to `s.Engine.RunGuide(guideCtx, plan, show, guideAdvance)`.
  - If `errors.Is(err, context.Canceled)`, return `GuideResult{Steps: steps, Stopped: true}, nil`.
- Implement `stopGuide` and `advanceGuide` methods.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/rpc/...`

- [ ] **Step 5: Commit**

```bash
git add internal/rpc/
git commit -s -m "rpc: add stop_guide and advance_guide with session lifecycle"
```

---

### Task 4: Shell Integration (Tauri Commands & Guiding Banner UI)

**Files:**
- Modify: `ui/src-tauri/src/lib.rs`
- Modify: `ui/index.html`
- Modify: `ui/src/main.ts`

- [ ] **Step 1: Update `ui/src-tauri/src/lib.rs`**

Add Tauri commands:
- `stop_guide`: calls engine RPC `"stop_guide"`.
- `advance_guide`: calls engine RPC `"advance_guide"`.
Register both in `tauri::generate_handler![..., stop_guide, advance_guide]`.

- [ ] **Step 2: Update `ui/index.html`**

In `<div id="screen-guiding">`:
Add "Next" button:
```html
<button id="guiding-next" class="btn">Next</button>
<button id="guiding-stop" class="btn quiet">Stop</button>
```

- [ ] **Step 3: Update `ui/src/main.ts`**

- Wire `$("guiding-next").addEventListener("click", () => invoke("advance_guide").catch(console.error))`
- Wire `$("guiding-stop").addEventListener("click", async () => { await invoke("stop_guide").catch(console.error); await invoke("clear_pointer").catch(console.error); show("ask"); })`
- Update Escape key handler to trigger `stop_guide` if current view is `"guiding"`.
- In `showTheWay`: ignore `stopped` without showing failure.

- [ ] **Step 4: Verify TypeScript compilation & Cargo check**

Run:
```bash
cd ui && npx tsc --noEmit
cd src-tauri && cargo check
```

- [ ] **Step 5: Commit**

```bash
git add ui/src-tauri/src/lib.rs ui/index.html ui/src/main.ts
git commit -s -m "ui: wire manual advance and clean stop for guiding HUD"
```

---

### Task 5: Full Verification Gate, Rebuild Engine & Launch App

**Files:**
- None (verification & build)

- [ ] **Step 1: Run full verification gate**

```bash
go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...
gofmt -l internal/ cmd/
cd ui && npx tsc --noEmit
cd src-tauri && cargo check
```

- [ ] **Step 2: Rebuild engine and UI bundle**

```bash
go build -o ui/vans-engine ./cmd/vans-engine && cd ui && npm run build
```

- [ ] **Step 3: Launch live application**

```bash
cd ui && npm run tauri dev
```

- [ ] **Step 4: Push branch & prepare PR**

```bash
git push -u origin multi-step-coaching
```
Draft PR description following guidelines (problem statement, solution & technical details, verification). Wait for user instruction before merging.
