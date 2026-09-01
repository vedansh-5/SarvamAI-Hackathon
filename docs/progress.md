# VANS — Progress Log

A durable record of implementation progress, completed milestones, and immediate roadmap status.

---

## Current Status Overview

* **Current Phase:** Phase 2 (Screen Context) — **Complete**
* **Active Branch:** `phase-2-screen-context`
* **Next Phase:** Phase 3 (Acting on the machine — Accessibility & Synthetic Input)

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
| **Phase 1** | Hotkey summoned overlay | Complete | `⌥Space` global hotkey, accessory policy, dismiss on blur, translucent panel surface, window sized to its content |
| **Phase 2** | Screen context | Complete | Active app & window title metadata, Screen Recording preflight, silent display capture |

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

## Next Steps: Phase 3 (Acting on the machine)

1. Read the macOS Accessibility tree (`AXUIElement`) to find real, addressable UI elements.
2. Post synthetic input (`CGEvent`) for clicks, typing, and scrolling.
3. Establish bidirectional RPC between Go runtime and Rust shell.
