# VANS V1 — Design Spec

**Date:** 2026-08-04
**Status:** Draft for review
**Reasoning trail:** [`docs/decisions.md`](../../decisions.md) — every decision here has a `D-nnn` entry there with alternatives and tradeoffs. This document states *what to build*; that one states *why*.

---

## 1. Goal

Build the first vertical slice of the VANS runtime: an AI execution layer that drives real macOS applications through structured interfaces, invoking an LLM only when it must.

**The V1 task.** From a natural-language instruction, navigate to a webpage, extract the content the user named, and write it into Apple Notes with user confirmation.

**The property that matters.** The first run on a given page shape calls the LLM to synthesize an extraction program. Every subsequent run on that shape replays the program with **zero synthesis calls** and produces the same result. That gap is the entire thesis — "runtime first, LLM second" — and V1 exists to make it measurable rather than assertable.

**Scoping that claim honestly.** Turning "grab the pricing tiers off example.com and save them to Notes" into `(url, target description)` is itself a reasoning step. V1 keeps it out of the measured pipeline by giving the GUI two fields — a URL and a plain-language description of what to extract — so no instruction parsing happens at all. The zero-call guarantee therefore covers the whole runtime pipeline in V1, because the only LLM boundary is synthesis.

A free-text instruction bar is a natural V2 addition. When it arrives it introduces a second LLM boundary, and the guarantee must be restated as *zero synthesis calls* rather than *zero calls*. Recorded here so that restatement is a deliberate change rather than a quiet erosion.

## 2. Scope

**In:**

- Tauri GUI shell (window, task input, progress, confirmation dialogs, permission onboarding)
- Go runtime as a Tauri sidecar
- AppleEvents adapter driven by generated `.sdef` tool surfaces
- Chromium/CDP adapter (navigate, snapshot DOM, evaluate)
- Selector-program synthesis, caching, and deterministic replay
- Layered verification and a bounded failure ladder
- Read/write classification with a confirmation gate on writes
- SQLite persistence for tool surfaces and learned programs

**Out:**

- Accessibility API, vision, OCR, synthetic mouse/keyboard (D-010)
- Clicking, pagination, form filling (D-018 admits navigation only)
- MCP adapter (D-008)
- App Intents / Shortcuts tier (D-010)
- Personal memory, preference learning, privacy routing to local models
- Windows support (D-009 and D-010 are macOS-specific and do not port)
- Multi-goal plan trees — V1 tasks are a fixed three-stage pipeline, not a planner-generated tree

**Note on the planner.** V1 does not need a general planner, and does not build one. The task shape is fixed — navigate → extract → write — so there is nothing to plan. The LLM's only job is *synthesis*: writing the selector program.

This is a deliberate reduction from the architecture discussed during design, which included a `plan` component and the two-stage capability resolution of D-015. Both are unnecessary while the task shape is fixed and only one target app surface (Notes) is ever loaded. The D-002 goal-tree design remains the target architecture; V1 implements the *primitive* it depends on (synthesize once, cache, replay, verify) without the tree on top. A planner arrives when task shapes stop being fixed, and D-015's routing arrives when more than one candidate app surface exists.

## 3. Architecture

```
┌──────────────────────────────────────────┐
│  Tauri shell (Rust)                      │
│  task input · progress · confirmations   │
│  permission onboarding                   │
└───────────────┬──────────────────────────┘
                │ localhost WebSocket, per-launch token (D-014)
┌───────────────┴──────────────────────────┐
│  Go runtime                              │
│                                          │
│              engine                      │
│         (state machine)                  │
│                 │                        │
│   ┌──────┬──────┼───────┬────────┬─────┐ │
│  env  registry synth  security verify   │ │
│         │        │                store │ │
│    ┌────┴────┐  LLM                  │  │ │
│  sdef-gen  cdp-gen                 SQLite│ │
│    │          │                          │
│ adapter/   adapter/                      │
│ appleevents  cdp                         │
└──────────────────────────────────────────┘
```

`env`, `registry`, `verify`, `security`, the candidate finder, and the program runner are pure over injected data — no network, no apps, no API key required to test them (D-021).

## 4. Components

### 4.1 `capability/sdef` — AppleEvents tool-surface generator

Scans `/Applications` and `/System/Applications`. Reads `Info.plist` for `NSAppleScriptEnabled` and `OSAScriptingDefinition`. Parses `Contents/Resources/*.sdef` **XML directly** — the `sdef` CLI requires full Xcode and is unavailable (D-009).

Emits a `ToolSurface`. Classifies every tool `Read` or `Write` from the `access` attribute on properties and verb analysis on commands. **Unknown verbs default to `Write`** (D-012). Cached in SQLite by bundle ID + version.

### 4.2 `capability/cdp` — Chromium prober

Detects Electron bundles via `Contents/Frameworks/Electron Framework.framework`. Launches with `--remote-debugging-port`. Its surface is *generic* (navigate, snapshot, evaluate), not per-app typed — Chromium apps must have their competence learned rather than declared (D-011).

**Connections are revocable.** A verified failure mode: Discord accepted the debug flag, served CDP, then self-updated and relaunched itself without the flag seconds later. The adapter detects drop and re-establishes; it never assumes a session it opened stays open (D-011, D-020 rung 0).

Target enumeration uses `Target.getTargets` over the browser WebSocket. The `/json/list` HTTP endpoint was observed empty while `/json/version` succeeded and is not treated as authoritative.

### 4.3 `env` — Environment Manager

Running applications, frontmost application, window titles. Via System Events, which requires no special permission. **Never acts.**

### 4.4 `extract/candidates` — structure finder (pure)

DOM → `[]Candidate`. Finds repeated sibling structures: N nodes of matching tag/class shape under a common parent. Pure algorithm, no model involved. This is the deterministic work that shrinks a page to a handful of regions so the LLM sees candidates rather than 500KB of markup.

### 4.5 `synth` — program synthesizer (the only LLM boundary)

Input: user intent + candidate regions. Output: `ExtractionProgram` including its own `expect` block (D-005).

Provider-agnostic interface, one implementation. **Model selection is deferred to implementation time** (Q-5) — the interface is what V1 commits to.

### 4.6 `extract/run` — program runner (pure)

`ExtractionProgram` + DOM → `[]Row`. Fully deterministic, testable offline against DOM fixtures.

### 4.7 `verify` — verification (pure)

Three layers, in order (D-019):

1. **Runtime structural** — nonzero rows; consistent field arity across rows; no required column entirely empty; no degenerate all-rows-identical case.
2. **Model-declared** — the `expect` block.
3. **Read-back** — for AppleEvents writes, query the object back and compare against intent (D-013).

**Layer 2 may only tighten layer 1, never loosen it.**

### 4.8 `security` — classification and gating

Consumes the generator's Read/Write classification. Reads flow freely. Writes halt for GUI confirmation showing exactly what will be written. Appends to an audit log. No bypass path exists in V1.

### 4.9 `store` — SQLite

Two tables: tool surfaces (by bundle ID + version) and learned programs (by cache key). Plus the audit log.

### 4.10 `engine` — orchestration

The state machine, and the only component that coordinates others. Dependencies injected, so the engine itself is testable with fakes.

States: `Idle → Navigating → Extracting → Verifying → AwaitingConfirmation → Writing → Confirming → Completed | Failed`

## 5. Data model

```go
type ExtractionProgram struct {
    Root   string        // CSS selector for the repeating container
    Fields []Field
    Expect Expectation   // authored by the model (D-005)
}

type Field struct {
    Name     string
    Selector string       // relative to Root
    Attr     string       // "text" | "href" | "src" | any attribute
}

type Expectation struct {
    MinRows  int
    Required []string     // field names that must be non-empty
}

type CacheKey struct {
    Origin        string   // scheme + host
    PageSignature string   // D-006: hash of DOM skeleton, text stripped
    IntentHash    string
}

type ToolSurface struct {
    AppID   string        // bundle identifier
    Version string
    Tier    Tier          // AppleEvents | CDP
    Tools   []Tool
}

type Tool struct {
    Name   string
    Class  string         // owning sdef class, if any
    Access AccessKind     // Read | Write  (unknown ⇒ Write, D-012)
    Params []Param
}
```

**Page signature (D-006).** Hash of the DOM skeleton: tag names plus normalized class-name shape, depth-limited, all text stripped. Same site with different content yields the same signature — that is what makes the cache useful. A redesign yields a different signature — that is what keeps it correct. Depth limit and class normalization need empirical tuning (Q-7).

## 6. Execution flow

```
user instruction (GUI)
   │
   ├─ env: is a Chromium target available?
   ├─ adapter/cdp: Page.navigate(url)
   ├─ verify: URL matches, load complete            ← D-004 category 2
   ├─ adapter/cdp: snapshot DOM
   │
   ├─ pageSignature(DOM) → store.GetProgram(key)
   │      hit  ──────────────────────► program
   │      miss ─► candidates(DOM) ─► synth(LLM) ─► program
   │
   ├─ run(program, DOM) → rows
   ├─ verify(rows)  ── fail ─► failure ladder (§7)
   │      pass ─► store.PutProgram(key, program)
   │
   ├─ registry: Notes surface → `make new note`
   ├─ security: classified Write ─► GUI confirmation showing rows
   │      declined ─► Failed (clean, no side effect)
   │
   ├─ adapter/appleevents: make new note
   └─ verify: read note back by id                  ← D-013
          │
       Completed
```

Cache miss and cache invalidation converge on the same synthesis path — one implementation, exercised from both directions.

## 7. Failure ladder (D-020)

| Rung | Trigger | Action | LLM? |
|---|---|---|---|
| 0 | CDP disconnect, app relaunched, `osascript` timeout | Reconnect, replay same program. Max 2. | No |
| 1 | Verification failed, program from **cache** | Invalidate, fall through to synthesis. Once. | Once |
| 2 | Verification failed, program from **fresh synthesis** | Re-synthesize with failure reasons. Max 1. | Once |
| 3 | Anything else | Surface to GUI: what was tried, candidates found, failing predicate. | No |

Retry counters are **per stage, not global**.

**Hard rule:** any write that has already executed is never retried. It goes directly to rung 3. Verification ambiguity is not grounds for writing twice.

## 8. Testing (D-021)

**Fixtures, all checked in:**

- `testdata/sdef/` — real dictionaries copied from bundles (Mail, Notes, Chrome). Generator tests run with no apps installed.
- `testdata/dom/` — captured DOM snapshots. Candidate finder, runner, and verifier run fully offline.
- `testdata/mutations/` — captured DOMs mutated programmatically: renamed classes, inserted wrapper divs, deleted fields. Asserts the signature shifts correctly and verification fails. Tests cache invalidation without waiting for a site to redesign.

**Tiers:** pure (no network/apps/key — the bulk, CI); adapter integration (tagged, local only); synthesis (recorded cassettes in CI, live on demand).

**The thesis test:** assert the LLM call counter is exactly zero on a warm run.

## 9. Success criteria

V1 is done when all of the following hold:

1. Cold run succeeds on three structurally different websites.
2. Warm run on the same page shape: **LLM call count is zero**, asserted in the test suite.
3. Mutation fixture: signature shifts, verification fails, resynthesis recovers, result is correct.
4. A write cannot execute without explicit confirmation — verified by test, not inspection.
5. A CDP disconnect mid-task is recovered by rung 0 without resynthesis.
6. The pure test tier runs green with no apps installed, no network, and no API key.
7. `make new note` result is verified by read-back, not assumed.

## 10. Open questions

Tracked in `docs/decisions.md`. Blocking none of the above:

| # | Question |
|---|---|
| Q-5 | LLM provider and model selection |
| Q-6 | Windows strategy — nothing in D-009/D-010 ports |
| Q-7 | Page signature tuning: depth limit, class normalization |
| Q-8 | Re-verify D-008's MCP landscape against live sources |
| Q-9 | Coverage for native apps with no dictionary (Telegram, DBeaver) |
| Q-11 | Structural-check thresholds for legitimately optional fields |

Deferred to task 2 (D-022, natural-language CSS injection): Q-10 — whether its verification bar is "applied, not evaluated," or whether it pulls vision forward.
