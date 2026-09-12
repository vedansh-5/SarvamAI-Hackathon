# VANS — Design Decision Log

**VANS — Vedansh's Agent Navigation System.** A runtime that sits between the user and the operating system, separating reasoning from execution: a language model writes a program once, and the runtime owns, verifies, caches, and replays it with no model involved.

A running record of design decisions, the reasoning behind them, and what each one costs.

**Purpose:** decisions made in conversation evaporate. This file is the durable record. Every entry states what was decided, what alternatives were rejected, *why*, and what we gave up. When a decision is later reversed, the entry stays and gets a `Superseded by` line — the reasoning trail matters more than the current answer.

**Status vocabulary:** `Accepted` · `Proposed` (leaning, not committed) · `Open` (unresolved, blocking or not) · `Superseded`

---

## Environment facts

Measured on the development machine, 2026-08-04. These are load-bearing — several decisions below rest on them, and they are expensive to re-derive.

- macOS 26.5.1 (build 25F80)
- `osascript`, `sdef`, `shortcuts`, `plutil` all present
- **`sdef` binary is unusable** — requires full Xcode; machine has Command Line Tools only
- Accessibility permission **not granted** (`System Events → UI elements enabled` = `false`)
- `shortcuts list` returns empty (no user shortcuts defined)
- Listing running processes via System Events works with no special permission

Scripting dictionary survey (parsed directly from `Contents/Resources/*.sdef`):

| App | Classes | Commands | Properties |
|---|---|---|---|
| Mail | 27 | 19 | 191 |
| Music | — | 31 | — |
| Chrome | 5 | 23 | 31 |
| Photos | — | 18 | — |
| Safari | — | 10 | — |
| Calendar | — | 8 | — |
| Notes | 4 | 2 | 27 |
| Reminders | — | 1 | — |
| **Notion, VS Code, Telegram, Discord** | **none** | **none** | **none** |

Runtime classification of third-party apps (by bundle inspection):

- **Electron:** Discord, Notion, VS Code, Postman, MongoDB Compass, Claude, Antigravity IDE
- **Native, no scripting dictionary:** Telegram (Swift), DBeaver (Java)

The native-without-dictionary apps are reachable by *neither* V1 tier. This split drives D-009 through D-011.

---

## D-001 — V1 is one vertical slice, not the full architecture

**Status:** Accepted

**Context.** The founding vision document describes ~15 components (Intent Manager, Skill Engine, Planner, Execution Engine, Environment Manager, Capability Manager, three adapters, two memory tiers, Security Manager, Overlay Engine, Event Bus, State Machine). Each is a real project. The repository was empty.

**Decision.** Build one vertical slice end to end with the remaining components stubbed or absent, rather than scaffolding all of them shallowly.

**Reasoning.** A shallow scaffold of 15 components proves nothing and locks in boundaries chosen before any contact with reality. A single slice that runs end to end exercises the real interfaces and exposes wrong boundaries while they are still cheap to move.

**Tradeoff.** The architecture diagram will look unfinished for a long time. Components deferred now may need retrofitting, and retrofitting is more expensive than designing in — accepted deliberately, because designing in the wrong thing is more expensive still.

---

## D-002 — Plans are trees with lazily expanded, cached subtrees

**Status:** Accepted

**Context.** The vision document asserts two principles that cannot both hold for open-ended tasks:

- *"Planning should happen once"* and *"execution should avoid repeated LLM calls"*
- *"The planner never decides implementation details"*

Traced against "book a flight," this breaks. If plan steps are coarse (`search flights DEL→BLR`), something must reason about an unfamiliar UI at execution time — an LLM in the execution loop. If steps are fine (`type DEL into the origin field`), the planner had to know the page structure before the page was ever loaded — the planner is doing implementation, and it cannot plan what it has not seen.

**Decision.** The plan is a tree, not a list. The planner emits coarse goals. Each goal is expanded into concrete actions lazily on first encounter, by an LLM. **The expansion is the cached unit**, keyed by `(goal, app, page signature)`.

Cold: LLM expands the goal into concrete actions. Warm: cache hit, deterministic replay, zero LLM calls. Verification failure or cache miss re-expands *that subtree only*.

**Reasoning.** Preserves "plan once" at the goal level and "no LLM in execution" in steady state, without either being false on day one. It also supplies the concrete definition of a learned skill that the Skill Engine section left abstract: **a skill is a validated expansion — code the LLM wrote that the runtime owns and re-validates.** Not a recorded click sequence, not a cached plan.

**Tradeoff.** First run on any new app or page shape is slow and costs an LLM call. Cache invalidation becomes a core correctness concern rather than an optimization (see D-006). Debugging is harder: failures can originate in synthesis or in replay, and telling them apart requires the failure path to record which.

---

## D-003 — Hub-and-services, not a layered pipeline

**Status:** Accepted

**Context.** The vision document's architecture diagram is a vertical stack with Intent Manager near the top and Long-term Memory near the bottom. Tracing "book a flight to Bangalore next Friday" showed the Intent Manager cannot produce an executable task without reading personal memory first — origin city, preferred airline, cabin class, what "morning" means. The dependency runs bottom-to-top, against the diagram.

**Decision.** Model the runtime as an Execution Engine hub with Memory, Environment, Capability, and Security as services queried by anything that needs them.

**Reasoning.** The layered picture pushes toward wrong module boundaries — it implies each layer only talks downward, which is false for memory and environment state.

**Tradeoff.** Loses the clean "each layer only knows the one below" property, which is genuinely useful for reasoning about a system. Mitigated by keeping services as narrow interfaces with no back-references into the engine.

---

## D-004 — Verification splits three ways; only two support autonomy

**Status:** Accepted

**Context.** *"Every action must define an expected outcome"* is easy to write and hard to implement. Tracing three tasks showed the difficulty is not uniform — it depends entirely on the *kind* of outcome.

**Decision.** Classify every action by outcome type, and permit autonomy only for the first two:

| Outcome type | Example | Runtime-verifiable? |
|---|---|---|
| Data retrieval | extract table, read balance | **Yes** — compare the data itself |
| State change | navigate, filter, open file | **Yes** — observe resulting state |
| World side effect | send, buy, submit, delete | **No** — receipt only; unrecoverable if wrong |

**Reasoning.** The stated principle is true for two categories and quietly false for the third — which is exactly the category where being wrong is expensive. Making this explicit prevents the runtime from claiming a verification guarantee it does not have.

**Tradeoff.** Category three always requires a human gate, which caps how autonomous VANS can be on its most valuable tasks. Partially mitigated for native apps by D-012.

**Consequence.** Irreversibility must be a property of a step, not a runtime judgment call. A step marked irreversible halts for confirmation *regardless* of how confident a cached skill is.

---

## D-005 — The LLM authors the verification predicate alongside the program

**Status:** Accepted

**Context.** Who writes the expected outcome? If the planner does, it is reasoning about implementation, violating its stated boundary. If the capability layer does, it knows local mechanics ("the click landed") but not intent ("we are on the right flight's seat map").

**Decision.** The same synthesis step that produces an executable program emits its success condition as part of the artifact. The runtime enforces the contract; it does not author it.

```json
{
  "root": "div.results > article",
  "fields": [
    {"name": "title", "selector": "h3 a", "attr": "text"},
    {"name": "url",   "selector": "h3 a", "attr": "href"}
  ],
  "expect": { "min_rows": 1, "required": ["title", "url"] }
}
```

**Reasoning.** Neither the planner nor the capability layer has both intent and mechanics in view. The synthesizer has both, exactly once, at the moment it writes the program.

**Tradeoff.** Verification quality is only as good as the model's self-assessment, and a model that writes a bad extractor may write a matching bad predicate that passes. Mitigation: the runtime layers its own structural checks (consistent field arity across rows, no required column entirely empty) that the model cannot weaken.

---

## D-006 — Cache key is a structural page signature

**Status:** Accepted

**Context.** D-002 makes cache correctness load-bearing. The key choice determines whether the cache is useful or actively harmful.

**Decision.** Hash the DOM *skeleton* — tag names plus class-name shape, depth-limited, all text stripped.

**Reasoning.** Same site with different content (a new search query) yields an identical skeleton → cache hit, which is the entire point. A site redesign yields a different skeleton → miss → resynthesize. Keying on URL misses constantly and the cache is useless; keying on origin replays stale selectors into garbage.

**Tradeoff.** Signature tuning is delicate — too sensitive and every minor DOM change invalidates; too loose and stale programs run against changed pages. Expected to be the first real source of bugs. Depth limit and class-name normalization both need empirical tuning against real sites.

---

## D-007 — Tauri GUI is in V1

**Status:** Accepted — user decision

**Context.** Initial recommendation was to defer Tauri and start with a CLI, on the grounds that a GUI proves no part of the core thesis and introduces a Rust↔Go boundary on day one. User overrode: the GUI is the shape the product is meant to have, and building toward it from the start is the point.

**Decision.** Tauri shell in V1. Rust owns window, tray, overlay, permission onboarding, and task UI. Go owns the runtime.

**Reasoning.** Beyond the user's product judgment, the permissions story independently supports this. The runtime needs per-app-pair Automation (TCC) grants, and any later Accessibility or Screen Recording work needs manual grants in System Settings. Walking a user through that is a UI problem, not a CLI problem. The original recommendation to defer is withdrawn.

**Tradeoff.** Two languages and an IPC boundary from day one. Roughly 3–4× the slice size of the CLI-only version. Mitigated by keeping the runtime a library with the IPC server as one caller, so headless testing stays possible.

---

## D-008 — MCP is an opportunistic tier, not a foundation

**Status:** Accepted

**Context.** The vision document ranks MCP second in the interaction priority ladder, above Accessibility and DOM. Question raised: what do apps actually expose over MCP?

**Decision.** Keep MCP in the ladder — when an app ships a local server it is the best available tier — but build nothing on it. Not implemented in V1.

**Reasoning.** MCP today is an *API-wrapper* ecosystem, not an *app-exposure* layer. Servers named for consumer apps almost always wrap the vendor's **cloud REST API**: they need an API key or OAuth, they talk to vendor servers, and they never touch the copy of the app running on the user's machine. For a runtime whose premise is understanding the state of *this* computer, that is the wrong axis. Genuine local-app MCP servers are essentially all developer tools (Figma Dev Mode, some IDE plugins, Chrome DevTools MCP). Nothing in Mail, Notes, Notion-the-app, Telegram, or Discord.

**Tradeoff.** If first-party local MCP adoption accelerates, this tier becomes valuable and we will have built a parallel mechanism (D-009). Judged acceptable because D-009 is useful regardless.

**Confidence note.** Landscape assessment current as of the assistant's May 2026 knowledge cutoff and not independently verified against live sources. Worth re-checking before committing further; this space moves fast.

---

## D-009 — Generate tool surfaces from `.sdef` rather than waiting for MCP

**Status:** Accepted

**Context.** Following D-008: if apps do not expose structured tool surfaces, where does the "structured interfaces before perception" principle get its structure?

**Decision.** VANS generates the tool surface itself. Parse each app's `.sdef` scripting definition into a typed tool schema and hand it to the planner.

**Reasoning.** macOS already ships a machine-readable, first-party, per-app capability description. `.sdef` is XML declaring classes, properties, elements, and commands — structurally the same thing as an MCP tool schema. Parsing it yields, for Mail alone, 19 typed commands over 27 object classes with 191 addressable properties. No vendor cooperation, no API keys, no network, operating on the local app with the user's existing logged-in state.

**Tradeoff.** macOS-only; Windows needs an entirely separate mechanism (UI Automation, COM, PowerShell) with no shared implementation. Dictionary quality varies widely — Mail is rich, Reminders exposes one command. And a dictionary describes *capability*, not *reliability*: scriptable does not mean bug-free.

**Implementation note.** The `sdef` CLI requires full Xcode and is unavailable on this machine. Parse `Contents/Resources/*.sdef` XML directly from the app bundle. Verified working.

---

## D-010 — V1 adapters: AppleEvents (sdef) + CDP. No Accessibility, no vision, no synthetic input.

**Status:** Accepted

**Decision.** Two adapters, both fully structured. Every V1 action is typed and verifiable. No `AXUIElement`, no screenshots, no OCR, no `CGEvent` synthetic mouse or keyboard.

Evidence-based macOS ladder:

| Tier | Mechanism | Coverage | In V1 |
|---|---|---|---|
| 0 | MCP | Rare — dev tools only | No |
| 1 | AppleEvents via `.sdef` | Apple apps, Chrome, Safari, Finder | **Yes** |
| 2 | App Intents / Shortcuts | Growing; not enumerable from CLI | No |
| 3 | CDP | Chrome + every Electron app | **Yes** |
| 4 | Accessibility (`AXUIElement`) | Universal fallback | No |
| 5 | Vision + `CGEvent` | Last resort | No |

**Reasoning.** Tiers 1 and 3 together already cover Mail, Notes, Calendar, Music, Photos, Finder, Chrome, Safari, Notion, VS Code, Discord, and Telegram on this machine. Accessibility is where per-app brittleness lives, needs a permission grant not currently held, and drags in coordinate-level interaction. Deferring it keeps V1 entirely deterministic and keeps the thesis honest — if the structured tiers cannot carry real work, that is a finding worth having early rather than papering over with vision.

**Tradeoff.** Apps with neither interface are unreachable: Preview, System Settings, and most native non-scriptable apps. Tier 2 is skipped despite being first-party and growing, because available actions cannot be enumerated from the CLI — discovery is the blocker, not execution.

**Note on tier 2.** App Intents is likely the *right* long-term tier and should be revisited once a discovery mechanism exists.

---

## D-011 — The "Browser Adapter" is a Chromium Adapter

**Status:** Accepted

**Context.** Most zero-dictionary apps on this machine — Notion, VS Code, Discord, Claude, Postman, MongoDB Compass, Antigravity — are Electron. Electron is Chromium. Chromium speaks CDP.

*Correction, 2026-08-04:* an earlier version of this entry also listed Telegram and DBeaver as Electron. Bundle inspection disproved it — Telegram is native Swift, DBeaver is Java. Neither has a scripting dictionary, so both are unreachable in V1.

**Decision.** Name and scope the adapter as a Chromium adapter, not a browser adapter. One implementation covers Chrome plus the majority of installed third-party applications.

**Reasoning.** This is what makes multi-app V1 tractable. The apparent conflict between "support many apps in V1" and "start with the browser" dissolves — they are the same build.

**Tradeoff.** Electron CDP requires launching the app with `--remote-debugging-port`, so the runtime must relaunch apps. That is user-visible friction and it loses unsaved in-app state.

**Verified, 2026-08-04 (resolves Q-4).** Discord launched with `--remote-debugging-port=9333` served CDP correctly — `Chrome/148.0.7778.280`, `Electron/42.7.1`, protocol 1.3, browser WebSocket endpoint live. The mechanism works.

Two caveats found in the same test:

1. **The flag is revocable by the app.** Within seconds, Discord self-updated 0.0.403 → 0.0.406 and relaunched *itself* without the injected argument. The debug port vanished while the app kept running. **The adapter must treat CDP connections as revocable** — detect drop, and either re-establish by relaunching or degrade gracefully. It cannot assume a session it opened stays open.
2. **`/json/list` returned empty while `/json/version` succeeded.** Cause ambiguous — possibly the self-update relaunch, possibly an Electron quirk where HTTP target enumeration is unpopulated and `Target.getTargets` over the browser WebSocket is required instead. Needs retest; assume the WebSocket path is authoritative.

Only Discord was tested. Per-app viability across the other six Electron apps remains unverified.

**Corollary.** Electron AX trees are typically poor, since they are derived from DOM. CDP beats Accessibility for these apps on quality as well as availability — reinforcing D-010.

---

## D-012 — Security classification is derived from the `.sdef`, not hand-maintained

**Status:** Accepted

**Context.** D-010 puts writes in scope for V1: sdef surfaces include `make new note`, `send`, `delete`. That is D-004's category three, the one requiring a human gate. A hand-maintained list of dangerous operations per app does not scale and rots.

**Decision.** Derive read/write classification from the schema. `.sdef` properties carry an `access` attribute; commands are classifiable by verb (`get`/`count`/`exists` read; `make`/`delete`/`send`/`move` write). Reads flow freely. Writes gate on GUI confirmation.

*Precision note, 2026-08-04:* per `/System/Library/DTDs/sdef.dtd`, `access` is `(r | w | rw)` — **three** values, not two. Write-only properties exist. An earlier version of this entry said "`r` versus read-write," which would have mapped `w` incorrectly. Treat `r` as read; `w` and `rw` both as write-capable.

**Reasoning.** The capability generator and the permission model come from the same first-party schema. Adding an app adds both its tools and its safety classification in one step, with no manual curation.

**Tradeoff.** Verb-based command classification is heuristic and will misclassify. Errors are asymmetric in severity: a write misread as a read is a silent unconfirmed side effect. Mitigation — default unknown verbs to *write*, and require an explicit allowlist entry to demote anything to read.

---

## D-013 — Writes are verifiable on the AppleEvents tier

**Status:** Accepted

**Context.** D-004 established that world side effects are not runtime-verifiable. That conclusion was drawn from web automation, where the only evidence is a hoped-for confirmation string.

**Decision.** Treat AppleEvents writes as verifiable via typed read-back, and require every write action on this tier to define one.

**Reasoning.** The typed object model gives genuine read-back: create a note, query it back by ID; send mail, read the sent mailbox. The tier that permits action is the same tier that confirms it. This is a property web automation does not have, and it means category-three actions are meaningfully safer inside sdef apps than in a browser.

**Tradeoff.** Read-back confirms the object exists; it does not confirm the action was *desired*. Confirmation gates from D-012 remain necessary. Does not extend to the CDP tier, which keeps the weaker guarantee.

---

## D-014 — Tauri ↔ Go over localhost WebSocket

**Status:** Accepted

**Decision.** Go runtime ships as a Tauri sidecar binary. IPC is a localhost WebSocket authenticated with a per-launch token, not stdio.

**Reasoning.** The Event Bus needs to *push*: progress, verification results, confirmation requests. Bidirectional streaming over stdio is workable but awkward, and the event stream is central rather than incidental.

**Tradeoff.** Opens a local port — hence the per-launch token and loopback binding. More moving parts than stdio.

---

## D-015 — Capability resolution is two-stage

**Status:** Proposed

**Context.** Mail alone is 191 properties across 27 classes. Twenty scriptable apps is thousands of tool entries — far past what fits in a planner prompt. This is the affordance-leak problem from the original vision document (a planner that does not know what is possible produces unexecutable plans) in concrete form.

**Decision (leaning).** Two stages: a cheap app-level routing pass (which apps does this intent touch?), then load only those apps' surfaces, summarized.

**Reasoning.** Most intents touch one or two apps. Routing is a far smaller problem than planning and can use a cheaper or local model.

**Open.** How surfaces get summarized without losing the details the planner needs is unresolved. A wrong routing decision is unrecoverable — the planner never sees the app it needed. Needs a fallback path.

---

## D-016 — AppleEvents execution via JXA through `osascript`

**Status:** Proposed

**Decision (leaning).** Generate JXA and execute via `osascript` rather than binding raw AppleEvents through cgo.

**Reasoning.** JXA takes and returns JSON, which is dramatically easier than parsing AppleScript record output. No cgo, no Objective-C bridge in the build.

**Tradeoff.** Process spawn per call, roughly 50–100ms. Acceptable for V1 given the alternative's complexity, but it violates the latency-as-first-class principle at scale. Migration path: a persistent JXA host process, or raw AppleEvents via cgo later.

**Open.** Unmeasured. Needs benchmarking before acceptance.

---

## D-017 — Canonical V1 task family and concrete demo task

**Status:** Accepted

**Family.** *Structured data out of a Chromium app → into a native Mac app, with a confirmation gate on the write.*

**Concrete V1 task (resolves Q-1).** From the user's instruction: navigate to a webpage, extract the content they asked for, write it into Apple Notes.

**Extraction means selector-program synthesis**, not a whole-page text dump. The user names what they want ("the pricing tiers", "the comparison table"); the LLM writes a selector program once (D-005); the runtime replays it on every later visit with zero LLM calls (D-002).

**Reasoning.** One task exercises the entire system: navigation, CDP adapter, AppleEvents adapter, both capability generators, program synthesis, page signature and caching, read/write classification, GUI confirmation, and read-back verification. Cold run hits the LLM; warm run hits neither generator nor planner — which makes "runtime first, LLM second" observable rather than asserted.

**Rejected: whole readable-page dump → note.** Faster to build and immediately useful, but it has no selector program, therefore no synthesis, no cache, and no verification of consequence. Every run costs the same, so nothing demonstrates the core thesis. A working feature that proves none of the architecture.

**Stretch target.** Same task with Notion as the CDP source instead of Chrome. Same adapter, different target. Deferred because Notion's DOM is virtualized and contenteditable-heavy — it puts the candidate finder and page signature at their worst case, risking weeks spent on Notion specifically rather than on the runtime.

---

## D-018 — Navigation is in V1 scope

**Status:** Accepted

**Context.** V1 was originally scoped with no clicking, navigation, or pagination. D-017's task begins with "go to a webpage," which requires `Page.navigate`.

**Decision.** Navigation is in. Clicking and pagination stay out.

**Reasoning.** Navigation is a D-004 category-2 action — a state change, verifiable by checking resulting URL and load state. It carries none of the ambiguity of clicking, which requires identifying a target element and has no clean success predicate.

**Tradeoff.** Recorded explicitly rather than allowed to slip in, because "navigate" is the thin end of the interaction wedge — pagination and clicking will look like small extensions of it, and they are not.

---

## D-019 — Verification is layered; the model can only tighten it

**Status:** Accepted

**Decision.** Three verifier kinds, applied in order:

1. **Runtime structural checks** (CDP path) — nonzero rows; field arity consistent across rows; no required column entirely empty; no degenerate case where every row is identical, which indicates a selector that matched one container rather than N siblings.
2. **Model-declared checks** — the `expect` block from D-005.
3. **Read-back checks** (AppleEvents path, D-013) — query the written object back and compare against intent.

**The rule:** runtime checks run first and the model cannot weaken them. `expect` may only make verification *stricter*.

**Reasoning.** Closes the hole flagged in D-005's tradeoff — a model that writes a bad extractor may write a matching bad predicate, but it still has to clear a structural floor it does not control.

**Tradeoff.** Structural checks are generic and will produce false failures on legitimately irregular data (rows with genuinely optional fields). Threshold tuning needed; expect this to be annoying before it is right.

---

## D-020 — Failure ladder, and executed writes are never retried

**Status:** Accepted

**Decision.** Ordered rungs, each more expensive than the last:

| Rung | Trigger | Action | LLM? |
|---|---|---|---|
| 0 | CDP disconnect, app relaunched, `osascript` timeout | Reconnect, replay same program. Max 2. | No |
| 1 | Verification failed, program came from **cache** | Invalidate entry, fall through to synthesis. Once. | Once |
| 2 | Verification failed, program came from **fresh synthesis** | Re-synthesize with failure reasons in prompt. Max 1. | Once |
| 3 | Anything else | Surface to GUI: what was tried, candidates found, failing predicate. | No |

**Rung 0 exists because of the Discord finding in D-011** — an app can revoke the debug port mid-task, and that must not be misdiagnosed as a bad program.

Rungs 1 and 2 converge on one synthesis code path, exercised from both directions. Retry counters are **per goal node, not global**, so a five-goal tree cannot quietly burn ten LLM calls.

**The hard rule.** Anything classified as a write under D-012 that has *already executed* is never retried. It goes directly to rung 3. Verification ambiguity is not grounds for re-sending an email.

**Tradeoff.** Bounded retries mean some recoverable failures surface to the user that a more aggressive policy would have fixed. Accepted — unbounded LLM retry loops are worse in both cost and safety.

---

## D-021 — Test strategy: pure core, fixture-driven, with a zero-LLM assertion

**Status:** Accepted

**Decision.** Three checked-in fixture sets:

- `testdata/sdef/` — real dictionaries copied from app bundles. Generator tests run with **no apps installed**.
- `testdata/dom/` — captured DOM snapshots. Candidate finder, program runner, and verifier run offline.
- `testdata/mutations/` — captured DOMs mutated programmatically (renamed classes, inserted wrapper divs, deleted fields). Asserts page signature shifts correctly and verification fails.

Three test tiers: **pure** (no network, no apps, no API key — the bulk, runs in CI); **adapter integration** (tagged, needs real apps, local only); **synthesis** (recorded cassettes in CI so prompt changes stay regression-testable, live mode on demand).

**One test is the thesis:** assert the LLM call counter is exactly **zero** on a warm run.

**Reasoning.** The mutation fixtures test cache invalidation — which D-006 predicts is where the first real bugs live — without waiting for a real site to redesign. And "runtime first, LLM second" belongs in the test suite as an assertion, not in a README as a claim.

**Tradeoff.** Fixtures drift from reality; a captured DOM is a snapshot of a site that keeps changing. Needs periodic recapture, and passing tests will not prove live sites still work.

---

## D-022 — Task 2: natural-language CSS injection

**Status:** Proposed — explicitly not V1

**Decision (leaning).** Second task after D-017 ships: the user describes a visual change in natural language, and the runtime injects CSS into the page.

**Reasoning.** It fits the same primitive as extraction — the LLM authors an artifact, the runtime owns, caches, and validates it. Same cache key (page signature), same failure ladder. CSS *is* the program. Confirms the D-002 primitive generalizes beyond extraction, which is the main thing worth learning from a second task.

**Known problem to decide before starting.** Verification is inherently visual. The runtime can verify CSS was **applied** — query computed styles, confirm they changed as declared — but not that it **looks right**. Visual judgment is tier 5, the one tier D-010 excludes. Task 2 either accepts "applied, not evaluated" as its bar, or it pulls vision forward into the project. Decide deliberately, not mid-build.

**Also unresolved.** Injected CSS is ephemeral — lost on reload unless persisted. Persistence mechanism undecided.

---

## D-023 — Synthesis runs on Claude via structured outputs (resolves Q-5)

**Status:** Accepted

**Decision.** `Synthesizer` is a provider-agnostic interface with one implementation: the official `anthropic-sdk-go`, model `claude-opus-5`, adaptive thinking, and **structured outputs** (`output_config.format` with a JSON Schema) constraining the reply to the program shape.

**Reasoning.** Structured outputs make a well-formed program an API guarantee rather than a parse-and-retry loop, which removes an entire failure class from rung 2 of D-020. `Validate` still runs afterwards — the schema guarantees *shape*, not that the program is sensible, and a stored program can also go stale if the schema later changes.

**Tradeoff.** Structured outputs constrain the schema itself: numeric and string constraints (`minimum`, `minLength`) are unsupported, and every object must set `additionalProperties: false` plus `required`. Those constraints move into `Validate`, so the rules live in two places and a test asserts the schema's `attr` enum matches the validator's allowed set.

**Effort is left at the API default.** Lowering it trades synthesis quality for cold-run latency and cost, and has no effect on warm runs, which never reach this code. Configurable per-instance.

**Refusals are handled before reading content.** A declined request returns HTTP 200 with no usable content; reading content blocks first would produce nonsense.

---

## D-024 — The LLM call counter is a runtime component, not test scaffolding

**Status:** Accepted

**Decision.** `synth.Counter` wraps any `Synthesizer` and counts calls. The engine composes it in.

**Reasoning.** D-021 requires asserting the LLM call count is exactly zero on a warm run. That assertion needs a real, production-path counter — a test-only mock would prove the mock, not the runtime. Failed calls count too: they reached the model and cost money.

---

## D-025 — Default synthesis backend is a free hosted open-weight model

**Status:** Accepted — amends D-023

**Context.** D-023 pinned synthesis to Claude. Running the development loop against a paid frontier model on every cold run is not viable while iterating, and the requirement was explicitly *hosted* — not a local runtime.

**Decision.** A second `Synthesizer` implementation speaks the **OpenAI-compatible chat-completions shape**, selected by environment variables. The default is OpenRouter's free tier. The Anthropic implementation stays and is selected with `VANS_LLM_PROVIDER=anthropic`.

**Correction (2026-08-05).** The default model was `google/gemma-4-31b-it:free` and is now `google/gemma-4-26b-a4b-it:free`. Measured: 31b returned 429 on every real synthesis request while trivially small requests to the same account succeeded, so the limit is on upstream load, not on the account. The 26b-a4b variant is a mixture with 4B active — weaker, but reachable. This is availability, not quality; `VANS_LLM_MODEL` overrides it when 31b has capacity.

**Reasoning.** One implementation covers OpenRouter, Groq, Cerebras, Together, vLLM, LM Studio, and llama.cpp — every one of them exposes the same endpoint shape, so the choice of provider is a base URL, not a code change. This is the payoff of the provider-agnostic interface D-023 committed to; adding the backend touched no other package.

Model choice: verified against OpenRouter's live model list on 2026-08-05 — of 19 free models, 4 advertise structured-output support. Gemma is dense and instruction-tuned, which makes it more reliable at emitting a bare JSON object than the reasoning-tuned mixtures, several of which prepend a thinking trace.

**Tradeoff — and why the architecture absorbs it.** An open-weight model writes worse programs than a frontier model: weaker candidate choice, sloppier selectors, over-optimistic `expect` blocks. That degrades **cold-run latency, not correctness**, because a bad program fails verification (D-019) and the failure ladder re-synthesizes it (D-020). The cost is more rung-2 retries, not silent wrong data. This is the first real evidence that the verification design earns its keep.

Free tiers also rate-limit aggressively, so HTTP error bodies are surfaced verbatim rather than collapsed to a status code. A 429 or a 5xx is retried with exponential backoff (`Retry-After` honoured when present in delay-seconds form, clamped to 60s); a 4xx that is not 429 is never retried, since the request itself is wrong and retrying only burns quota. Rate limiting is a wait, not a failed task — treating it as one would make the free tier unusable, which is the point of this decision.

**Robustness the frontier path did not need.** Open-weight models wrap replies in markdown fences, prepend commentary, and emit `<think>` traces even when instructed otherwise. `extractJSONObject` scans for the first balanced JSON object, ignoring braces inside string literals, so a formatting quirk does not push a correct program into the failure ladder. Three `SchemaMode` levels degrade from API-enforced `json_schema` to `json_object` to prompt-only, for models that reject the stricter forms.

---

## D-026 — Programs execute in Go against a captured DOM, not in the page over CDP

**Status:** Accepted

**Context.** The runner could evaluate a program's selectors two ways: send `document.querySelectorAll` into the page via `Runtime.evaluate`, or parse the captured HTML in Go and match selectors there. The browser's own CSS engine is the more faithful of the two — it supports every selector a model could write, including whatever shipped last month.

**Decision.** Programs run **in Go**, against the same HTML string that produced the page signature and the candidate regions. CSS matching uses `github.com/andybalholm/cascadia`.

**Reasoning.** The three things the runtime does to a page — sign it, find candidates in it, extract from it — must all describe the *same* DOM. Querying the browser separately evaluates selectors against a snapshot taken later than the one that produced the cache key. A page that mutated in between (infinite scroll, a late-loading rail, a re-render) would pass verification while returning data from a shape the signature does not describe. That is the failure mode the cache exists to prevent, reintroduced through the back door.

Running in Go also keeps the runner pure: no browser needed to test it, and it works on HTML from any source, which matters once non-Chromium surfaces arrive.

**Tradeoff.** Cascadia implements CSS3 plus parts of CSS4, not everything Chrome accepts. A model emitting `:scope` or a very new pseudo-class writes a program the runner cannot compile. This is handled rather than avoided: a selector that fails to parse is a `ProgramError` carrying the parse error, which the ladder feeds back as a rung-2 re-synthesis reason. The model is told exactly what it wrote that the runtime cannot run, so the fidelity gap costs one extra synthesis on first encounter instead of producing wrong data.

---

## D-027 — The verifier's floor is what the model cannot weaken

**Status:** Accepted — implements D-019

**Context.** D-019 established that verification is layered and the model may only tighten it. The runner made that concrete, and the concrete version exposed a hole: the model authors `expect`, so it also chooses how weak its own contract is. A program declaring `min_rows: 1` with an empty `required` list passes every contract check while returning one row of empty strings.

**Decision.** Verification enforces two independent layers.

*The model's contract:* row count meets `min_rows`; every field named in `required` is non-empty in every row.

*The runtime's floor, which no `expect` block can lower:*
1. Zero rows never passes, whatever `min_rows` says.
2. Every declared field must be non-empty in **at least one** row.
3. A record yielding nothing in any field is dropped, and if more than `MaxDropRate` (34%) of matched records are dropped, the root selector is judged broader than the records.

**Reasoning for floor 2.** A field empty in every row is a broken selector. The user named that field in the request, so a column of blanks is a wrong answer being reported as a successful one — and it is the most common partial failure, where the model gets two selectors right and one wrong.

**Tradeoff.** A field that is legitimately absent across a whole page — a `discount` column no product currently carries — fails a run that arguably succeeded. Accepted, because the error is in the safe direction: it routes to re-synthesis rather than writing blanks into the user's notes, and the user asked for that field, so the answer is incomplete either way.

**Reasoning for the drop rate.** Real listings interleave placeholder cards, skeleton loaders, and ad slots that share the records' shape. Failing on the first one is too brittle and keeping them puts blank rows in the output, so they are dropped and counted. The count is the signal: a few is normal page furniture, a third or more means the root is matching a container class rather than the records — a failure that row count alone cannot catch, since enough good rows survive to satisfy `min_rows`.

**Every reason is reported, not the first.** Failure reasons are re-synthesis input (D-020, rung 2). A model told only about the first broken selector fixes it and leaves the second for the next round trip, and every round trip is a model call the runtime exists to avoid. Reasons also distinguish *selector matched nothing* from *selector matched an element that was empty* — different defects that send the model to different fixes.

---

## D-028 — Cache key is origin + page signature + normalized intent; paraphrases miss

**Status:** Accepted — implements the `CacheKey` in the V1 spec

**Context.** The store needs a key that hits on the ordinary case (the same search run again tomorrow) and misses on the dangerous one (a page that changed shape). All three parts of the spec's `CacheKey` had to be made concrete.

**Decision.**
- **Origin** is `scheme://host`. The path and query are deliberately excluded — a search results page differs from every other search results page only in its query string, and keying on the full URL would mean a cache that never hits. The signature is what distinguishes page *shapes*; the URL only says which site.
- **Signature** is D-006's structural hash, unchanged.
- **Intent** is normalized shallowly — case folded, punctuation dropped, whitespace collapsed — then hashed. Nothing tries to understand the sentence.

**Tradeoff — paraphrases miss.** "product names and prices" and "the name and price of each product" ask the same question and will each be synthesized. Accepted, because the alternatives are worse. Semantic matching needs an embedding model, and that is a network call on the warm path — a warm path that calls a model is the thing this whole design exists to avoid. The asymmetry decides it: a miss costs one synthesis and is then cached, while a false hit runs the wrong program and reports it as a success.

**Origin scoping is a safety property, not just a namespace.** Two unrelated sites can share a DOM skeleton. A program written for one has no business running on the other, and the signature alone would not stop it.

---

## D-029 — A key holds several programs, tried in order, rather than one that gets overwritten

**Status:** Accepted

**Context.** The obvious store is one program per key, replaced on re-synthesis. Building the failure ladder against it exposed a thrash: when two page variants share a signature, each visit fails the cached program, re-synthesizes, and overwrites — so the alternate visit fails again. The cache would call the model on *every* visit, which is worse than having no cache, and it fails precisely on the headline claim.

**Decision.** A key maps to up to `MaxProgramsPerKey` (4) programs. Lookup returns them ordered by fewest consecutive failures, then most recent success. The runner tries each in turn; the model is called only when every stored program has failed. Re-synthesis inserts; it does not replace. Re-inserting a byte-identical program returns the existing row rather than resetting its statistics — a model handed failure reasons sometimes returns exactly what it just wrote.

**Reasoning.** A program failing verification does not prove the program is wrong. It may be right for a variant the signature collapsed. Discarding it throws away the only evidence that distinguishes "this program is broken" from "this page is not the page it was".

`consecutive_failures` rather than a lifetime failure rate: a program that worked for a year and broke yesterday should not outrank one that has never worked.

**Tradeoff.** When a site simply churns forward — a new build hash every deploy — the older programs are dead weight, and each is a wasted local run before the lookup falls through. The waste is bounded (four programs, all pure in-Go DOM matching, no network) and is the price of not thrashing on A/B tests and staged rollouts, which are common. Measured in `TestAlternatingVariantsDoNotThrash`: with one program per key the same six alternating visits cost 8 model calls; with four, 2.

**Statistics are recorded after verification, not after extraction.** A program that produced rows the verifier rejected did not succeed, and recording it as one would promote it to the front of the next lookup.

---

## D-030 — Pure-Go SQLite; stored programs are re-validated on every load

**Status:** Accepted

**Decision.** `modernc.org/sqlite`, not `mattn/go-sqlite3`. WAL journal, `busy_timeout`, and a single connection.

**Reasoning.** modernc is cgo-free. V1 ships as a Tauri desktop app (D-007), and cgo makes cross-compilation and code signing materially harder for no benefit at this scale — a single-user program cache is not where SQLite's C performance matters. One connection because write throughput is not the constraint here and SQLite serializes writers anyway; an in-memory store opened on several connections would silently be several different databases.

**Programs are validated on load, and unusable rows are deleted rather than skipped.** A cached program outlives the schema that produced it. Replaying one the runner cannot execute would surface as a page failure rather than as what it is, and skipping without deleting means every future lookup pays for it again.

---

## D-031 — The confirmation gate is the shape of the API, not a flag on a call

**Status:** Accepted — implements D-004 category 3

**Context.** D-004 established that a world side effect cannot be verified away and needs a human gate. The obvious implementation is one `Run(task)` method with a `confirm func(rows) bool` callback or a `RequireConfirmation bool` field.

**Decision.** The engine exposes two methods. `Prepare` navigates, extracts, verifies, and caches — and touches nothing outside the browser. `Commit` performs the write. There is no third method, and no way to reach a side effect without holding a `Plan` that `Prepare` returned.

**Reasoning.** A caller cannot forget to check a boolean it was never given. With a flag, the safe path depends on every future caller — the Tauri shell, the CLI harness, a test, whatever comes after — remembering to set it. With two methods, forgetting the gate means forgetting to write at all, which fails loudly and harmlessly. The gate stops being a policy the code follows and becomes a fact about what the code can do.

It also matches the GUI shape D-007 committed to: the window shows what `Prepare` returned and the button calls `Commit`.

**A plan is spent on the first Commit, successful or not.** D-020's hard rule is that an executed write is never retried, and `committed` is set *before* the sink is called rather than after. A write that timed out may still have landed; treating an ambiguous failure as permission to write again is how a runtime creates two notes where the user expected one.

---

## D-032 — The engine never launches applications

**Status:** Accepted

**Context.** Stated directly during development, after a test launched Discord to check whether it served CDP: *"don't run up apps and systems — why do we need it anyways"*. The reasoning generalizes past the test.

**Decision.** The runtime attaches to what is already running. `ChromiumBrowser` connects to an existing debugging endpoint and, when there is none, returns an error naming the exact command for the user to run. Nothing in the engine spawns a process.

**Reasoning.** Launching an app is itself a world side effect — it changes what is on screen, what has focus, and what is in the user's dock — and it would happen during `Prepare`, which is the phase defined as touching nothing. A runtime that opens applications to satisfy its own plan is doing something the user did not ask for, *before* any gate has been reached. The failure mode is also worse than it looks: the app may already be running without the debugging flag, so "launch it" means "quit and relaunch the user's browser", losing their tabs.

**Tradeoff.** Setup is manual, and V1 must explain it. That is the correct place for the cost — a one-time instruction the user follows knowingly, rather than a surprise the runtime performs repeatedly.

---

## D-033 — Only http and https values become links in written output

**Status:** Accepted

**Context.** Extracted rows are rendered into a note, and a link field is far more useful clickable than as text.

**Decision.** A value is rendered as an anchor only when it parses as a URL with an `http` or `https` scheme and a non-empty host. Everything else is escaped and shown as plain text. All values and the title go through HTML escaping regardless.

**Reasoning.** Extracted values are attacker-controlled in the ordinary case — any page can put anything in an `href`. Rendering `javascript:` or `data:` as a clickable link inside Notes is the page reaching out of the browser and into an application that trusts its content. This is the first place in the pipeline where untrusted page data crosses into another app, so the filter belongs here rather than in the adapter.

Values are shown rather than dropped when they fail the check: a hidden field looks like a bug, and the user can see for themselves that the link was not one.

---

## Amendment to D-020 — rung 1, given D-029

Rung 1 was written as "verification failed on a program from cache → invalidate, fall through to synthesis." D-029 changed what a cache entry is: a key holds several programs, not one.

Rung 1 now reads: **every cached program is tried before synthesis is considered.** Each failure is recorded against that program, which demotes it in future lookups; nothing is deleted. Synthesis happens only when the whole cache for that key has been exhausted, and it is given the rejection reasons from every program tried, not just the last.

`InvalidateSignature` still exists for the blunt case — a signature shown to be wrong for one intent is not trustworthy for the others — but it is a deliberate operation, not the automatic response to a single failed run.

---

## Note on `cmd/vans` and the rejected CLI-first decision

`cmd/vans` is a development harness, not a reversal of D-007. The rejected proposal was to *ship* a CLI as V1's product surface and defer the GUI; the reasoning against it (permissions onboarding, and a confirmation gate that wants a window) is unchanged. What the harness provides is a way to run the real pipeline against a real browser and a real Notes.app before that window exists, so the engine is exercised by something other than its own tests. It has no `--yes` flag: a harness that can skip the gate is a harness that tests a different program than the one that ships.

---

## D-034 — First live run, and the limit it exposed

**Status:** Recorded — measurement, plus an open design gap

**Measured 2026-08-16**, Hacker News via Chrome 151 over CDP, synthesis on OpenRouter's free tier (`gemma-4-26b-a4b-it:free`):

| Run | Time | Model calls |
|---|---|---|
| Cold | 10.6 s | 1 |
| Warm | 363 ms | 0 |
| Warm | 329 ms | 0 |

32× faster and free after the first visit. This is the first end-to-end evidence for the central claim, on a real site rather than a fixture.

**The limit.** The first attempt asked for *title, link and score*. It failed, twice, and rung 3 reported exactly why: `field "score": selector "td.subtext > span.score" matched nothing in any of the 30 records`.

The cause is structural, not a model failure. Hacker News puts the score in the **next sibling row**, outside the record:

```html
<tr class="athing submission"><td class="title">…<a>Title</a></td></tr>
<tr><td class="subtext"><span class="score">318 points</span>…</td></tr>
```

The program primitive selects one element per record and reads every field relative to it (D-005). No selector can reach out of the root, so this data is unreachable — and the model demonstrated it understood *where* the score was, since its second attempt named `td.subtext > span.score` correctly. What it could not do was express "the sibling row", because the contract has no way to say it.

**This is a simplification we took from MDR without recording it.** Liu, Grossman & Zhai define a data record as a *generalized node*: a set of **consecutive sibling subtrees**, not a single one. `internal/extract` implements only the single-subtree case. Every table-based layout that splits a record across rows — and any list that puts metadata in a sibling element — is out of reach until that changes.

**What the failure surface got right.** No wrong data was produced and nothing was written. The runtime spent its budget, refused to guess, and reported the failing predicate, the two programs it tried, and the candidate regions it found. Rung 3 behaved as designed (D-020), and the two-phase gate meant the failure cost nothing outside the browser.

**Resolved by D-035** — the gap was closed rather than documented.

---

## D-035 — A record is a window of consecutive siblings, not a single element

**Status:** Accepted — closes Q-14, resolves the gap D-034 found

**Context.** D-034 recorded that a record split across sibling elements is unreachable, and traced it to a simplification we took from MDR without writing down: the paper defines a data record as a *generalized node*, a set of consecutive sibling subtrees, and `extract` implemented only the single-subtree case.

**Decision.** A record is a window of up to `MaxRootSpan` (6) consecutive sibling elements.

- `Program.RootSpan` — how many siblings form one record, counting the one `Root` matched.
- `Candidate.Span` — the span a region was found with, shown to the model along with the whole record's markup.
- The runner builds a window per root match and searches **every element of it** for each field.

**Zero is one.** `RootSpan` decodes as zero for every program cached before this existed, and `Span()` normalizes it to 1 — which is exactly what those programs describe. No migration, no invalidation, no re-synthesis.

**Two rules decide where a record begins.** Trying every rotation of a repeating period finds the right group but not the right boundary, and the boundary matters because it becomes the root selector.

1. **The first element's selector must match exactly as many elements as there are records.** A bare `tr` matches every row on the page, so replaying it would build overlapping windows. `tr.athing` matches exactly the 30 record starts. This is what makes the boundary reproducible rather than merely correct once.
2. **The first element must have text.** Hacker News separates records with an empty spacer row; the rotation starting on the spacer is rejected in favour of the one starting on the title.

**The runner enforces non-overlap at replay time too.** A root that matches more often than records repeat would otherwise emit windows sharing elements, reporting the same data several times. Consumed elements are tracked and a record starting inside a previous window is dropped.

**Verified live** on the case that motivated it. Hacker News, *title, link and score* — the request that failed twice under D-034:

```json
{"root":"tr.athing.submission","root_span":2,
 "fields":[{"name":"score","selector":".score","attr":"text"}, …],
 "expect":{"min_rows":1,"required":["title"]}}
```

30 rows, one model call, 9.5 s cold and 344 ms warm. The model chose span 2 rather than 3 — it needs the title row and the subtext row but not the spacer — and marked only `title` required, correctly recognizing that HN's job postings carry no score. The layered verifier (D-019, D-027) accepted a legitimately blank column because the model's contract said it could be blank, and rejected nothing that was real.

**Tradeoff.** Candidate discovery now runs `MaxSpan × span` passes over each parent's children instead of one. It is still linear in page size with a small constant, and the cost falls entirely on cold runs — warm runs never call it. The larger risk is spurious multi-element regions on pages with incidental periodicity; the two boundary rules are what hold that down, and `MaxRootSpan` bounds the damage when they do not.

---

## D-036 — The shell talks to the engine over a private pipe, never a port

**Status:** Accepted

**Context.** The desktop shell is Rust (Tauri); the runtime is Go. They have to talk. The usual answer, and what most Tauri projects do, is to run the backend as a localhost HTTP server and `fetch` from the frontend.

**Decision.** The Go runtime is a **sidecar child process** speaking newline-delimited JSON over stdin and stdout. It has no network listener. Linking Go into Rust as a C library was also rejected — it reintroduces cgo, which D-030 avoided precisely so the desktop build stays cross-compilable.

**Reasoning — this is a security decision, not an ergonomic one.** `Commit` performs irreversible side effects: it writes into the user's Notes. A localhost port is reachable by **every process on the machine** — any Electron app, any `npm install` script, anything that can open a socket. `POST /commit` from any of them would walk straight around the confirmation gate that D-004 and D-031 exist to enforce. Binding to a random port with a bearer token narrows it but does not close it; the token has to live somewhere readable, and the port is still open.

A pipe is reachable only by the process that created it. The gate holds because there is no second way in.

**Consequences that fell out of it.**

- *Plans are session-scoped.* A plan id is meaningful only to the process that issued it, so a confirmation cannot be replayed later or from elsewhere. Verified by test.
- *stdout is protocol-only.* Diagnostics go to stderr; a stray line on stdout desynchronizes the shell's decoder.
- *Progress needs a push channel.* A cold run takes ten seconds and a silent window looks broken, so the engine's log hook is forwarded as notifications tagged with the request they belong to.

**Tradeoff.** No browser devtools against the backend, and a crashed sidecar has to be respawned by the shell rather than reconnected to. Both are cheaper than an open local port on a component whose whole job is gating irreversible actions.

---

## D-037 — Preflight never touches Notes; the user asks for that check

**Status:** Accepted — reconciles D-007 with D-032

**Context.** D-007 chose a GUI partly *for* permissions onboarding: discovering a denied automation permission halfway through a task is a bad experience, and a window can ask for it up front. But the only way to test automation access is to send an Apple event, which **launches Notes** — and D-032 says the runtime does not launch applications.

**Decision.** The automatic preflight checks the synthesis backend and the browser endpoint only. Notes has its own method, `check_notes`, called only when the user presses a button that says what it will do.

**Reasoning.** The conflict is real but it is about *timing*, not about whether the check happens. What D-032 forbids is the runtime opening applications on its own initiative. A button the user presses is their initiative, not the runtime's — and pressing it at a moment they chose is exactly the onboarding benefit D-007 wanted, versus hitting the denial mid-task.

An app that launches Notes merely because its window opened is doing something nobody asked for, which is the thing being avoided.

---

## D-038 — Active-application lookup uses AppKit, and the window title degrades on its own

**Status:** Accepted — replaces the System Events implementation in Phase 2

**Context.** Phase 2 read the frontmost application by asking System Events over `osascript`. It worked, but it sat on the path that runs every single time the panel is summoned.

**Measured 2026-08-27** on the development machine: **355 ms per call**, and System Events needs an Accessibility grant that is still not given (see § Environment facts). A third of a second of latency on every hotkey press, gated behind a permission the user has not granted, for information macOS hands out for free.

**Decision.** Application identity comes from `NSWorkspace.frontmostApplication` via `objc2-app-kit` — name, bundle identifier, and pid, with **no permission required** and no measurable cost.

The window *title* is treated as a separate concern, because it genuinely is one: macOS gates it behind either Screen Recording (`CGWindowListCopyWindowInfo`) or Accessibility (the AX API). It is read via CGWindowList when Screen Recording is granted, and **degrades to an empty string otherwise** rather than failing the lookup.

**Reasoning.** These two facts have different privacy weights and macOS treats them differently, so collapsing them into one call meant the cheap, unprivileged half inherited the cost and the permission requirement of the expensive, privileged half. Splitting them means the runtime always knows *which application* you are in — the part that decides which adapter tier applies — and knows *what you are looking at* only when you have allowed that.

**Verified** by a test that calls the real AppKit path: `app="Google Chrome" bundle="com.google.Chrome" pid=28226 title="" screen_recording=false`. Identity present, title correctly absent.

**Consequence for Phase 3.** Accessibility is still needed for acting on other applications, but it is no longer needed merely to *observe* which application is in front. The permission is now required only where it is genuinely unavoidable.

---

## Open questions

| # | Question | Blocking? |
|---|---|---|
| ~~Q-1~~ | ~~Which concrete demo task instantiates D-017?~~ | **Resolved** — see D-017 |
| ~~Q-2~~ | ~~Verification predicates and the failure/retry ladder~~ | **Resolved** — see D-019, D-020 |
| ~~Q-3~~ | ~~Test strategy~~ | **Resolved** — see D-021 |
| Q-10 | Task 2 verification bar: "applied, not evaluated", or pull vision forward? | No — blocks task 2, not V1 |
| Q-11 | Structural-check thresholds (field arity tolerance for optional fields) | No — empirical. First values set in D-027 (`MaxDropRate` 34%, "non-empty in at least one row"); needs tuning against real pages. |
| ~~Q-4~~ | ~~Which Electron apps accept `--remote-debugging-port`?~~ | **Resolved** — verified on Discord; see D-011. Two caveats recorded. |
| Q-9 | How is Telegram/DBeaver-class coverage handled? (native, no dictionary, not Chromium) | No — out of V1 scope by D-010 |
| ~~Q-5~~ | ~~LLM provider and model selection~~ | **Resolved** — see D-023. Local-model routing for memory-touching paths stays open (no personal data in V1). |
| Q-6 | Windows strategy (UI Automation / COM); nothing in D-009 or D-010 ports | No |
| Q-7 | Page signature tuning: depth limit, class normalization | No — empirical |
| Q-12 | Paraphrased intents miss the cache (D-028). Worth a local embedding model once one is on the machine anyway, or a stored list of known phrasings per program? | No — costs synthesis calls, never correctness |
| ~~Q-14~~ | ~~Records spanning consecutive siblings are unreachable~~ | **Resolved** — implemented, see D-035 |
| Q-13 | Nothing evicts a program that stops being used. Bounded per key (D-029) but unbounded across keys. Needs an age or size policy before the store is long-lived. | No — not before V1 ships |
| Q-8 | Re-verify the D-008 MCP landscape against live sources | No |

---

## Rejected

Recorded so they are not relitigated.

- **CLI-first, defer Tauri** — overridden by D-007. Reasoning stands on its own merits but is outweighed by product shape and the permissions onboarding argument.
- **Table-only extraction (`querySelectorAll('table')`)** — proves nothing. No planner, no capability resolution, no cache, no meaningful verification. A browser utility, not a runtime.
- **Whole readable-page dump as the V1 task** — see D-017. Useful, but proves none of the architecture.
- **Accessibility API in V1** — see D-010. Deferred, not rejected permanently.
- **Layered pipeline architecture** — see D-003.
- **Building on MCP as the primary structured tier** — see D-008.

---

## D-039 — Instructions are routed on their shape, not their text

**Status:** Accepted — `internal/route`

**Context.** Until now the overlay was a form with three boxes — page, what to
extract, note title — because that is exactly the shape of `engine.Task`. There
was one verb. Anything the user might want that is not "read records off a web
page and write them to Notes" had no way of being asked for.

Making the overlay accept a free-text instruction means something has to decide
what an instruction is *asking for*, and that decision is a language problem.
It is the second place a model becomes necessary.

**Decision.** The router reuses the primitive the extractor already established
(D-002, D-006): the model decides once, the runtime caches and replays.

The cache key is the instruction's **shape** — the sentence with its values
lifted out and replaced by placeholders:

```
"the top 10 stories from https://news.ycombinator.com"
"the top 30 stories from https://lobste.rs"
            ↓ both shape to
"the top {n0} stories from {u0}"
```

Those are the same instruction asked about different particulars, so the second
costs no model call. This is the page signature idea moved up a level: there,
structure is cached so content can change; here, phrasing is cached so
particulars can change.

The model's answer is a `Decision` — a kind, plus **templates** for that kind's
slots (`url: "{u0}"`, `intent: "top {n0} stories"`). Templates rather than
finished strings is what makes the decision reusable: the structure is cached,
the values come from whichever instruction arrives.

**Why not cache on normalized text.** `store.NormalizeIntent` already lowercases
and strips punctuation, and it would have been the cheap thing to reuse. But it
keeps the literals, so every new URL is a new key and the router would call the
model on essentially every instruction — a cache that never hits.

**Why the router may answer "I don't know."** `KindUnknown` is a valid decision.
A router forced to pick the nearest kind sends the task somewhere it will fail,
far from the cause, with an error the user cannot act on. Declining is a better
answer than a wrong route, and it is what lets the overlay say which
capabilities exist.

**Three floors the model cannot lower**, mirroring the verifier's (D-016):

- A decision naming a kind the engine cannot run is rejected.
- A decision whose required slots do not *resolve* is rejected — not merely
  whose templates are non-empty. A decision can be structurally sound and still
  fill to nothing, because the template references `{u0}` and this instruction
  carries no URL. That is a miss, not a route.
- **A template that hardcodes this instruction's values is rejected.** Found
  against a live model, not in review. Asked about
  `"the top 10 stories from https://news.ycombinator.com"`, it templated the
  number and hardcoded the site:

  ```
  intent: "the top {n0} stories from the Hacker News homepage"
  ```

  Correct for the instruction in front of it, wrong for every later one of the
  same shape — and because the decision is cached, the wrongness is durable and
  silent. `lobste.rs` was asked for Hacker News stories, and nothing failed.
  The prompt now forbids it *and* the runtime refuses it, because caching makes
  this the one mistake that does not announce itself. Values shorter than three
  characters are exempt: a bare `10` occurs in ordinary prose and inside longer
  numbers, so matching on it would reject correct templates.

**Where validation lives.** The decider decodes; the router judges. Validating
inside the decider was tried and reverted: the error came back shaped like a
transport failure and skipped the retry ladder, so a fixable answer ended the
task instead of being sent back with the reason.

**Token budget.** Routing allows 16384 tokens against a decision that is a few
hundred bytes. The budget is not for the answer but for the reasoning several
free models emit before it, charged to the same allowance. At 4096 and at 8192
responses were cut off mid-reasoning and never reached the JSON — surfacing as
a token-limit error rather than as anything resembling a routing problem.

**Known limits.** Shaping is lexical, so paraphrases still miss the cache
(Q-12, now sharper — it applies to routing as well as extraction). Bare
hostnames are recognised by TLD rather than by parsing, so an unusual TLD is
read as prose; an explicit scheme always works.

## D-040 — An application's own dictionary is the capability surface

Driving applications means knowing what they can do. The two obvious routes are
both wrong: asking a model what an app supports invents commands that do not
exist, and hand-writing an adapter per app — which is what `appleevents.Notes`
is — does not generalise past the apps somebody found time for.

Every scriptable Mac application already ships a machine-readable answer. The
`.sdef` in its bundle is a first-party declaration of its classes, properties
and commands, written by whoever wrote the app. It is the same kind of object
as a page signature: something the runtime reads rather than something the
model asserts.

**Read from the bundle, not from `sdef(1)`.** The `sdef` command-line tool
resolves includes properly but requires a full Xcode install, and a runtime
that stops working because someone has only the command line tools is a runtime
with a dependency it never declared. The dictionary is globbed out of
`Contents/Resources/*.sdef` — exactly one on every one of the 21 scriptable
applications on the development machine.

**Includes are spliced, and exclusions honoured.** Cocoa apps pull in the
standard suite with `xi:include` rather than redeclaring it, so an unresolved
include loses `make`, `get` and `delete` — most of what is worth driving. Four
of the dictionaries on this machine use an xpointer that *removes* commands
from that suite: Safari does not respond to `make`, and others drop `delete`,
`duplicate` or `move`. Those exclusions are load-bearing. An app that declares
it does not do something and is asked to do it anyway fails at the Apple event,
long after the plan the user confirmed said it would work. An xpointer in any
other shape is an error rather than a guess.

**Ambiguity is refused, not broken by ordering.** `Find("mai")` with both Mail
and Mailbox installed returns both names and no app. Acting in the wrong
application is precisely the failure a confirmation gate cannot catch: the
plan would name the app the runtime chose, and it would be the wrong one, and
it would look correct.

**What this is not yet.** Discovery only. Nothing here writes a program,
executes an Apple event, or decides what a confirmation looks like when the
side effect leaves the machine.

## D-041 — What an application's surface is, and what it is not

The dictionary (D-040) is the raw material; the surface is what a model is
actually shown. Building one turned up three things that are not obvious from
reading the grammar.

**`get` and `set` are in no dictionary.** Not in any application's, and not in
`CocoaStandard.sdef` either. They belong to the Apple Event Object Model
rather than to any application's vocabulary, and every scriptable application
answers them — which is also why JXA spells a property read as a call,
`note.body()`, rather than as a command. A validator built only from declared
commands rejects every program that reads anything. They are seeded into every
surface.

**Hidden does not mean refused.** `hidden="yes"` asks a dictionary browser not
to list a command. The application still answers it. Hidden commands are
therefore collected for validation and left out of the rendering: the model is
not told about them, and a program that uses one is not rejected for it.

**Objects mostly arrive by class-extension.** Notes declares no `application`
class; it extends the standard one and hangs its notes and folders off that.
A surface built from `<class>` elements alone reports an application that can
do nothing with notes. Extensions are merged into the class they extend, and
into a synthesised one if it was never declared.

**Size is not the constraint it looked like.** Every one of the 21 scriptable
applications on this machine renders in under 2200 tokens — Notes 759,
Reminders 688, Messages 359, the widest, Keynote, 2216. Truncating to a budget
was built first and then removed: a surface trimmed to fit is a silent claim
that an application cannot do something it can, and there was no need for it.
What remains is a backstop at 24000 bytes that nothing reaches, which drops
whole object types rather than properties and says how many it dropped.
Commands are never dropped, because they are what the validator checks
against.

It is a cold-path cost in any case. The program that comes out of this is
cached by instruction shape; the warm path spends nothing.

## D-042 — An application program is data, not code

A model asked to drive an application will happily write JXA. Taking it would
hand the runtime a string whose meaning can only be established by reading it,
and reading arbitrary JavaScript well enough to say what it will do to
somebody's Mail is not something a validator can be trusted to do. The check
that matters — does this change anything? — would come down to the model's own
account of itself.

So the model does not write code. It chooses from the vocabulary the
application declares: a list of steps, each naming a command, what to act on,
and what to set. The runtime renders the script. This is the selector program
(D-002) one tier up, and the same trade: less expressive, in exchange for
something that can be checked, cached and replayed.

**What the floors are.** Every one consults the dictionary rather than the
model, so none can be argued past:

- The program must address the application it was written for.
- Every command must be one the application declares, or one the object model
  provides. Invented verbs are rejected by name.
- Every class must be one the application declares.
- Every property must exist on the class it is set on, **and be writable**.
  That last one is worth the most: a read-only property fails at the Apple
  event, which is to say after the user has confirmed a plan that said it
  would work.
- A step may only look inside a binding an earlier step produced.

**Whether it writes is not the model's to say.** `Writes` asks the dictionary.
Whether a confirmation appears is the one decision on that screen with
consequences.

**Values cannot become code.** Every value reaches the script through
`encoding/json`, which closes quotes, backslashes, control characters and
U+2028/U+2029 — the last of which JavaScript treats as a line terminator and
JSON does not. Identifiers never go through it: only a name checked against the
dictionary is ever written out, and `identifier()` stands behind that as the
check that holds if an earlier one is wrong. HTML escaping is off, because `<`
does no harm inside a string literal and `<div>` in a script the user
is about to confirm does.

**Two things the shape of JXA forces.** Collections are addressed by plural —
ten dictionaries on this machine declare `plural` on the class and those are
used verbatim, the rest get ordinary English. And a bound lookup must stay a
specifier: in JXA a specifier is a function, so calling it turns a live
reference into a dead copy, and a step that binds a list followed by one that
puts something in it would be addressing an object the application no longer
recognises. Evaluation happens once, at the end, on the result being reported.

## D-043 — Acting in an application, end to end

`app.act` is live: routed, written, validated, cached, previewed, confirmed and
run. Cold costs one routing call and one synthesis call; warm costs none.
Measured against Notes with a real model:

    cold   2970 ms   1 decision + 1 synthesis
    warm     11 ms   0 calls
    warm     10 ms   0 calls

**The bug that nearly shipped.** The first working version cached the program
under the instruction shape and passed the model the *resolved* action — the
one with this instruction's words already in it. So the program came back with
the value written into it:

    {"command":"make","make":"note","with":{"body":"pick up the dry cleaning"}}

and every later note of that shape got the first note's text. Warm runs were
fast, made no model calls, reported success, and wrote the wrong thing. Nothing
failed. It was found by running three requests in a row and reading the
previews rather than the timings, which is the only way it shows.

This is D-039's leak floor one tier down, and it needed the same two answers:
the model is told plainly that its program is kept and reused, and the runtime
refuses a program that hardcodes a value the shape has a placeholder for.
Synthesis now happens against the templated action, the template is what gets
cached, and filling happens at use:

    {"command":"make","make":"note","with":{"body":"{s0}"}}

Filling touches values only. Commands, classes and bindings came from the
dictionary and are not the instruction's to change — a placeholder there would
be a program rewriting its own structure per request. The filled program is
validated again before it runs.

**Staleness is checked on the way out.** A cached program was valid against the
dictionary the day it was written. An application update can withdraw a command
or make a property read-only, so a cache hit is re-validated and dropped if it
no longer fits — the same treatment a cached routing decision gets.

**Capabilities are asked of the build, not of a constant.** Acting needs four
things together: the catalogue, a backend that can write programs, somewhere to
cache them, and something to run them. A build missing any of them declines
`app.act` with what it *can* do, rather than accepting the request and failing
partway through. The list shown after "I didn't catch that" is generated from
the same check, so it cannot offer what this runtime was not assembled to do.

## D-044 — Drive the browser the user is already in

The debugging port reaches the wrong browser. Chrome 136 and later refuse
`--remote-debugging-port` on the default profile — deliberately, because an
open debugging port on a signed-in browser is every cookie in it — and passing
the flag to a Chrome that is already running does nothing at all, since the
second process hands off to the first and exits. Between them, CDP means a
second Chrome on a throwaway profile, signed in to nothing. Most of what
anyone actually wants to ask about is behind a login.

Chrome is scriptable, and the tier built for acting in applications (D-040 to
D-043) already knows how to talk to scriptable applications. Its dictionary
carries everything needed: `execute … javascript:` for the rendered DOM, a
writable `URL` on a tab, `loading` to wait on, and a read-only `id` per tab.

**One gate, and it is the right one.** Chrome keeps "Allow JavaScript from
Apple Events" off by default, for the same reason it closed the debugging
port. It is a menu item, once, and it gets its own error code so the window
can say which menu item rather than telling someone to relaunch their browser.

**VANS opens its own tab.** The first version took over the frontmost tab,
with a comment arguing that leaving tabs behind was the worse side effect.
That was wrong and obviously so: the tab someone is reading is theirs, and
taking it over to go and look something up is the rudest thing a background
agent can do to a browser. VANS opens a tab, puts the active tab index back so
the view does not change underneath them, addresses it by id for the rest of
the task, and reuses it for later ones. If it is closed, the next task opens
another.

**The port stays as a fallback.** A live browser that is running with
scripting on needs no setup at all, so preflight prefers it and only falls
back to reporting on the debugged one. When both fail, the error with a fix in
it wins: "turn on this menu item" beats "no page target among 0 targets",
which is a description of a second browser the user never wanted to run.

**Schemes.** The router hands over what the instruction said, and instructions
say `news.ycombinator.com`. CDP guesses; the dictionary refuses with "Invalid
URL entered", which is true and unhelpful. The adapter supplies the scheme.

## D-045 — What the live browser changed about caching

Reading the page through the browser the user is actually in (D-044) changes
what gets signed. Measured on DuckDuckGo, three identical searches:

    run 1   sig aa9fa438   1 synthesis call
    run 2   sig d7435ee2   1 synthesis call
    run 3   sig aa9fa438   0 calls

The results page renders more than one DOM skeleton for the same query. The
cache is not failing — run 3 is a hit on run 1's signature — but each variant
has to be met once before it is free. This is the case D-029 kept several
programs per key for, arriving in practice rather than in theory.

**Google is no longer the default.** It randomises the class names on its
results page, so its signature never repeats: every search would be a cold
run, two model calls and seven seconds, for the same question asked twice.
DuckDuckGo renders a small number of stable shapes, which is what makes the
second ask free. Naming an engine in the instruction still works, Google
included — the default is a choice about what caches, not about what is
allowed.

**Both reasons survive a browser failure.** When the live browser and the
debugging port both fail, the live one's error is what goes back, with the
fallback's appended. Discarding it left only "no page target among 0 targets",
which describes a browser the user never wanted to run and says nothing about
why the real one could not be driven.

## D-046 — Two things a real browser made obvious

**One tab, remembered.** The live adapter opened a tab per engine process and
abandoned the last one, so a browser ended up with nine of them. The id now
lives in the store, and a restarted engine adopts the tab rather than opening
another. Verified across two processes: the tab count did not move.

**A mixed results page is not a broad selector.** The drop-rate floor (D-016)
rejects a root that matches far more elements than it yields rows, on the
grounds that it has grabbed a container class instead of the records. That is
the right instinct and it was catching the wrong thing. A search results page
puts results, image strips, "people also ask" and video carousels under one
class as siblings, so more than half of them legitimately hold nothing a
record needs — and the ones that do are the answer.

Found on a real Google page: nineteen elements, eight complete records, and
the program thrown away with the candidate list on screen saying
`div#rso > div.MjjYud — 8 records`. The runtime had the answer and refused it.

The floor now fires only when a high drop rate is *also* accompanied by few
surviving rows, which is what taking a container actually looks like — a
handful of rows at most. Both sides are tested: eight records among nineteen
elements pass, one record among ten still fail.

## D-047 — An answer, and the one thing that cannot cache

Search returned a table. For "who is X" the table is not the answer; it is the
raw material for one. The runtime now writes a reply over the rows it already
has, and shows which rows it rests on.

**This is the boundary of the caching claim, and it is worth stating plainly.**
Everything else caches because it is about a *shape* — this kind of
instruction, this kind of page. An answer is about this *content*. Two
questions of the same shape have different answers, so there is nothing to
replay. A warm run is therefore zero calls for routing and extraction, plus
one small call over rows already in hand. Pretending otherwise would mean
either caching an answer to a different question or not answering at all.

**Grounding is a floor, not an instruction.** A model given thin results
answers from memory, and a remembered answer reads exactly like a read one:
fluent, specific, and about something else. So the runtime checks rather than
asks —

- an answer citing nothing is refused, because that is what answering from
  memory looks like;
- a citation past the end of the results is refused;
- a link that appears in no row is refused, because a fabricated URL is the
  one invention that cannot be caught by reading.

Refusals go back with the reason, the same ladder everything else uses.

**A failed answer never fails the task.** The rows are extracted and worth
showing on their own. Trading a working result for a missing summary would be
losing the thing that works to the thing on top of it.

**The citations are shown, not kept as provenance.** An answer written off a
page is worth what the page is worth, and the quickest way to judge it is to
see which results it used. Clicking one goes to it.

## D-048 — Dismiss on a click, not on losing focus

Dismiss-on-blur closed the panel with nobody touching anything. The log, once
it printed every window event rather than only the one being acted on:

    window event: Focused(false)
    blur re-check: key=false visible=true
    hiding: lost the keyboard after a blur     (three times, untouched)

A panel that floats over full-screen applications and never activates gives up
key status constantly, and almost never because the user left: the menu bar
auto-revealing does it, so does a full-screen window's own controls appearing,
so does being moved or resized. Two attempts were made to keep blur and
qualify it — ignore it while dragging, ask AppKit again a moment later — and
each added a condition that was correct in one case and wrong in another.

Blur is the wrong event. The one that means "I have gone somewhere else" is
the mouse going down in another application, and it is available: a global
`NSEvent` monitor for mouse-down. Hovering, scrolling, and a menu bar sliding
into view produce none. Clicks inside the panel never reach a global monitor,
and the click's position is checked against the window frame anyway.

**Dragging moved to the shell.** The frontend's route to the window position
was two async calls that could fail into a console nobody is reading, which
is indistinguishable from a drag that does nothing — and that is exactly how
it presented. The shell reads the position when the drag begins and the
frontend sends screen deltas, so a failure prints somewhere it can be seen.

## D-049 — The page already on screen

VANS could search the web and read a page it was given an address for. It
could not read the one in front of you, which is most of what anyone means by
"summarise this".

**Its own kind, not web.extract with a missing url.** A missing required slot
is a rejected decision, and this decision is complete: the user did supply the
page, by having it open. `page.here` takes one slot — what to get — and the
address comes from the browser.

**It navigates nowhere, and that is the point.** Going anywhere would answer a
different question and would throw away the state that made the page worth
asking about: a search that had been refined, a list that had been filtered,
anything behind a login. It reads the active tab of the frontmost window —
deliberately not the tab VANS opened for itself (D-044), because a request
about "this page" means theirs, and reading the wrong one answers confidently
about a page they cannot see.

**One ladder, not two.** Everything after the page is in hand — the cache, the
rungs, the model, the verifier — was split out of Prepare so a page the
runtime navigated to and a page the user already had open go through exactly
the same path. Two copies would drift, and the one that drifted would be the
one nobody was watching.

Proven against a real tab: a Prime Video page, three rows, answered without
the tab moving.

## D-050 — Reaching applications that declare nothing

The application tier (D-042) reaches 21 applications on this machine and none
of the ones people spend the day in. Telegram, Notion, Discord, Slack and
VS Code ship no scripting dictionary, so there is nothing to validate against
and nothing to drive.

**The ladder, cheapest first.** Acting in an application resolves in order:

1. Its scripting dictionary. Fastest, and the only tier that can say from a
   declaration whether a command changes state.
2. Its web version, in the browser already being driven. No new permission, no
   new dependency, and the user is already signed in.
3. The accessibility tree (D-051). The only tier left for something with
   neither a dictionary nor a web version.

An application name that resolves to no installed bundle now falls to rung 2
before it is refused, because "Telegram" usually means the tab.

**The page is the authority.** A page declares nothing, so the checks are
against the DOM that was actually captured: every selector must match exactly
one element, and text may only be typed into something that can hold it.
Ambiguity is refused rather than resolved — the plan says "click Send", four
things match, and the user confirms one while another happens. Typing into a
`<span>` is refused because it does nothing at all, which is worse than
failing: the plan is confirmed, the click lands, and the message is empty.

**Everything is confirmed.** There is no dictionary to ask whether a click
sends a message or opens a menu, so every page program goes through the gate.
The preview is the model's own description of each step — that line is the
only thing between the user and a message they did not mean to send.

**Nothing is verified afterwards.** A page states no identity for what a click
did, so unlike an application (D-004) there is no object to ask about. The
receipt says unverified rather than assuming, because the difference is real.

**Re-checked at run time as well.** The DOM was captured a moment ago and a
live page moves. An element that was there when the plan was made can be gone
when it is confirmed, and clicking nothing while reporting success is the
failure worth preventing. Text is set through the value setter and followed by
input and change events, because every framework worth naming ignores a value
that arrives without them — the box shows the text, the application never sees
it, and the message sends empty.

## D-051 — Acting through the accessibility tree

D-050 left rung 3 unbuilt, so an application with no scripting dictionary and
no web version was simply out of reach. Spotify, the desktop Discord build and
every Electron application that declares nothing are that case, and between
them they are most of what is actually on screen.

**Through System Events, not the C API.** macOS exposes the accessibility tree
twice: `AXUIElement` in ApplicationServices, and the Processes suite of System
Events over Apple events. The C API is faster and wants cgo, a framework link
and a second way of talking to the system. The Apple-event route goes through
the same osascript runner every other tier already uses, so this costs no new
dependency and no new build constraint. If walking a window ever gets too slow
to sit through, that is the moment to reach for the C API — not before.

**The permission is real and it is the user's.** System Events reports
`uiElementsEnabled` false until VANS is ticked in System Settings › Privacy &
Security › Accessibility, and until then every call comes back "not allowed
assistive access". It gets its own sentinel and its own fix string, surfaced
the way Chrome's scripting gate is: a permission whose remedy is a switch the
runtime must not reach over and flip for itself (D-032).

**The tree is the authority.** The same argument as the DOM in D-050, one tier
down. A window declares nothing about what its buttons do, so every check is
against what was captured: a selector must match exactly one control, text may
only go into something that can hold it, and a disabled control is refused.
Where several controls share a role and a title, the tree numbers them and the
program has to say which one it means.

**Resolved fresh, never remembered.** A control is reached by its index path,
which is exact and cheap, but the path is not part of the program. The window
is captured again when the plan is confirmed, the program is re-checked
against that capture, and the paths are worked out from it. A window that was
rearranged between the plan and the confirmation is followed rather than
refused; a control that has genuinely gone is refused rather than resolved to
whatever slid into its position. Clicking the wrong button reports success
exactly as cheerfully as clicking the right one, which is why the check is on
the role and the title and never on the position.

**It caches, because the program is a shape after all.** The first cut of this
tier cached nothing, reasoning that a control is addressed by an index path,
and a path describes this window at this scroll position rather than a kind of
request. That was the wrong level to look at: the model never writes a path.
It writes a role and a title, which is exactly a shape, and the runtime
resolves the rest. So a window program is cached by application and
instruction shape like every other tier, and a warm run costs no model call.
The same change retires the stale-path risk, because no path survives between
the plan and the run.

**What it costs instead is time.** Reading 218 controls out of a System
Settings window takes about 24 seconds over Apple events, because every
attribute read crosses a process boundary. Asking a whole collection for its
roles in one event, rather than each element in turn, is what brought it back
under the timeout it first blew through — and it is still the slowest thing in
the runtime. That measurement is what decides when `AXUIElement` and cgo earn
their cost; until a window like that reads in a second or two, this rung stays
the last one tried.

**Everything is confirmed, and typing is verified.** Nothing declares whether
a click sends a message or opens a menu, so every program goes through the
gate. After a type step the value is read back off the control and compared
with what was meant to be there — more than the page tier can say (D-050),
less than a dictionary's receipt (D-004).

**Only the application in front exposes a window.** Measured on this machine,
System Events lists windows for the frontmost process and reports zero for
every other one: with the editor in front, System Settings, Notion, Telegram,
WhatsApp, Chrome and Finder all read as zero windows, twice each, while each of
them read as one when its turn came to be in front. The successful capture of
218 controls happened to a window that was in front at the time, and the same
call minutes later, backgrounded, refused.

So this rung reaches what the user is looking at and nothing else. That is
narrower than it first sounds and more coherent than it looks: it is the same
premise as reading the page in front (D-049), and a request to drive a
background application is refused with a reason rather than acted on blind.
It also means the tier cannot be exercised against a safe application without
that application being the one on screen, which is why typing through it is
recorded here as unproven rather than quietly skipped.

## D-052 — An answer that goes into the application in front

B-002 asked for a destination that puts the answer into another application,
the way Note puts it into Notes. "Whatever application makes sense" was never
answerable in the abstract; what makes it answerable is D-051, which reaches
exactly one application — the one in front. So that is what this destination
means, and it is also what a person means when they say put it here.

**The panel not taking focus is what makes it possible.** The overlay is a
non-activating panel, so the application the user was working in stays the
frontmost one while they type into VANS. Without that there would be nothing
behind to write to. When VANS itself does come out in front, the sink refuses
and says so, rather than typing the answer into its own window.

**The cursor decides where the text lands.** The control holding focus gets it.
With nothing focused and exactly one control on screen that can hold text, that
one gets it. With several and no cursor, it refuses — that is the case a
confirmation cannot save you from, because the preview names one control and a
different one is filled, and a person confirming a plan reads the sentence, not
the window.

**Gated and verified.** Typing into somebody's editor changes something outside
VANS, so it goes through the confirmation like the clipboard does, and the
value is read back afterwards to say whether it landed (D-004). Reading back
what was typed is the one verification this tier can honestly offer.

**No change to the window.** Destinations are asked of the runtime rather than
listed in the frontend, so registering the sink is what made the button appear
— and a build that cannot reach the accessibility tree simply does not offer it.

## D-053 — The runtime decides what to suggest

The four "try asking" lines were hardcoded in the window, and correct only
because someone checked them by hand against what the engine could carry out.
That is a promise maintained by memory, and the failure mode is the panel
offering something that then cannot be done — which reads to a user as the
runtime being broken rather than the list being stale.

**The check is mechanical now.** Every suggestion carries the kind of task it
needs, and the runtime filters the list through the same `runs` test the router
is answered with, plus a check that any destination it names actually exists.
A build assembled without a Notes sink stops offering Notes without anybody
remembering to edit a list, and a build that cannot reach the accessibility
tree does not suggest typing into an application.

**The window asks rather than knows.** Same shape as destinations: the panel
fetches the list and renders whatever comes back. The built-in four remain as
the fallback for a runtime that has not answered yet — never as a second
opinion, because a window that can disagree with the runtime about what is
possible is the problem this removes.

**What is on screen leads.** The frontmost application already reaches the
panel, so a browser in front puts "what is this page about" at the top. That
is the whole of the context tier for now, and it needs no new storage.

**What is deliberately not built.** B-001 wanted the strongest signal first:
suggest what is already warm in the cache, since a warm shape costs no model
call. The store cannot answer that in words. It keeps shapes — `search {s0}` —
and never the literals that filled them, because persisting them is exactly
what D-039 refuses. Turning a warm shape back into a sentence a person would
recognise means storing what they typed, which is the most sensitive thing
VANS would hold. That is a decision about retention, not a feature to slip in
behind one, so it waits for an explicit answer.
