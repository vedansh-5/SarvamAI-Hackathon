# Project Instructions & Working Guidelines

A durable log of all instructions, constraints, and practices established for VANS across sessions.
Read before each task to maintain alignment.

---

## 1. Workflow & Pull Requests

- **All work happens on branches and lands via PRs.** Never commit directly to `main`.
- **Work in byte-sized PRs.** Break features down into small, scoped milestones.
- **Minimal and scoped changes.** Change only what is required for the milestone.
- **Minimal comments.** No doc comments on functions, no block comments explaining rationale. Name functions functionally so their names explain what they do. Rationale belongs in `docs/decisions.md` or PR descriptions.
- **Signed-off commits.** Every commit must be signed off (`git commit -s` or `Signed-off-by`).
- **Squash commits before merging.**
- **Merge only when explicitly instructed.** Do NOT merge pull requests automatically; wait for the user to review and ask to merge.
- **PR descriptions must be concise and structured:**
  - Brief scope summary.
  - Problem statement (what was broken or missing).
  - How it was solved (technical details).
  - Verification commands and results.
- **No attribution trailers or AI signatures** in commits, PR titles, PR bodies, or code. Everything reads as written by its developer.

---

## 2. Testing & Verification Gate

Before opening any PR, the full gate must pass with clean exit codes:

```bash
go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...
gofmt -l internal/ cmd/        # must print nothing
cd ui && npx tsc --noEmit
cd ui/src-tauri && cargo build  # when Rust shell changes
```

- Verify using exit codes (`if cmd; then ...`), never `cmd && echo OK`.
- Live tests stay behind build tags and double opt-ins (`-tags integration`, `VANS_LIVE=1`).
- Never test destructive actions or raw probes on the user's running applications.
- **Always rebuild and restart VANS after every change:** Stop any running instance, build both the Go engine binary and frontend bundle (`go build -o ui/vans-engine ./cmd/vans-engine && cd ui && npm run build`), and restart `cd ui && npm run tauri dev` immediately so the user can test every change live.

---

## 3. Core Architecture & Constraints

- **The Engine is Go (`cmd/vans-engine`), the Shell is Tauri v2 (Rust + TypeScript).**
- **The Thesis:** The model writes a program once; the runtime caches and replays it with zero model calls on warm runs.
- **Accuracy above all:** Deterministic floors the model cannot lower; validate before trusting.
- **Minimum permissions & no data leaks:** Send only roles/labels for accessibility; send only selectors/counts for web styling. Never leak private user text.
- **Never launch applications:** VANS acts on what is already open on screen.
- **The Ladder:** Scripting dictionary (`appact`) -> Open page (`webact`) -> Accessibility tree (`uiact`) -> Web version -> Driving loop (`drive`).

---

## 4. On-Screen Coach & Guidance

- **Reference standard:** [Clicky](https://www.heyclicky.com/) (fluid, unobtrusive, on-screen guidance that points at controls and stays with the person as they work).
- **Smooth motion:** Spotlight cutouts and tooltips must morph and glide smoothly between steps rather than snapping abruptly.
- **Non-blocking:** The coach overlay covers the display, never intercepts mouse events, and lets the user click the real controls underneath.
- **Reactive progress:** Poll the window state (`uiact.Changes`) to detect user actions automatically.
