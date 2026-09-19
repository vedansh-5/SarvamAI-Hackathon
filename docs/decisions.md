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

**Through System Events, not the C API.** *(Reversed — see D-057.)* macOS
exposes the accessibility tree twice: `AXUIElement` in ApplicationServices, and
the Processes suite of System Events over Apple events. The Apple-event route
went through the osascript runner every other tier already used, so it cost no
new dependency. The condition set here for reaching for the C API — a window
too slow to sit through — was met, and the decision went the other way.

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
which is exact and cheap, but the path is not part of the program. The paths
are worked out from a capture, and the program is re-checked against that
capture before anything runs. A window that was rearranged between the plan
and the confirmation is followed rather than refused; a control that has
genuinely gone is refused rather than resolved to whatever slid into its
position. Clicking the wrong button reports success exactly as cheerfully as
clicking the right one, which is why the check is on the role and the title
and never on the position.

**The capture from preparing was reused while it was seconds old.**
*(Removed — see D-057.)* It existed because reads cost twenty seconds, and it
was safe because the renderer of the day re-checked the role and title at each
path as it ran. Neither is true any more: a read costs under half a second, and
the renderer that acts now clicks coordinates and re-checks nothing. The window
is read again every time.

**It caches, because the program is a shape after all.** The first cut of this
tier cached nothing, reasoning that a control is addressed by an index path,
and a path describes this window at this scroll position rather than a kind of
request. That was the wrong level to look at: the model never writes a path.
It writes a role and a title, which is exactly a shape, and the runtime
resolves the rest. So a window program is cached by application and
instruction shape like every other tier, and a warm run costs no model call.
The same change retires the stale-path risk, because no path survives between
the plan and the run.

**What it costs instead is time.** *(Superseded by D-057.)* Over Apple events
every attribute read crosses a process boundary, and 218 controls out of a
System Settings window took about 24 seconds. That measurement is what decided
when `AXUIElement` and cgo earned their cost. They earned it.

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

**A dead end worth naming, so it is not tried twice.** `entireContents()` looks
like the answer to the round trips: one Apple event for every descendant of a
window. It is not. What comes back is a plain array of specifiers rather than a
collection, so it answers no bulk property read — `role()` on it is not a
function — and every attribute goes back to one event per element, which is the
cost being avoided. Measured against an Electron window it also returned
nothing at all, for the same reason Chrome does: those applications do not
publish a tree until something convinces them to. If this ever needs to be
faster, the saving is in walking fewer containers, or in the C API, and not
here.

## D-054 — Working through a task on screen, where it can be watched

Every tier before this one drives an application through an interface: Apple
events to its dictionary, JavaScript into its page, accessibility calls against
its controls. They change things, and none of them is visible. Nothing moves,
nothing is typed, and a request is one validated program rather than a task
being worked through. That is the gap: a task like "reply to this and send it"
is several steps, each one decided by what the last one put on screen.

**Real input, not API calls.** A step now happens the way a person would do it:
the pointer goes to the control and clicks, and text arrives as keystrokes.
System Events synthesises both, so they are ordinary events as far as the
application is concerned, and they are visible as far as the user is concerned
— which is the whole point. Where to click comes from the same capture as
everything else, since accessibility reports a position and a size for each
control. A control that reports no box is refused rather than clicked at a
guess.

**One step at a time, because the screen answers back.** The model is given the
task, what is on screen now, and what has already been done, and it chooses one
step. Then the screen is read again. A whole sequence written up front is a
sequence written against a screen that no longer exists by step three: the menu
opened, the dialog appeared, the list scrolled. Deciding one step at a time is
slower and it is the only version that is honest about what it can see.

**Every step is still checked before it runs.** The tier is a loop, not a
licence. Each step is validated against the capture it was chosen from — the
control must exist, be enabled, match exactly one thing, and hold text if text
is going into it (D-051). A step that fails is not executed; the model is told
why and asked again, once. This is D-042's argument surviving contact with a
loop: the program is data and it is checked, even when it is one step long.

**Perception is the tree, not a picture.** The shell can already take a
screenshot, and nothing consumes it: the synthesis client sends a plain string
as message content, so images would need a different message shape and a model
that reads them. The accessibility tree carries roles, titles, values, focus
and now coordinates, which is what choosing a control actually requires. A
picture would say what it looks like; the tree says what it is.

**Confirmed once, then watched.** A plan says which application, the task in
the user's words, and that it will take up to twelve steps on screen. That is
the gate. After it, each step is logged as it happens and the panel already
renders progress, so the run is narrated while it is visible. Twelve steps is
where it stops and says it did not finish, rather than continuing to click.

**Not yet proven.** No click and no keystroke has been synthesised on this
machine. `se.click({ at: [x, y] })` is the documented bridge form, the
permission is granted, and the argument above is untested against a real
window — deliberately, because the first real click belongs in a supervised
test rather than in a background probe.

## D-055 — Remembering the last three instructions

D-053 stopped short of the thing B-001 actually wanted. Suggesting what is
already warm in the cache is the strongest signal there is — a warm shape costs
no model call — but the store keeps shapes and never the literals that filled
them, so a warm entry reads `search {s0}` and cannot be shown to anyone. Making
it readable means keeping what the user typed, and that was a decision to ask
for rather than to make quietly. The answer was the last three.

**A cap, not a policy.** Three is enough that the panel leads with something the
person actually wants and small enough that it is not a record of anyone's day.
It is also a number that can be checked by looking at the table, which a
retention policy expressed in prose is not.

**The kind is kept with the text.** A remembered instruction goes through the
same mechanical check as a built-in one (D-053): it carries the kind it routed
to, and it is only offered if this build still runs that kind. Trusting it
because it worked once would be the same promise-by-memory that check exists to
remove — an adapter can be absent from a build, and then the panel would be
offering something it cannot do.

**Asking twice does not fill the list.** Re-asking the same thing updates when
it was last used rather than adding a row, so three repetitions of one request
leave room for two others.

**It can be forgotten.** Keeping what someone typed comes with a way to remove
it, so the table can be emptied outright. That is the part of this that is not
a feature.

**It turns out to be the warm-cache version anyway.** Re-asking a remembered
instruction lands on the shape it routed to the first time, which is cached, so
it costs no routing call and usually no synthesis call either. B-001 wanted
suggestions that are cheap because they are warm; these are, by the same
mechanism, and they are readable because they are the sentence rather than the
shape.

## D-056 — What the screen tier can actually reach, measured

D-054 was written without knowing which applications publish a tree at all.
Measured on this machine, the answer splits cleanly and it decides what the
tier is worth.

**Native applications publish a real tree.** Notes reports one window with
seven children, System Settings reported 218 controls when it was in front.
Everything the tier does — choosing a control by role and title, clicking where
it is, typing into it — works against these.

**Electron applications publish windows and nothing inside them.** WhatsApp
reports three windows with zero children; VS Code reports two with zero. The
window is there and the contents are not, so there is nothing to choose a
control from.

**Three attempts to unlock them from Apple events, all negative.** Chrome's
process does not expose `AXManualAccessibility` at all, so it cannot be set
through System Events, which can address an attribute that exists but cannot
create one. Where the attribute does exist — VS Code, Notion — setting it true
left the children at zero. `AXEnhancedUserInterface` exists on every process
checked, and setting it on Chrome changed nothing. Every flag was put back as
it was found.

**Zero windows usually is not gating.** Notion, Telegram and Finder all read as
zero windows while something else was in front, and the same applications read
as one when it was their turn. That is D-051's rule, not a locked tree, and the
two are easy to confuse from a single reading.

**What would reach Electron.** The flag is meant to be set by an assistive
client through the accessibility C API against the application's AXUIElement,
which is a different thing from asking System Events to set an attribute. That
means cgo and a framework link, which this project has deliberately never had
(D-051). It is a decision about what VANS is willing to become, not a detail to
slip in behind a probe.

## D-057 — Reading the tree through the C API

D-051 said the C API was the thing to reach for if walking a window ever got
too slow to sit through, and not before. It got too slow. Measured on the same
machine: 119 controls out of a Notes window in 132 milliseconds through
`AXUIElement`, against roughly 24 seconds for 218 controls through Apple
events. That is not a tuning difference, and every argument for staying on the
Apple-event route was an argument about cost that no longer holds.

**cgo, and a build that fails honestly without it.** The walk lives behind
`darwin && cgo`, with a fallback for every other build that refuses rather than
returning an empty tree. A tier that silently sees nothing is worse than one
that says it was built without the thing it needs.

**The pid comes from the process table first.** `AXUIElementCreateApplication`
wants a pid and the callers have a name. `pgrep` answers that in 12 to 15
milliseconds where an Apple event took 101 to 146, so it is asked first. It
matches the executable rather than the application, though, and the two differ
— Visual Studio Code runs as Code — so a miss falls back to the System Events
lookup, which knows an application by the name every other tier uses. A full
capture of a Notes window is now 117 milliseconds, against 410 when every
lookup crossed a process boundary.

The fast path also made two error tests stop testing anything: they fed a
canned refusal through a fake runner for an application that happens to be
running, so `pgrep` answered and the fake was never reached. They name
something nothing runs under now, which is the path they were always about.

**The boundary moved, because the tests could not reach it.** The engine used
to hold an osascript runner and hand it to `uiact.Capture`. Once the walk went
through the C API there was nothing left for a fake runner to fake, and five
tests failed for that reason alone. That is the useful kind of test failure: it
was pointing at a dependency that had always been wrong. The engine now depends
on `uiact.Screen` — capture, act, frontmost — and the osascript implementation
is one of its satisfiers.

**The path renderer is gone.** It re-addressed a control by walking
`uiElements[i]` through System Events, using index paths recorded at capture.
Those paths now come from the C API, whose child ordering need not agree with
the Apple-event bridge's. Nothing in production used it any more, and a
renderer that can quietly click the wrong control is not worth keeping for its
tests.

**Electron is still out of reach, and that was checked properly.** A single
reading suggested Chromium windows had children after all; it did not survive
five passes, and the stable answer is the one D-056 already recorded. Neither
`AXManualAccessibility` nor `AXEnhancedUserInterface` changes it, through
either interface. What this work bought is every native application, quickly —
not the Electron ones, which still belong to the page tier.

## D-058 — Checking what is under the cursor, and what leaves the machine

Two things were wrong with acting on screen, and they are both about trust
rather than capability.

**A coordinate is not a control.** The visible renderer clicks a point, and a
point is whatever happens to be there when the click lands. The capture said a
button was at those coordinates; between the capture and the click a sheet can
open, a list can scroll, another application can come forward. Clicking the
wrong thing reports success exactly as cheerfully as clicking the right one,
which is the failure this tier exists to make visible rather than to commit.

So every step is now checked against the screen immediately before it runs.
`AXUIElementCopyElementAtPosition` says what is actually at the point, and the
step is refused unless that element has the role and title the plan named, and
unless it belongs to the application being driven. A window that moved, a
dialog that appeared, another application sitting over the target: all refused
with the reason, rather than clicked.

**The check has to come after the application is in front.** The first version
verified before the script ran, and the script's own first action was to bring
the application forward — so the target was routinely still behind something
and every step would have been refused. Bringing the window forward is now a
step of its own, and the check happens once it is there.

**The window's contents were going to the model.** The captured tree carried
every control's value, and the tree is what the prompt is built from. On a
Notes window that is the text of the note; on a mail window, the message. None
of it was needed: choosing a control needs its role and its title, not what it
holds. Values are now withheld — a control that holds text is described as
holding text — with one exception, the control that has the cursor, because a
step that types into a field is often deciding what to do about what is
already in it.

**One permission, and no pictures.** The tier needs Accessibility and nothing
else. The obvious way to reach applications that publish no tree is a
screenshot and a model that can read it, and that is a worse trade twice over:
it wants Screen Recording, and it sends an image of whatever is on screen to
somebody else's computer. Reading the tree keeps both the permission and the
data smaller, and an application that refuses to publish a tree is left to the
page tier instead.

## D-059 — The accessibility route to Electron applications is closed

D-056 recorded that Electron applications publish windows with nothing inside
them. Everything since has been an attempt to get past that, and all of it
failed in ways worth writing down so the ground is not covered twice.

**Both interfaces agree.** Read at the same moment, System Events and
`AXUIElement` return identical numbers: Code, Notion and WhatsApp at zero
windows, Notes at one window with seven children. An earlier reading that
seemed to show the C API seeing more was two measurements taken minutes apart,
not two interfaces disagreeing. There is no bridge problem to route around.

**When they do publish windows, the windows are empty.** WhatsApp read as two
windows with zero children across five consecutive passes. That is the actual
shape of the problem: not a missing window, an empty one.

**Neither flag opens them.** `AXManualAccessibility` does not exist on Chrome's
process at all, and where it does exist — Code, Notion — setting it returns
success and changes nothing, at one, three and six seconds, against freshly
created elements rather than cached ones. `AXEnhancedUserInterface` exists
everywhere and changes nothing either. Both were restored after testing.

**What is left, and what is refused.** An Electron application will publish its
tree if it is told to at launch, which is the user's business and not something
VANS does (D-032). A screenshot and a model that can read it would work, and it
is refused: it wants Screen Recording on top of Accessibility, and it sends a
picture of whatever is on screen to somebody else's computer. The route that
remains is the one already built — these applications nearly all have a web
version, and the page tier drives pages.

## D-060 — Reaching the applications that publish nothing, without taking more

D-059 closed the accessibility route to Electron applications. This is the
route that remains, and the three things that had to be fixed before it was
safe to take.

**The tab in front is not a fallback.** An application with no dictionary was
sent to the page tier first, and the page tier acts on whatever tab is in
front. So a request naming Spotify could synthesise a program against whatever
happened to be open and, if it validated, act there. The front tab is the right
target when the request is *about* the page in front, and never because an
application was named. The page tier now refuses when the page in front does
not belong to the application asked for.

**Acting in our own tab needed its own door.** Navigating and reading already
used the tab VANS opened for itself, but executing was hardcoded to
`windows[0].activeTab`. Wiring a web application through that would have typed
into whatever the user was reading — the same failure this project has been
caught by once already. Own-tab execution is now a separate call, a plan says
which tab it belongs to, and a test asserts an own-tab plan never reaches the
tab in front.

**The page tier was sending whole pages.** Up to sixty thousand bytes of raw
HTML went into the prompt: on a mail page, the mail; on a chat page, the
conversation. Choosing a control needs a way to name it and a label to
recognise it by, not the page. The tier now sends a list of controls — a
selector, what kind of thing it is, and its label from `aria-label`,
placeholder, name or a button's own words — and never what a control holds.
Input values are excluded by construction, and the tests assert that page text,
field values and draft text cannot appear.

**Then the route itself.** An application that publishes no tree usually has a
web version, so VANS navigates its own tab there and drives it as a page. That
costs no permission beyond the browser it already drives, sends no pictures
anywhere, and is confirmed like any other page action. It sits between the
window tier and working through a task on screen, because driving a known web
application is cheaper and more predictable than clicking around a window.

**What this does not do.** It reaches the applications in the table and no
others; an unknown name still falls through to the tiers below. The table is a
list of addresses, which is the honest shape for something that cannot be
discovered — there is no way to ask a desktop application where its web version
lives.

## D-061 — Answering about a window, and saying up front what is missing

Two gaps that only showed up once the tiers below them worked.

**"What does this say" only worked in a browser.** The request for the thing in
front went through a reader that is the browser and nothing else, so asking
about a note, a message or a PDF had nothing to read. That was never a decision;
it was what existed when the question was first asked. Reading a window costs
about a tenth of a second now (D-057), so the request looks at what is actually
in front: if the frontmost application is not a browser, its window is captured
and the answer is grounded in the controls that carry something — their kind,
their label, and their text.

It costs no synthesis to extract. There is no selector program, because there is
no page: the tree is already a list of things with labels, which is what rows
are. The only model call is the answer itself.

**The text travels here, and not when acting.** D-058 withholds what a control
holds, because choosing something to click needs its role and its label and
nothing else. A question *about* the contents is the case that argument does not
cover — the text is the answer, and withholding it would mean answering from
labels alone. So values travel on the reading path and stay home on the acting
path. That is the distinction worth keeping: not whether the data is sensitive,
but whether the request is about it.

**Accessibility is now reported before it is needed.** Nothing said the
permission was missing until a task failed halfway through, which reads as the
runtime being broken rather than a switch being off. Preflight asks whether the
trust is in place and offers the fix. Asking does not prompt for it —
`AXIsProcessTrusted` reports, and only the variant with options puts a dialog on
screen — which is what makes it safe where probing Notes is not (D-032). The
check is not required: reading pages, searching, and answering all work without
it, and a build that cannot reach a window should say so rather than refuse to
start.

## D-062 — Clicking where the control actually is, and asking the right question about what is on top

Three faults, all on the path between a captured window and a click that lands.

**Nothing on screen could be paused.** Every rendered action called `delay`
through the current application's standard additions, and that command answers
`-1708` here. It sits after every click, keystroke and key press, so the whole
renderer failed on its first step and had never once run to completion. The
pause is now `NSThread.sleepForTimeInterval`, which is in the process rather
than across an Apple Event, and is cheaper than what it replaced. An earlier
reading blamed the `frontmost` assignment for the same number; that was wrong,
and the assignment works.

**The middle of a control is not a place you can click.** A note longer than
its window makes the text area 22468 points tall, so its centre sits ten screen
heights below the desktop. The click point is now the middle of the part that
is showing: the control's rectangle clipped by every ancestor that encloses it
— a scroll area clips its contents — and by the bounds of the displays. A
control scrolled out of view has no visible rectangle and is refused rather
than clicked at a coordinate in space.

**"Is something covering this point" was asked of the wrong API.** The check
compared the owner of the accessibility element at the point against the
application being driven. With Stage Manager running, that element belongs to
`WindowManager` at every point on screen, so the comparison was false
everywhere and the guard refused every click it was ever asked about. Window
stacking is what the question is about, so it is now asked of the window list:
the frontmost ordinary window containing the point must belong to the target.
The accessibility hit test still checks that the control there is the one that
was planned, but only when it answers with the application's own element —
when Stage Manager answers instead, the control check is skipped rather than
the click refused, because the window list has already settled the question
that matters, which is whether the click reaches the right application.

## D-063 — Saying which application an action happens in

Acting in an application has worked from typed text for a while, and the window
never mentioned it. Nothing in the panel said it was possible, and the one place
an action did appear read as though it were about to happen somewhere else.

**The gate named the wrong thing.** A plan carries the application it acts in —
the dictionary tier, the window tier and the driving tier all record it — and
that name stopped at the runtime boundary. The window had only the destination,
which is where an *answer* goes, so the confirmation in front of an action in
Notes asked whether to do it in "panel". The application now travels with the
plan, and the gate, the result line, the button and the receipt all say where
the thing will happen.

**Nothing offered an action.** The suggestions under the ask field are what
tells a person what this is for, and every one of them read a page or searched.
One example that acts in an application is offered now, and only when this build
can carry it out — the same test the other suggestions pass.

**The example names an application that is actually installed.** The list of
what can be driven is discovered from the scripting dictionaries on the machine
rather than written down, so the suggestion is chosen against that list. A
suggestion naming something that is not there is the same failure as offering a
destination that does not exist: the panel promising what it cannot do.

## D-064 — Being in front is not the same as being on screen

Bringing an application forward reported success as soon as the process was
frontmost. That is not what the caller needs to know. A window is brought
forward so it can be read and clicked, and a process can be frontmost while none
of its windows is anywhere on screen — Stage Manager does exactly this to the
windows of applications that are not active. The failure then surfaced three
steps later as the accessibility API declining to list a window, which reads as
the runtime being broken rather than a window being hidden.

Forwarding now waits for a window to actually appear, briefly, and says plainly
what happened when none does, with the setting that most often causes it.

**The window list was the wrong place to ask.** It was the first answer tried,
and it failed every application it was pointed at: the only window of some is at
the desktop layer rather than the ordinary one, and for a Chromium application
the process that owns the window is not the one its name resolves to. Both would
have made forwarding refuse windows that were plainly on screen.

The question is really whether reading the window will work, so it is asked of
the same accessibility windows the capture itself needs. An application that
publishes a window with nothing in it — which is what Chromium does (D-056) — is
a different failure and keeps its own.

## D-065 — Input is posted, not scripted

Every click, keystroke and key press was rendered into JXA and handed to
System Events. None of it ever worked: `click at` answers -1700 here, so the
renderer failed on its first step in every run it ever made.

The fallback was worse than the bug. Driving the screen through System Events
means the Accessibility grant has to go to `osascript` — a binary any script on
the machine can use — rather than to VANS. That is the opposite of asking for
as little as possible, and it is not a trade worth making to keep a rendering
strategy.

Input is now posted as CoreGraphics events from the runtime itself, under the
accessibility trust it already holds to read a window. Three things follow.

**It is faster by the whole cost of the old path.** A step no longer spawns a
process and crosses an Apple Event boundary; posting an event is a function
call. The pause between steps is what remains, and that exists for the
application's benefit, not the runtime's.

**Text stops being part of a script.** It used to be interpolated into a JXA
string literal, which is why there was a test for text closing its own quote
and appending a keystroke. Nothing is parsed now — the characters are posted as
key events — so the escape hatch is gone rather than guarded.

**A program renders to data.** `Plan` turns a program and a window into a list
of acts with resolved coordinates, which is what the tests now read. The
verification in front of a click is unchanged and still runs against the window
as it is at that moment.

## D-066 — Typing where the cursor is needs no tree

The destination that types an answer into the application in front required a
typeable control in that application's accessibility tree. It refused Chrome
with "nothing on screen can hold typed text", which is true of what Chromium
publishes and false of what is on the screen.

That requirement was an artefact of how typing used to work. Text was delivered
by clicking a control and then sending keystrokes, so a control had to be found
and its coordinates resolved. Posting key events (D-065) does not need a target:
the characters go wherever the focus already is, which is what the destination
has always said it does.

So the precise path is tried first — find the control, address it, type into it,
which is the better outcome when the application describes itself — and the
cursor is the fallback when it does not. Chrome and the Electron applications
move from refusing outright to working the way the destination is described.

The gate is unchanged and still stands in front of it. "Where the cursor is" is
confirmed by the person who can see the screen, which is the only place that
judgement can honestly be made.

## D-067 — Reading the window back

Every receipt for an action said `Verified: false`, because nothing ever looked.
The runtime checked that a control was where it expected *before* clicking and
then reported success on the strength of having sent the events. A step that
landed somewhere harmless and a step that did exactly what was asked produced
the same answer.

The window is read again after acting now, and the two readings are compared.
What changed is said in plain terms — a control appeared, one is gone, one holds
different text — and that is what a person sees rather than "done".

**What can honestly be confirmed depends on the verb.** Typing has an intent
that can be checked: the text either turned up in the window or it did not. A
click does not — nothing says what a button was supposed to do — so the only
truthful test is whether the window moved at all. A click that changed nothing
is reported as not confirmed, which is the useful half of the distinction: it is
the signature of a click that missed.

**The driving loop gets this as feedback.** Each step it takes now carries the
effect it had into the next request, so the model is told "nothing in the window
changed" rather than being left to assume its last instruction worked. A loop
that cannot tell a successful step from a wasted one spends its steps repeating
the wasted one.

## D-068 — Restyling a page, and the runtime owning how it looks

A restyle marks what the user asked about on the page in front of them. The
program is a list of rules, and a rule is a selector and an emphasis — nothing
else. The model chooses what to point at and how strongly; every colour, radius
and duration is the runtime's.

**No value from the model ever reaches the page.** The first version let it write
declarations, which meant validating strings: an allowlist of properties, a
character class for values, and a ban on `url(`, because a stylesheet can
exfiltrate what its selector matched by asking for a background image. Emphases
removed the question rather than answering it. There is no value to check
because there is no value.

**A mark is a tint, not a border.** An outline around a thing reads as a
debugging tool, and a border changes an element's size, which moves the page
under the reader. Every emphasis is a background tint with a small radius, and
the one exception marks an edge rather than enclosing anything.

**The palette is chosen on the page.** Only the page knows what it is wearing,
so the injected script walks up from the body until it finds a background that
is not transparent, measures its luminance, and picks the set that will sit on
it. Tints go *down* on a dark page rather than up: a wash that reads as soft on
white glows on black.

**Contents stay on the page.** The model is given selectors and counts, never
text. The page in front of someone is as likely to be their mail or a
conversation as a shop, and a restyle decides which elements change rather than
what they say. That is D-058 applied to a second tier.

**Taking it off is not a thing to confirm.** Clearing puts the page back, so it
skips the gate, and it is recognised in Go rather than by the model: "clear the
highlighting" costs no call and cannot be misread.

## D-069 — Showing is the same loop as doing

Asking "how do I do this here" and asking VANS to do it are the same problem
read twice. Both need the window, a step written against it, and a check that
the step fits. They differ in one place: one clicks the control, and the other
points at it and waits for the person.

So there is one loop, and the caller passes in what happens to each step.
Driving hands it to the screen; coaching hands it to the overlay and then waits.
Everything else — capture, the step, the two attempts, the record of what has
been done, the effect read back afterwards — is written once.

**The wait is the interesting part.** Nothing tells the runtime that a person
has clicked something, so the window is read every half second and compared with
how it looked when the tip went up. A change means they acted, and the next step
is worked out from the window as it is now rather than as it was predicted to
be. Forty-five seconds without a change ends it, because a dimmed screen left
over someone who walked away is worse than stopping early.

**A step is pointed at where it is visible, not where it is.** The rectangle
comes from the same clipping the click point uses (D-062), so a control scrolled
half out of view is ringed around the part that shows.

**Nothing is confirmed, because nothing is done.** Pointing at a control changes
nothing, so it skips the gate — the same reason reading a page does. The guide
is driven by its own call rather than by `Commit`, which is where side effects
live.

**The panel must not take the overlay down when it dismisses.** It dismisses on
a click outside, and during a guide the first thing the person does is click
outside — on the control they were just shown. The overlay ends with the guide,
not with the panel.
