# VANS — Progress Log

A durable record of implementation progress, completed milestones, and immediate roadmap status.

---

## Current Status Overview

* **Current Phase:** Phase 3 (Acting on the machine & On-Screen Coach) — **Infrastructure Merged, Live Verification Pending**
* **Active Branch:** `main`
* **Next Phase / Milestones:** Live Click Loop Proof, Live Multi-Step Coaching, Phase 4 (Vision Tier)

---

## Milestone History

| Step / Phase | Scope | Status | Notes |
|---|---|---|---|
| **Steps 1–2** | `.sdef` capability parsing, AppleEvents adapter | Merged | Native macOS scripting dictionary extraction |
| **Step 3** | CDP adapter (Chromium) | Merged | Direct Chrome devtools protocol DOM inspection |
| **Step 4** | Candidate finder (MDR) | Merged | Repeating table & card structure detection |
| **Step 5** | Page signature | Merged | Canonical structural DOM hashing for zero-call cache |
| **Step 6** | Synthesizer (LLM Boundary) | Merged | Free open-weight & Anthropic selector program synthesis |
| **Step 7** | Runner + layered verifier | Merged | In-process selector evaluation and output verification |
| **Step 8** | Program store | Merged | SQLite persistence for learned selector programs |
| **Step 9** | Engine + failure ladder + CLI harness | Merged | Cold run ~9.5s (1 model call) → Warm run ~344ms (0 model calls) |
| **Step 9c** | Generalized nodes | Merged | Extraction across multi-column sibling nodes |
| **Step 10** | Desktop shell (Tauri) | Merged | Private stdio RPC pipe, preflight checks, confirmation gate |
| **Phase 1** | Hotkey summoned overlay | Complete | `⌥Space` global hotkey, non-activating panel over full-screen apps, native `NSVisualEffectView` frosting, light/dark switch, window sized to its content |
| **Router** | Free-text instructions (D-039) | Complete, wired | Shape-keyed routing cache; cold 1 call, warm 0. `Engine.Ask` routes it; the panel is one field |
| **Sinks** | Destinations for an answer | Complete | panel / clipboard / notes. The gate follows the destination rather than being a fixed step |
| **Phase 2** | Screen context | Complete | Active app & window title metadata, Screen Recording preflight, silent display capture |
| **Phase 3 (Input & AX)** | AX tree walker, CoreGraphics events, read-back | Merged (#40–#46) | Fast C-API tree walking (~130ms), visible-box clipping, `CGEvent` posting, `Changes(before, after)` |
| **Phase 3 (Styling)** | Live page restyling | Merged (#47) | Structural CSS injection with dark-mode background sensing and zero data leakage |
| **Phase 3 (Coach)** | On-screen coach overlay, multi-step guide & non-activating panel | Merged (#48–#61) | `app.guide` route, overlay HUD, 500ms polling, advance channel, non-activating panel, context forwarding |

---

## Phase 2 Implementation Details (`phase-2-screen-context`)

1. **Go Runtime & RPC Protocol:**
   * Created `internal/screen/context.go` defining structured `Context` and `Rect` models (`app_name`, `bundle_id`, `window_title`, `pid`, `bounds`).
   * Added `screen_context` method to `internal/rpc/protocol.go` and `internal/rpc/server.go`.
   * Unit tests added in `internal/screen/context_test.go` and `internal/rpc/server_test.go`.

2. **Native Rust Shell (`ui/src-tauri/src/screen.rs`):**
   * Active application detection using macOS `NSWorkspace` before the overlay grabs focus.
   * PID and bundle identifier from `NSWorkspace` — no permission, sub-millisecond.
   * Window title via `CGWindowListCopyWindowInfo`, degrading to empty when Screen Recording is not granted (D-038).
   * Non-intrusive `CGPreflightScreenCaptureAccess()` and `CGRequestScreenCaptureAccess()` checks (D-037).
   * Silent low-latency display capture via `/usr/sbin/screencapture -x`.

3. **Frontend UI (`ui/src/`):**
   * Live `🎯 Active: <App> — <Title>` context banner on the task screen.
   * Preflight Screen Recording check button with one-click permission request.
   * `overlay://shown` event wiring to auto-focus the prompt input and update context on summon.
   * `Escape` key listener to dismiss the overlay panel.

---

## Phase 3 Implementation Details (Acting on the machine & On-Screen Coach)

1. **Accessibility Tree Walking & Target Resolution (`internal/uiact`):**
   * Migrated window capture from slow AppleEvents (23.9s) to direct C-API accessibility walker via cgo (~130ms, D-057).
   * Clipped control bounding boxes to ancestor frames and screen bounds so long text controls have click points on screen (D-062).
   * Window occlusion verification via `CGWindowListCopyWindowInfo` (D-062, D-064).

2. **Direct Synthetic Input Posting (`internal/uiact/input_darwin.go`):**
   * Moved input synthesis to `CGEventPost` to avoid granting broad permissions to `osascript` (D-065, #44).
   * Built read-back verification comparing pre- and post-action AX snapshots via `Changes(before, after)` (D-067, #46).

3. **Live Web Page Restyling (`internal/webstyle`):**
   * Implemented soft background tinting for web pages based on ambient background luminance measurement (D-068, #47).
   * Restyle clear operations recognized deterministically in Go at zero LLM cost.

4. **On-Screen Coach Flow (`internal/engine/guide.go`, `ui/src/coach.ts`, `ui/src/main.ts`):**
   * Transparent, click-through full-display overlay window at window level 26 (#48).
   * `app.guide` route kind, separated from `web.style` with cache fingerprinting (D-070, #52).
   * Question framing prompt turning "where is X" into a next-step target (D-071, #53).
   * 500ms polling loop detecting user AX state changes, with a 45s timeout (D-069, #50).
   * 1-line HUD panel shrink during active guidance (#54).

---

## Next Steps

1. **Prove the full click loop live on an actual application** (capture -> visibleBox -> verify occlusion -> CGEvent click -> read back).
2. **Exercise multi-step coaching live** on a real workflow to verify human-in-the-loop state transitions.
3. **Exercise the App Agent Drive loop** to execute multi-step tasks autonomously.
