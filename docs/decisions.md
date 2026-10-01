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

## D-070 — Telling "show me where" from "make it stand out"

"Show me where the search bar is" was routed to the tier that restyles a web
page. Both kinds had been described in the same words — something on screen
standing out — and the router had nothing else to separate them, because it is
given the instruction and no idea what is in front of the person.

The descriptions now say what each is *about* rather than what it looks like:
one changes how a web page appears, the other answers a question about the
application on screen by pointing at a control. That alone routes "where is",
"how do I" and "which button" correctly, and leaves "highlight the headings"
where it was.

**A decision is only as good as the list it was made against.** The routing
cache is keyed by the shape of the instruction, so the corrected descriptions
would have changed nothing for anyone who had already asked — the wrong
decision was cached, and a cached decision was replayed without asking whether
the kinds had moved underneath it. The list of kinds is now fingerprinted and
stored with each decision, and a decision taken against a different list is
dropped rather than replayed. That is what made the fix reach an existing
install rather than only a fresh one.

**Two things recorded earlier turned out to be wrong.** Stage Manager does not
prevent a window from being read: with it on, the frontmost application
captures normally. And D-051's "only the frontmost application publishes
accessibility windows" is false — Finder published its window while another
application was frontmost. What is true is narrower: an application whose
window is not on screen at all publishes nothing, which is what the earlier
readings were actually seeing.

## D-071 — A question is not a task

The coach reuses the step writer that drives an application, which is right:
both need the next control to use. But it was handed the routing slot rather
than the words that were typed, and then the words as they stood.

Routing reduces "where is the search bar" to its slot, which is "the search
bar" — a noun. The coach now takes the instruction as the person wrote it.

That was not enough on its own. The step writer is told it is working through a
task and to stop when the task is finished, so a question handed to it straight
comes back as "nothing to do" — which is what it did, twice, silently, with no
step and no error. The question is wrapped as the thing it actually implies:
work out which control answers this and give it as the next step. The step's
target is then what gets pointed at.

Wrapping rather than writing a second prompt keeps one step writer. The two
callers differ in what they do with the step, not in what they need from it.

## D-072 — Asking who is in front without asking permission

"Which application is in front" was answered by System Events, which means an
Apple Event, which means macOS asks the user to grant Automation — and refuses
the call until they do. That refusal arrives as "macOS blocked the write" on a
request that never intended to write anything, because the destination that
types into the application in front has to know which one that is before it can
do anything at all.

The window list already knows. The frontmost ordinary window is the first
on-screen window at layer zero with any opacity, and its owner is the
application the user is looking at — the same list that answers whether a point
is covered (D-062). Reading it needs no grant of any kind.

So it is asked first, and System Events is the fallback for the case where the
list cannot answer. One fewer permission on the common path, and the panel stops
demanding Automation to do something that does not automate anything.

## D-073 — Focus movement confirms an action

D-067 established read-back verification: typing checks whether the text landed,
while clicking checks whether the window changed at all. A click that changed
nothing was reported as not confirmed, as "the signature of a click that
missed".

`uiact.Changes` tracked added and removed controls and text value changes, but
omitted focus. When a user or action clicked a text field, search box, or
focusable control without immediately altering its text, `Changes` reported
zero differences, causing `Landed` to report the action unconfirmed and the
on-screen coach to wait until timeout.

Focus transition (`!prev.Focused && el.Focused`) is now treated as a genuine
window change. Clicking a text field focuses it, which is an intentional,
state-altering user interaction. Window title transitions
(`before.Window != after.Window`) are similarly recorded as changes.


## D-074 — Raising a window with the permission already held

Every action brings its application forward first, and that was done by asking
System Events to activate it. System Events is reached over Apple Events, so
macOS requires an Automation grant — a second permission, asked for separately,
on top of the accessibility one the runtime must already hold to read the window
at all. When it is missing, the refusal arrives as "macOS blocked the write" on
the first step of anything.

Accessibility can raise an application directly: setting `AXFrontmost` on the
application element does what the activate call did. It needs nothing beyond the
trust already granted for reading, so the common path now asks for one
permission instead of two.

System Events stays as the fallback, and the window list answers whether the
application is already in front, in which case neither is used. That ordering is
the point: do it with what is already granted, ask for more only when that
fails, and do nothing at all when there is nothing to do.

**Unverified on this machine.** Accessibility trust for binaries launched from a
terminal was revoked partway through this work, so the raise could not be
exercised live — which is exactly why it ships behind a fallback rather than as
a replacement.

## D-075 — Never activate the VANS process over Stage Manager

Summoning the panel must never make VANS the active application. On macOS with
Stage Manager enabled, activating any application causes every other application
to withdraw its windows from the accessibility API: controls drop to zero and
`walkApp` fails with `ErrNoWindow`.

`overlay::show` previously called `win.set_focus()` after `win.show()`. In Tauri/tao
that reaches `[NSApp activateIgnoringOtherApps:YES]`, defeating
`NSWindowStyleMaskNonactivatingPanel`. `show()` alone already invokes
`makeKeyAndOrderFront`, which allows a non-activating panel to receive keystrokes
without process activation.

To protect against windows already off-stage when summoned, `Capture` retries once
via `bringForward` if the window list returns `ErrNoWindow`. Furthermore, the shell
captures the frontmost application context at the moment the hotkey is pressed and
passes it through `prepare` to the engine (`appInQuestion`), preventing the panel itself
from ever being mistaken for the target application.


## D-076 — A drive receipt rests on the window, not on the model's word

`runDrive` returned `Verified: true` whenever the step loop finished without an
error. That loop finishes when the model answers "done", so the receipt was the
model marking its own homework — the exact failure D-004 exists to prevent.

Verified now means every step VANS performed was followed by a change in the
window it was aimed at, read back through `Changes(before, after)`. A step that
left the window untouched makes the receipt unverified even when the model is
confident the task is complete, which is the same bar `Landed` sets for a single
app action and `string(out) == text` sets for a write.

This is evidence that the actions took effect, not proof that the task was the
right one. Nothing on screen can supply the latter, so the receipt does not
claim it.

## D-077 — A step can hold modifiers down

A step could press Enter, Escape or Tab and nothing else, so no task needing
Command-S, Command-Return, Shift-Tab or Command-F could be expressed at all —
the drive loop would work around it with clicks, or stop and say the window did
not contain what it needed.

A step now carries `modifiers` alongside `key`: named modifiers from a fixed set,
validated against it before anything is posted, and combined into the
`CGEventFlags` mask held down for that one keystroke. Named rather than free
text, because a combination the runtime cannot press has to be a refused step the
model is told about, not a keystroke that silently arrives without its modifier.

The vocabulary lives in `uiact` — `Keys`, `Modifiers`, and the two name lists the
schemas are built from — so the three copies of the key enum the tiers had
collapsed into one, and the sentence in the prompts that lists what can be
pressed is generated from the same source. A prompt that offers a key the
verifier rejects is a step thrown away for no reason.

Modifiers stay out of the page tier. A synthetic keydown with `metaKey` set does
not reach a browser's own shortcuts from inside the page, so offering them there
would promise something the tier cannot do.

## D-078 — A step nobody has done yet is still the step

The coach pointed at a control, waited 45 seconds for the window to change, and
then failed the whole run with "nothing happened". Anyone who took longer than
that to find a control lost the sequence and had to ask again from the beginning.

Waiting now returns `errStillWaiting` rather than an error, and the coach points
at the same step again — same number, same rectangle, same words. Three quiet
spells in a row is when it accepts that nobody is coming back.

Repeating costs no model call. The step already exists; the model is asked for
the next one only once the window shows the current one was done. This is the
same principle as the program cache one tier down: work already done is not done
again because time passed.

## D-079 — The application being coached holds the keyboard

The panel holds the keyboard while the question is typed, and the person is then
told to click something in another application. Two things followed from leaving
it that way: their first keystroke went to VANS, and — because macOS withdraws
the windows of an application that is not active (D-075) — the window being
coached stopped publishing controls, so the next step could not be worked out.

`RunGuide` brings the application forward before pointing at anything. The panel
keeps floating above it; it simply stops being the window with the keyboard.

The coach overlay is ordered on screen with `orderFrontRegardless` after `show()`
for the same reason: it has to appear above the active application without VANS
becoming active to get there.

## D-080a — A read does not change the world

`Capture` briefly recovered from an empty window list by bringing the application
forward and reading again. It is the wrong place for it: the coach reads the
window twice a second while waiting for someone to act, so a read that raises
would drag them back to the coached application every time they looked at
something else — and the guard that ends a guide when the window is gone would
never fire, because the read would keep resurrecting it.

Recovery moved to `windowToWorkFrom`, used by the one caller that insists on a
window: the step about to be planned against it. `RunGuide` also brings the
application forward once at the start. Everything else reads passively.

## D-080 — Naming an application costs no Automation grant

`pidFor` fell through to System Events whenever `pgrep -x` missed, which raises
the Automation prompt — the one that had been blocking coaching runs. The window
list already knows the pid of every application with something on screen, keyed
by the same owner name, and asking it needs no permission VANS does not hold.

The order is now: the process name, then the window list, then System Events.
The whole coaching path — resolve, raise, capture, read back — no longer touches
Apple Events at all.

## D-081 — The panel leaves the screen while the coach points

Coaching put a 380pt panel on top of the application it was pointing into, and
brought it forward again for every step. The steps belong on the application:
that is where the person is looking, and a window over the thing being pointed
at is the opposite of pointing at it.

The panel hides when coaching starts and comes back only when there is something
to say — the run finished, or it failed. Next and Stop are one hotkey away the
whole time, on the screen the panel is still showing, and the caption on the
overlay says so.

## D-082 — A click on the ring is the step being done

Whether the person had acted was inferred from the window changing, which is both
late and loose: System Settings ticked a value over eight seconds after a step
was pointed at, and the coach moved on from a step nobody had touched.

Two answers, in order of confidence. The shell watches for a mouse-down inside
the rectangle the coach is pointing at and tells the runtime immediately — that
is the step being done, with no inference at all. Polling remains as the fallback
for steps that are not a click, and `ActedOn` is what it asks now: the control
pointed at gained focus, changed, or went away; the window title changed; or
enough controls came and went to mean a pane answered. A single value changing
somewhere else is a clock, not a person.

## D-083 — Losing the window is not losing the run

Three failed reads in a row ended a guide with "no longer open on screen". In
practice that fires when the person glances at another application, because the
window stops publishing the moment it is not the active one (D-075) — so
coaching ended every time someone looked away.

Reads that come back empty now mean the same as reads that come back unchanged:
nothing has happened yet. The run ends on the same patience as any quiet step,
so stepping away and coming back keeps the sequence, and a window that is really
gone still stops it.

## D-084 — The first event to a window nobody is listening in is lost

The ring never appeared, through several rounds of looking at levels, collection
behaviours and ordering — none of which was the problem. The coach window is
built the moment there is something to point at, and its page takes a moment to
load. `emit_to` immediately after that reaches no listener, and the event is
dropped rather than queued. The event carrying the first step of every run was
the one being lost, so what anyone ever saw was nothing.

The page emits `coach://ready` once its listeners are attached, and the last
pointer is drawn again. It costs one redraw on the first step of a session and
nothing after that.

The general form is worth remembering: a window built on demand cannot be sent
anything until it says it is listening.

## D-085 — Nothing leaves the screen until something replaces it

The panel hid the instant coaching started, on the reasoning that the steps
belong on the application (D-081). With the first step being lost (D-084) that
left an empty screen and read as VANS having closed.

It leaves when the first step is actually on screen, which is also the honest
rule: the panel is the only thing saying anything until the coach is drawing.

## D-086 — A window in no capability is refused the event system

The ring never appeared, across rounds of work on window levels, collection
behaviours, ordering, activation and event timing. None of it was the cause.

Capabilities in Tauri v2 are granted per window, by label, and `capabilities/`
held one file listing `"windows": ["main"]`. The coach window was in none of
them, so it was refused the core event plugin outright: `listen` in the overlay
page was denied, no `coach:point` ever arrived, and the page sat there with its
dimming layer at zero opacity. From outside, a window that is permitted nothing
and a window that is drawing nothing look exactly alike.

`capabilities/coach.json` gives the coach window `core:event:default` and nothing
else. It listens for what to point at and says when it is listening; it has no
reason to reach anything further.

Two habits come out of this. A new window needs a capability before it needs
anything else. And a webview that cannot say what it is doing is a webview whose
failures are invisible — which is why `note` exists, putting both pages' lines
in the same log as the shell's.

## D-087 — What the coach draws

A ring says which control. It does not say what to do with it, and "highlight the
search field" and "click the search field" are different instructions.

The overlay draws, over whatever application is in front: the screen dimmed
except for the control, corner brackets on its edges, a pointer cursor resting
on it with a slow nudge, a click ripple under the cursor, an arrow curving from
the caption to the control, and the caption itself carrying the step number and
the instruction. Speech is deliberately not here yet.

## D-088 — The window has not moved yet when you ask where it is

The first step of a session was drawn at (-196, -133), off the screen entirely.

`draw` sized the overlay to the display, then read `outer_position` back to
translate the target rectangle from screen space into window space. Right after
`set_position`, that read still reports where the window used to be — the default
place a new window is put — so the rectangle was shifted by an origin that was no
longer true. Every step after the first was correct, because by then the window
really was where it had been put.

`cover` returns the origin it used, and the translation uses that. Nothing is
read back.

## D-089 — A refusal has to be something the model can act on

Coaching through System Settings reached "click Wallpaper" and stopped: the step
was refused twice with `nothing on screen is a AXButton named "Wallpaper"`, and
the second attempt, given that sentence, guessed a different wrong kind.

The row was on screen the whole time, as an `AXRow`. A refused step is re-asked
with the reason in hand, so the reason is a prompt — and one that says only what
is absent wastes the turn it buys. It now names what is there under that name:
"that name is on screen as AXRow". Sidebar items are the common case; the same
word is a button, a row or a static text depending on the application.

## D-090 — The overlay draws the verb, not only the place

A ring says which control. It does not say what to do with it, and "the search
field" is not an instruction — pointing at a field and typing in one are
different things being asked for.

The step already knows: it carries a verb and, for two of the three, something
the verb needs. That now reaches the overlay. A click gets the pointer cursor and
the ripple. Typing gets a caret sitting where the text will go, and the words
themselves in the caption rather than a description of them — "type your name"
leaves someone guessing, `"Vedansh"` does not. A key combination gets neither
cursor nor caret, because there is nothing on screen to press, and the caption
carries the chord.

## D-091 — Dimming draws the eye; it does not black out the application

The overlay dimmed the whole screen at 42% with a hard-edged hole cut in it,
which reads as a cutout in a dark sheet rather than as attention being drawn. The
person is still working in that application and has to be able to see it.

It is 34% now, and the hole has a radial falloff scaled to the control — a
sidebar row and a whole pane both end up sitting in a soft pool of clear screen
rather than a rectangle.

## D-092 — Finished and stopped should not look alike

The overlay vanished at the end of a run, which is exactly what it does when
someone presses Stop or when a guide gives up. Three different outcomes, one
appearance.

A finished run marks the last control with a tick and says "that is all of it"
in the caption, then takes itself away after a moment. Stop and give-up still
clear immediately, because there is nothing to celebrate.

## D-093 — Typing is done when the text is there, not when the field twitches

Coaching through System Settings asked for "type Wallpaper", the person typed,
and the coach moved on after the first keystroke — then asked for "type paper"
as a step of its own, because that was what was left.

Half-typed text changes the window, and any change was enough. Typing is the one
step whose completion is not a guess: the text is either in the window or it is
not. `ActedOn` checks for it, the same way `Landed` does one tier down, and falls
back to the window having moved on for everything else.

## D-094 — A step listed as done is done

The same run asked the person to press Enter, they pressed it, the Wallpaper
section appeared underneath — and the next step was to press Enter.

The step writer is shown what has been done, and was reading the screen
afterwards as an invitation to do it again: a search field still holding the word
typed into it looks exactly like a search field waiting to be typed into. It is
now told plainly that being listed under what has been done means it happened,
whatever the screen looks like now.

## D-095 — A guide that pointed at nothing did not finish

A run with no steps at all returned success, and the panel came back saying "that
is all of it" having shown nobody anything. It happens for a real reason: the
question was about System Settings and the window in front was something else, so
the model correctly refused.

Zero steps is now an error carrying the model's own sentence, which is the only
account of why. The panel comes forward and says it, which is what it is for.

## D-096 — Waiting three minutes on a silent model is the same as hanging

On 2026-09-22 a run sat on "working out step 1" and never moved. Nothing was
wrong with VANS: the configured model accepted the connection and never answered,
while `GET /models` on the same key came back in 0.68s and a different model
answered in 1.5s. The HTTP client allowed three minutes, and the step writer
re-asks once when an answer comes back unusable, so the wait was six.

One request now has 45 seconds, raisable with `VANS_LLM_TIMEOUT` for a model slow
enough to need it, and silence is its own failure rather than a generic one. It is
not retried — re-asking a model that never answered spends the whole wait again
with nothing new to offer — and it does not arrive at the panel as "something
went wrong inside VANS", because nothing in VANS went wrong.

Silence is told apart from the person pressing Stop by asking whether the caller's
context is still alive. Both arrive at the same line looking alike, and the
difference decides which side of the network someone goes looking at.

## D-097 — A second model, because one going down has now happened

The same measurement that produced D-096 had one model dead and another on the
same key and endpoint answering in a second and a half. `VANS_LLM_FALLBACK_MODEL`
is tried when the first model cannot answer at all: it went silent, it stayed
overloaded past the retry budget, or the provider returned 404 for that name —
the three ways a run dies with a working key and a reachable endpoint.

It is not tried for a 400. That is our own request being wrong, and every model
will reject it the same way, so spending the fallback on one only doubles the wait
before the same failure.

There is no default fallback. The right second model depends on the account and
the endpoint, and a guessed one would fail differently on every setup for no
stated reason.

## D-098 — A negative reading from the accessibility API about our own panel proves nothing

"The keyboard cannot be driven from outside" sat in the known problems for two
days and was never true. A posted Option-Space was reaching the global shortcut
the whole time; what failed was the way it was checked. The test asked the
accessibility API whether VANS was publishing a window, and §6 already records
that macOS withholds exactly that for a non-activating panel belonging to an
application which is deliberately never active. The question could not be
answered, and an unanswerable question was read as a no.

The witness is the app's own log. `[vans] hotkey fired` is printed from inside
the shortcut handler, and nothing about window visibility stands between the
keypress and that line. Both a System Events keystroke and a bare `CGEventPost`
to `kCGHIDEventTap` produce it.

The general form is worth keeping: for anything about VANS's own panel, the
accessibility API is not a valid instrument. A negative result from it is the
instrument declining to measure, not evidence of absence. This is the same trap
as D-075 from the other side.

## D-099 — Refuse where the answer is known, before paying to discover it

Asking the coach anything with Chrome in front used to raise Chrome, walk its
accessibility tree, find zero elements and report "Google Chrome has nothing on
screen to point at". Every word of that is true and all of it is misleading: it
reads as a window that happened to be empty, something that might work on the
next try. Chromium publishes no children at all, ever, and the application's own
name is enough to know it.

The refusal now happens before anything is read, carries its own error, and says
the useful thing instead: ask about the page and VANS works on it directly, which
it is good at. The general rule is the cheap half — when the name of the thing
already settles the answer, do not raise a window and walk a tree to rediscover
it.

## D-100 — An undo has to reach everywhere the thing was done

Clearing a restyle ran in the tab in front. Highlight one page, move to another,
ask for the highlighting to go, and the first page stays marked with nothing left
pointing at it — the only way back is remembering which tab it was, which is
exactly the state an undo exists to prevent.

Clearing now sweeps every tab of every window and reports how many carried a
restyle. Putting a restyle on still acts only on the page being looked at: that
request is about this page, and spreading it would be the same bug pointing the
other way.

A tab that refuses the script is skipped rather than failing the sweep. A PDF
viewer, a new tab page and a page still loading can each run nothing, and one of
those must not leave every other page highlighted. An adapter that cannot reach
past the front tab still clears that one — a partial undo beats none — and the log
says which happened.

## D-101 — A test that cannot fail is not evidence

`TestLiveDriveTask` was written to prove the drive loop against a real
application, and it could not have. Its step writer was a canned
`{Done: true}` — the model was never asked, no step was performed, and it
asserted that the receipt came back verified. It did, for the wrong reason: the
receipt's `Landed` flag means "the window moved for every step performed", which
is trivially true of no steps. The test passed the day it was written and proved
that the package compiled.

Two things came out of it. The engine no longer calls a run of nothing verified,
which is the same hole D-095 closed on the coach side and had been left open on
the drive side. And the test now asks the configured model, performs what it
answers, and reads the window back through a second capture — the receipt is the
engine's account of itself, and the window is the only independent witness.

The shape to watch for: an assertion whose truth does not depend on the behaviour
under test. `Verified == true` against a canned "done" was one. It is worth
asking of any new assertion what would have to break for it to fail, and being
suspicious when the answer is "nothing".

## D-102 — The window behind the panel is context, not the question

Asking the coach "how do I change the wallpaper" with an editor in front made it
look for wallpaper controls in the editor. It found none and said the window had
nothing to point at — true, and no use to anybody. The panel is summoned over
whatever happens to be on screen, and what is on screen is rarely where the task
is carried out: people ask how to change the wallpaper while reading mail.

Where a task belongs is now its own question, asked of the model with the things
only the machine knows — what is installed and what is open. The application in
front is passed as context and is frequently not the answer. Measured on
2026-09-23: "how do I change the wallpaper" with Antigravity IDE in front chose
System Settings; "how do I start a new chat" with Telegram in front stayed in
Telegram. Being asked the question is not a reason to send someone elsewhere.

It is a separate call rather than a slot on the router because the router decides
what kind of thing was asked from the sentence alone, and this needs the state of
the Mac. The answer is three short fields against a list of names, so it costs a
fraction of a step, and when it cannot be answered the application in front is
used — failing to improve on the old behaviour is not a reason to refuse the
request.

## D-103 — Two catalogues, because "can be scripted" is not "exists"

`Catalog.Find` searched only applications publishing a scripting dictionary, which
is right for the tier that drives them through Apple Events and wrong for deciding
where a task belongs. System Settings declares almost nothing, and most of what
people use declares nothing at all, so resolving a chosen name through `Find`
rejected the correct answer as unknown and fell back to whatever was in front.
Telegram did exactly that and got the right answer by luck.

`Installed` and `FindInstalled` are the wider pair. The filter that belongs to one
question was doing damage in another, and the fix is two named questions rather
than one list that means different things to different callers.

## D-104 — A test that consults the machine is not hermetic, however it looks

`TestRunScreenExecutesClickOnTargetControl` stubbed the front-window owner, the
process id through a fake runner, the walker and the click poster, and still
failed — because `pidFor` tries `pgrep` first, so on any Mac with Telegram open it
got the real Telegram's process id instead of the invented one. It then refused
the click with "Telegram's window, which is in front of Telegram", which is the
guard being correct about two different processes and reading like nonsense.

The test passed or failed by what the developer happened to have running. The
process lookup is now indirect like the other machine-facing calls in the package,
and the tests that invent a process id say so.

Worth generalising: this package fakes the accessibility API carefully and had one
un-faked path left. A test that looks fully stubbed can still have exactly one
line reaching the real machine, and that line decides the result.

## D-105 — The step that VANS will not carry out is still a step

The task belongs to an application that is not running. VANS does not open
applications (D-032), which left two bad options: refuse, or tell the person to
open it and ask again. The second is what shipped first and it breaks the one
thing the coach is for — not having to work out the next move yourself. Being sent
away to re-ask is working it out yourself.

Opening it is now step one. It is drawn like any other step, the person carries it
out, and the coach waits for the application to appear and picks up inside it. The
constraint is unchanged: VANS still launches nothing. What changed is that the
person doing it is treated as part of the flow rather than as the end of it.

Readiness is the window publishing controls, not the process existing. An
application that has launched but drawn nothing cannot be read, and planning a
step against it fails for a reason nobody could act on. Ninety seconds, three
times: finding an application and waiting for it to launch is slower than finding
a button in a window that is already open.

## D-106 — A step with nothing to point at is drawn as an instruction

"Open System Settings" has no control, no rectangle, and nothing to ring. The
overlay draws rings, brackets, a cursor and an arrow from a target rectangle, and
a zero-sized one makes all of that collapse into a dot in a corner — worse than
drawing none of it.

A zero-sized target is now read as an instruction: everything that marks a control
is taken off, nothing is cut out of the dimming, and the sentence goes in the
middle of the screen. The whole screen going quiet behind one line is the right
weight for a step that is about the Mac rather than about a control.

Nothing is clicked to advance it either, which falls out for free: the click
watcher only counts clicks inside the ring, and there is no ring.

## D-107 — A control is addressed by the name it shows, wherever that name lives

Two live runs died asking for "Wallpaper" while the word sat in the System
Settings sidebar the whole time. The sidebar items have no title at all: each is
an `AXStaticText` with an empty title and the value `"Wallpaper"`, `"Displays"`,
`"Sound"`. Matching looked only at the title, so nothing matched. The model was
shown only `AXStaticText (holds text)`, so it could not see the name either and
was guessing it from knowing macOS. And `nearMiss` looked at titles too, so the
refusal that exists to correct exactly this (D-089) came back empty and the model
repeated its wrong answer until the run stopped.

An element's label is now its title, or its value when the title is empty and the
role is one whose value names the control. Short values only, on labelling roles
only — the reason values are withheld has not gone away, and a message is not a
label. Forty characters and no newlines is the line: a sidebar item is a word or
two.

With it, the run works and the correction loop finally does what it was built for:
`nothing on screen is a AXRow named "Wallpaper" — that name is on screen as
AXStaticText`, and the next answer is a step that lands.

The general lesson is about the withholding rule rather than about System
Settings. A conservative default — never show a value — was right for the case it
was written for and quietly made whole applications unreachable. Worth asking of
any blanket rule which things it is silently deleting.

## D-108 — The overlay is the same application as the panel and has to look like it

The coach drew its step on a dark card with its own hard-coded greys and blues
while the panel was a light frosted surface. Two windows of one application that
do not share a palette read as two applications, and on a light desktop the dark
card sat on the content rather than above it.

The overlay now carries the panel's tokens and reads the same stored choice, which
Tauri shares between windows of one origin — no event to plumb and no second place
for the answer to live. It is read on every step, because the overlay outlives any
number of changes made in the panel behind it.

One number is deliberately different. The panel is 0.82 opaque and gets the rest
of the way from an AppKit material behind it; the overlay window has no material,
so the same surface at the same value would leave text sitting on whatever
application is underneath. It carries 0.97 and reads as the same material.

## D-109 — A window finishing its own work is not somebody using it

The coach decides a step is done by comparing the window before and after, and
when nothing about the target itself changed it falls back to counting controls
that came or went. Two controls is enough. An application that is still drawing
clears that bar without anybody touching anything, so the coach walked through
steps nobody had done and then reported the task complete.

Folding the window's own settling into the baseline — whatever it does to itself
in the first moment is what it looked like to begin with — fixes the case where an
application has just been raised. It is not enough on its own, and the measurement
says why: System Settings' Wallpaper pane loads thumbnails for several seconds,
which no reasonable settling window covers.

The rest of the fix is to stop inferring at all. The shell already watches for the
real event: a global mouse-down monitor that fires when a click lands inside the
ring, already wired to the channel the coach waits on. The coach has a direct
observation of the thing it is trying to detect and is overriding it with a guess
about window churn. The guess belongs to the drive loop, where VANS itself acted
and any change is evidence; the coach is watching a person and should require
either the observed click or evidence about the control.

Recorded rather than finished: the settling half is in, the rest is the next
thing. The general shape is worth keeping in mind — when a direct signal for
something exists, an inference about it is not a fallback, it is a competing
answer that will sometimes win and be wrong.

## D-110 — Where a direct signal exists, an inference is a competing answer

The coach decided somebody had acted by comparing the window before and after,
falling back to counting controls that came or went. Two was enough. An
application still drawing clears that without anybody touching anything, so the
coach walked from step 1 to step 3 of a task nobody had started, then reported it
complete.

The fix is not a better threshold. The shell already watches for the real event —
a global mouse-down monitor that fires when a click lands inside the ring, wired
to the channel the coach waits on. The coach had a direct observation of the thing
it was trying to detect and was overriding it with a guess.

`ActedOn` and `PersonActedOn` are now two tests for two questions. The drive loop
performed the step itself, so any change in the window is evidence it landed. The
coach is watching somebody else and requires the observed click, or evidence about
the control itself: it went, it took focus, what it holds changed, or the window
retitled. A key press keeps the looser test, because there is no click to observe
and pressing a key need not touch the control at all.

The general shape: when something can be observed directly, an inference about it
is not a safety net. It is a second answer that will sometimes win and be wrong,
and it wins exactly when the world is busiest.
## D-111 — "Something went wrong inside VANS" is a claim, and usually a false one

A coached run that ends short arrives at the panel as a failure, and the panel's
fallback heading for a failure it has no view for is "That didn't work —
something went wrong inside VANS". The messages underneath were accurate and
already written for a person: `no part of the AXButton "Tahoe" is visible on
screen`, `nothing happened in System Settings, so the steps stopped`. Nothing had
gone wrong inside VANS in any of those cases. The window was not what the step
needed, which is a different thing and points somewhere else entirely.

The ways a coached run can stop now carry a sentinel, and the panel has a view for
them: it names the application and shows the engine's own sentence as the
explanation rather than hiding it behind a disclosure triangle. The generic
heading stays for the thing it is actually about — a build missing a piece, a
fault with no better account of itself.

The engine's `engine: ` prefix is stripped on the way out. Naming yourself at the
front of every line is right in a log and is noise in a sentence being read by the
person it happened to.
## D-112 — Being covered is not a question for the model

The first live drive run had TextEdit publishing 44 controls while sitting behind
DBeaver. It was readable, so it was never raised, and the click was refused:
`(508,379) is inside DBeaver Community's window, which is in front of TextEdit`.
That refusal then went back to the model as a reason to write a different step —
and the model, having no way to see or move a window, answered by avoiding the
click altogether.

Nothing about the step was wrong. The recovery for a covered window is to bring it
forward, which is a thing VANS can do and the model cannot. A refusal that is only
about occlusion now raises the window and retries the same step, with no second
ask: the answer would be the same and it was never the problem.

Once per step. A raise that did not take will not take on a third go, and
`windowToWorkFrom` only ever raised when the window published nothing at all —
readable-but-buried fell straight through the gap between those two.

This is the sharper form of D-080a. Reading stays passive; the callers that mean
to change the world say so. A caller about to click is one of them, and it had
been left out.

## D-113 — A ledger, because "what has this done to my machine" had no answer

Every table in the store was a cache. Routes, programs, window actions,
instructions: losing any of them costs a model call and nothing else. There was
no record of what VANS had actually changed, which meant the question a person is
most entitled to ask about a program acting on their behalf — what have you done —
could not be answered at all.

One row per committed change: what was asked, which tier acted, where, the
sentence the person agreed to, what the window showed afterwards, whether it was
seen to happen, and how to put it back.

Three things decided here and worth keeping.

**It records from `Commit` and nowhere else.** Every side effect passes through
that one function, so the record cannot be incomplete without the code being
wrong in a way that is obvious. A recorder attached per tier would drift the first
time a tier was added.

**It records failures too.** A change that did not finish is the one somebody most
needs to go and look at, and silence about it is the worst answer. The row says
so in the same place as the rest.

**Writing it down never fails the change.** The world has already moved by the
time this runs, and reporting a change as failed because the reporting failed
would be the worst of both. A build with no store acts exactly as it did before
and remembers nothing, which is honest.

The stored sentence is the preview the person confirmed, not a description
written afterwards to fit what happened. What they agreed to is the thing worth
keeping.

Two hundred rows, then the oldest go. This exists to make VANS accountable and
reversible, not to keep a record of everywhere somebody has been.

## D-114 — Say both what can be put back and what cannot

The ledger carries how to undo each change, and for almost everything today that
is empty. Typing, clicking, a dictionary write and a multi-step run are all
one-way. A restyle is the one tier with a real inverse, and it is a complete one.

Empty is recorded and shown as "cannot be undone" rather than left blank. A blank
invites the reader to assume the generous answer, and the generous answer is wrong
for most of what VANS does. Saying it plainly is what makes the next step
possible: once the gate can tell the two apart, it can stop being equally alarming
about both, and only be frightening about the ones that deserve it.

No tier claims an inverse it does not have. A hopeful guess here is worse than the
gap, because it would be believed.

## D-115 — Undoing asks nobody's permission

Every other thing VANS does that touches the world goes through a confirmation.
Putting one of them back does not, and the reasoning is the same one that decided
clearing a restyle needs no gate (D-100): undoing is not a side effect anyone
needs protecting from. A gate in front of it would be asking somebody to confirm
that they meant the thing they just asked for, and every confirmation that guards
nothing teaches people to dismiss the ones that do.

## D-116 — Mark it undone only once it is back

The obvious shape is to mark the change off and then reverse it. It is wrong in
the way that matters: a change recorded as undone while it is still on screen is
not a missing answer, it is a confident wrong one, to the exact question the
ledger exists to answer.

So the inverse runs first and the row is marked only if it succeeded. If the
marking then fails, the log says so and the change stands as still-undoable —
offering to put something back twice is harmless where claiming it is already back
is not.

The general form: when a record and the world can disagree, order the writes so
the record is the thing that lags.

## D-117 — A drive trace is keyed by the shape, not by the window

The obvious key for a cached run is everything that could make it wrong: the
application, the instruction shape, and a signature of the window it started from.
The signature is the one to leave out.

A drive run exists precisely because the window has state that changes — a
document with different text in it, a list with a different number of rows, a
different unread count. Fold that into the key and almost every lookup misses, and
the cache that was supposed to take a six-step task down to zero calls takes it
down to six calls with an extra table.

What the signature was meant to protect against is already covered, and covered
better, by checking each step against the window as it actually is at the moment
that step runs. A key can only say *this looks like the same situation*. Validation
says *this step fits what is in front of me*, which is the question that matters,
and it asks it once per step rather than once per run.

So: `(app, shape)`, and per-step validation as the guard. The failure mode
degrades instead of falling off a cliff — a trace that stops fitting costs model
calls only from the step where it stopped fitting.

## D-118 — A trace that cannot be templated is pinned, not discarded

A cached run is only reusable if the values from the instruction have been lifted
back out into their placeholders, so that `Fill` can rebuild it for new ones.
Mostly they lift out cleanly: a typing step's text is usually the literal exactly,
and a whole field can be swapped at any length.

It is substrings that are dangerous. `uiact.Baked` already refuses to treat
anything under three characters as a literal to look for inside a longer string,
because replacing every "a" or "10" it finds would corrupt the step. So a trace
with a short literal buried inside other text cannot be templated safely.

The first instinct is to refuse to cache it. But there is something better
available for nothing: keep the trace and pin it to the literal values it was
recorded with, so it answers only for those. Asking for the identical thing a
second time — which people do constantly — is then still free, and a request with
different values misses cleanly and asks the model, which is what refusing to cache
would have done for every request including the identical one.

The rule underneath: when a cache cannot be made general, make it narrow before
making it absent. What must never happen is a trace replayed for values it was not
recorded with, because a step that types yesterday's text is wrong in a way no
check downstream would catch.

## D-119 — Typed text is judged harder than a selector

A trace is generalised by lifting the instruction's values back out into
placeholders. The question is what to do about whatever is left over, and the
answer turns out to be different for a control's name than for text that gets
typed in.

A stale selector is safe. If a trace says click the row named "Tab 2" and there is
no such row, validation refuses the step before anything happens, the run resumes
live from there, and the worst case is some model calls. The guard is already
built and it fires on exactly this.

Stale text is not safe, because nothing downstream can tell. A step that types
last week's sentence into a field passes validation — the field is there, it is
typeable, the text is non-empty — and lands, and changes the window, and reports
success. Every check VANS has says yes. The words are simply wrong.

So every word a step types has to be traceable to the request. Three things count:
a placeholder, a value pinned in place, and a word of the instruction that was
never a value at all.

That third one is the common case, and the first version of this refused it —
which would have refused almost everything. `Shape` lifts URLs, quoted strings and
numbers and nothing else, so "type hello into the document" carries no literals:
"hello" is part of the shape. Baking it in is not a risk, it is exact, and the
reason is worth stating because it is the load-bearing claim of the whole cache —
two instructions with the same shape differ *only* in their literal values, so any
part of the instruction that is not a literal is identical for every request that
can ever hit that key.

What is left after those three is a word the model supplied itself, and that is
what makes a run unkeepable. Composed prose is the obvious case. Text copied off
the window is the less obvious one and it is refused just as firmly: the window's
contents are precisely what will have changed by the time the trace is replayed.

One bounded imprecision, stated rather than hidden: the shape is lowercased, so
"Type Hello" and "type hello" share a key and the stored trace types whichever
case the first run used. This is inherent to caching by shape and is already true
of the single-step window-action cache. It is a wrong capital letter, not wrong
words.

The general form: how strictly a cached thing must be checked depends on whether
being wrong about it is detectable later, not on how likely it is to be wrong.

## D-120 — Learning from a run is not part of carrying it out

Writing the trace happens after the task has been done. If it fails — the disk is
full, the value would not encode — the change in the world has already happened,
and returning an error would report a change that was made as a failure.

So it logs and carries on, the same rule the ledger follows for the same reason
(D-114). The cost of losing a trace is one model call next time. The cost of
reporting a successful change as failed is that somebody does it again by hand.

## D-121 — Replay is a mode of the live loop, not a second loop

A replayed step has to be captured against a fresh window, validated against that
window, raised above anything covering it, and read back afterwards to confirm it
moved something. That is the entire live loop. Writing a separate replay loop means
writing all of it twice and then keeping the two the same forever — and the day
they drift is the day a cached run stops being held to the standard a live one is.

So there is one loop, and the trace is only a different answer to "what is the next
step". Where the model would be asked, the trace is read instead; everything after
that point is identical code. Divergence switches the source mid-loop rather than
returning to a caller, which is also what makes resuming free: the state the live
loop needs is already in the local variables.

The general form: when two paths must be held to the same standard, share the code
that enforces it rather than the intention to keep them alike.

## D-122 — A trace's length is the answer to "is it finished"

A live run spends one model call at the end purely to hear that the task is done.
A warm replay must not, or the run that is supposed to cost nothing costs one call
— which is most of the saving on a short task.

It does not need to. A trace is only stored for a run the model itself called
finished, so the sequence already encodes where the task ends. Reaching the end of
the trace *is* the model's answer, recorded earlier.

What makes this safe rather than a shortcut is that finishing was never the
evidence. `Verified` comes from every step having been seen to change the window,
and that check runs on replayed steps exactly as on live ones. The model's word
decides when to stop; the window decides whether it worked.

## D-123 — Count the divergence before deciding the run was empty

`runDrive` returns early when a run performed no steps, and the trace bookkeeping
sat after that return. So a trace that went off on its very first step produced a
run that did nothing, took the early exit, and was never recorded as having gone
off — which is exactly the trace that most needed recording. A sequence that
reliably broke would have been retried forever and never forgotten.

Found by a test asserting the divergence count, not by reading the code. The
ordering looked right: handle the empty case first, then the bookkeeping. The bug
is that "nothing happened" and "nothing was learned" are different claims, and the
first does not imply the second.

## D-124 — Absent and zero are different answers, so the counts are an object

A run that stepped through a task reports how many steps came from what was kept
last time and how many times the model was asked. The obvious way to carry that is
three integers on the receipt, left at zero where they do not apply.

It does not work, because the most interesting number here is a zero. A repeated
task asking the model nothing is the entire claim of the cache; `omitempty` drops
exactly that field on exactly that run, and a plain zero is indistinguishable from
a note written to Notes, which asked nothing through this path because it has no
such path.

So the counts live in their own object, absent for every tier that does not step
through anything. Absent means "not a question about this kind of work"; present
and zero means "asked nothing", which is the good news.

The general form: a field whose zero value is meaningful cannot share a
representation with a field that is merely unset.

## D-125 — The saving is shown, not claimed

The panel now says "all 3 steps from last time, nothing asked" or "3 steps, 4
model calls" alongside the result.

This is here because a performance claim that a person cannot check is a claim
they have to take on trust, and this one is easy to get wrong in a way nobody would
notice: a trace that silently stops matching costs the model calls it was supposed
to save, and the only visible difference is that things feel slower. Printing the
count turns that from a suspicion into an observation.

A run that reused nothing reports its calls too. Knowing the first run cost four
calls is what gives the second one's zero any meaning.

## D-126 — The live measurement gets a store of its own

The test that proves a repeated task is free runs the task twice and asserts the
second run asked the model nothing. Pointed at the real database, the first run
would only be the first run the very first time anybody ran it — after that the
trace is already there, the "cold" run is warm, and the test passes while proving
the opposite of what it says.

So it opens a store in a temporary directory. The first run is the first run every
time, and the test asserts that too: a cold run that made no model calls fails,
because that means the store was not empty and the comparison is meaningless.

The general form: a test about a cache has to control the cache's starting state,
or it is a test about what happened to be lying around.

## D-127 — Assert on the calls, and the clock will follow

Written first as "read the calls, not the clock", on the reasoning that a warm run
still reads the window before every step, validates against it, and reads it back
afterwards — so those reads would dominate a short task and the seconds would
barely move. The advice was to quote the call count, because "we built a cache and
saved 300ms" reads as a failure where "four model calls down to none" is the same
run described by what changed.

The measurement contradicted the prediction. TextEdit, one step: 6,052 ms cold
against 866 ms warm. Reading and acting on a window takes well under a second; a
model call takes about two. So the calls were most of the wall time even on the
shortest task there is.

The conclusion survives with a different reason. Assert on the call count, because
it is exact and does not move with the network — a test that asserts on seconds
fails on a slow morning. But do not apologise for the clock in advance. It moved
seven times.

Kept rather than rewritten, because the wrong prediction is the useful part: the
cost model here was guessed from which layer *felt* slow, and the layer that felt
slow was the one that had been optimised already (D-057 moved window reads to cgo
and took System Settings from 23.9s to 132ms). The remaining cost was the network,
which is where it usually is.

## D-128 — Waiting for a window is not waiting for it to be in front

Bringing an application forward ended with `waitForWindow`, which polls whether
that application has a window on screen. That is the right question when a window
is hidden, and the wrong one after a raise: a window with something on top of it is
on screen. So the wait returned the instant it was asked, the step was retried
while the raise was still happening, and it was refused a second time for being
covered — by a window that was on its way out of the way. Measured on 2026-09-26:
one wasted model call on a one-step task.

Two conditions that read as the same thing and are not. "Is it there" and "is it in
front" both sound like "can I use it".

The wait for frontmost is best effort and deliberately not an error. Whether the
target is actually reachable is settled by `VerifyTargets` against the window
itself, which was always the real guard; this only gives the raise time to happen.
An application whose window-list name differs from the name asked for would never
satisfy the check, and failing a step over that would be worse than trying it.

## D-129 — Correct the step when the window already says what it should be

The model asks for the `AXButton` "Wallpaper"; System Settings has an `AXRow` by
that name. The refusal says so, because `nearMiss` works the right role out in
order to word the message — and then the step was re-asked and came back identical.
Twice, live, and then the run stopped. Two model calls to arrive nowhere with the
answer in hand the whole time.

So it is corrected in Go. The condition is that there is nothing to decide: the name
matches exactly one control on screen and only its role is wrong. Several matches is
a question about which one was meant, and guessing that is how something gets
clicked that nobody asked for — that still goes to the model.

What makes this safe is that a corrected step is trusted no further than one the
model wrote. It goes through `Validate`, through `VerifyTargets` against the live
window, and through the read-back afterwards. If the correction does not fit either,
the model gets asked, because then the window was never the problem.

The general form: when the code can already compute the answer it is about to pay
to ask for, the question is the bug.

## D-130 — A refusal is written for the answer, not for the record

With the unambiguous refusals corrected in Go (D-129), what still reaches the model
is a name that matches several controls. That message used to describe the problem —
"4 elements are a AXRow named Wallpaper, so it does not say which one" — which is
accurate and leaves the reader to work out what to send instead.

It now says how many of each there are and that an index is wanted. The audience for
an error here is something composing its next answer, so the message is worth
writing as an instruction rather than as a report. The version that only described
the problem was answered identically twice.

## D-131 — Nothing here opens an application, including the fallback

Bringing an application to the front tried accessibility first and fell back to
System Events, on the reasoning that accessibility is already granted and Apple
Events costs a second permission — so the fallback was there for the rare case
accessibility could not do it.

The fallback ran `Application(x).activate()`, and that **launches an application
that is not running**. So the one rule this runtime has had from the start — it
attaches to what is already open and never starts anything (D-032) — was broken by
its own error path. Seen live: a coached run opened System Settings rather than
saying to open it.

Worse, the fallback was reached exactly when the accessibility raise had failed,
which is exactly when the application is least likely to be in the state the caller
assumed. The path most likely to launch something was the path taken when the
assumptions were already wrong.

So the fallback is gone and the script with it. Setting `kAXFrontmostAttribute`
raises a running application or fails, and it cannot launch anything. A failure is
now a `CannotRaiseError` naming the application, which the panel shows under the
code it already has for an application that is not open — because it is the same
thing to say: here is the one, and you open it.

The general form: a rule that the happy path keeps and the error path breaks is not
a rule. The error path is where it matters, because that is where the assumptions
have already failed.

## D-132 — The hotkey means what is in front of you

⌥Space summons the panel. During a coached run the caption beside the ring also
says ⌥Space is how to move to the next step. Both were true of the same key, and
what it actually did was toggle the panel — which still held the step it had
already shown. So pressing it put a second copy of the step on screen, with its
own Next and Stop. One step, two cards, and the caption was lying.

The fix is not a setting or a modifier. While the coach is on screen the hotkey
advances the step; while it is not, the hotkey summons the panel. The mode is
decided by what the person is looking at, which is a thing they can see, rather
than by state they would have to remember.

That only works because the coach now takes itself off the screen when they go
somewhere else (D-133). The two changes are one idea: the coach is on screen
exactly when it is relevant, and the hotkey means the obvious thing in each case.

## D-133 — The coach joins every Space, and hides itself when you leave

The coach window is set to join all Spaces. It has to be: it draws over somebody
else's window wherever that window is, including a full-screen application, and a
window belongs to the Space it was created on.

The same flag meant that swiping to another desktop took the ring along. A pointer
circling empty space on a desktop with nothing to do with the task.

So the flag stays and the window is hidden instead, by watching which application
is in front and taking the ring off the screen while that is not the one being
coached. Asked four times a second through AppKit, which needs no permission.

The step is kept, not cancelled. The engine already waits through exactly this —
a window that publishes nothing is usually somebody having stepped away (D-075),
and the wait is deliberately patient about it — so coming back redraws the step
that was already there rather than starting the run again.

Two deliberate softenings, both erring towards leaving it up: an unreadable
frontmost application is not treated as "somewhere else", because flickering the
ring off every time the question cannot be answered is worse than leaving it on a
maybe; and the panel is untouched, because being summonable from any Space is the
thing that flag is there for.

## D-134 — The pointer belongs to the person sitting there

A drive step clicked by posting a mouse event at the control's centre, which moves
the actual cursor across the actual screen. It works, and it is the single thing
that makes a runtime feel like it has taken the chair rather than pulled one up.

It is also not necessary. A control that publishes `AXPress` can be pressed where
it stands — no pointer involved, on a window that is not even in front. Typing
wants the field to hold the keyboard, and a click is only one way of arranging
that; asking for it directly is the other, and it moves nothing.

So accessibility is the way a step reaches its control, and the mouse is the
fallback for controls that publish nothing of their own.

**Asked before pressing, not inferred from failing.** `AXPress` can return success
on a control that quietly ignores it, which would leave a step that never happened
looking like one that did. So the action list is read first and the press is only
used where it is published. That is one extra accessibility call per step, against
reading action names for all 250 elements of every capture — the question is only
ever about the one control being acted on.

An accessibility error is not a reason to skip the step: the mouse still reaches
it, and the window is read back afterwards either way (D-004), so nothing about how
a step was performed changes what counts as having worked.

This is also what makes the ghost cursor honest rather than decorative
(`docs/specs/2026-09-26-ghost-cursor-design.md`). A drawn pointer next to a real
one that jumps is a cartoon. With the real pointer left alone, the drawn one is the
only pointer in the room, and it can show exactly where a mouse would have gone.

## D-135 — A page action goes where the person is looking

The router decides what kind of thing is being asked from the words alone, and
"type hello" reads the same whether the person is looking at a web page or at
TextEdit. When it chose "a page", the page tier acted on whatever tab the browser
had open. Seen live: TextEdit in front, and VANS asked a model how to type into a
LinkedIn profile nobody was looking at, then reported that it could not.

So a page action is redirected to the application in front when that application
is not a browser. Only the one request is redirected. The stored routing decision
stays a page action, which is the right answer the next time a browser is what is
in front — the words did not change, only what was in front of them.

## D-136 — The coach waits for the window to answer the click

The shell reports a click on the pointed-at control the instant it happens (D-109),
which was meant to make the coach feel immediate. It made it worse. The instant a
click happens is before the application has done anything about it, so the window
read at that moment was unchanged, the step was written down as "clicked Displays
— nothing in the window changed", and the model, told the click had done nothing,
reasonably asked for it again. Every coached run showed its first step twice.

The fix is to wait, after the click, for the window to move and then to finish
moving — up to a second and a half. A click that genuinely changes nothing runs
out the wait and is recorded as having changed nothing, which is then the truth.

The general form: an event saying something happened is not the same as the world
showing it, and a record of what happened has to be read from the world.

## D-137 — Next is a claim, and it is checked

On the step that asks someone to open an application, pressing Next was taken as
"it is open" and the run carried on — into an application that was not running,
where it stopped with an error about the application instead of the step that
asked for it. Seen live, with ⌥Space pressed before System Settings was opened.

Next now makes the coach look. If the application is open the run carries on; if it
is not, the step stays where it is. Pressing Next early costs nothing and cannot
skip anything.

## D-138 — The ghost cursor is not shaped like a cursor

The first version was a mouse arrow, drawn larger and shadowed. It read as the
person's own pointer having jumped somewhere — which is the one thing the ghost
must never be mistaken for, since the whole design turns on VANS not touching the
real pointer (D-134).

So it is a soft, rounded triangle in the accent colour with a glow of the same
colour — the shape Clicky uses, and for the same reason. It turns to face the way
it is travelling, so the tip leads round the curve and lands on the control first.
It carries on from wherever the last step left it, because a hand that teleports
between steps is not a hand.

## D-139 — Plan the whole task once, and check it a step at a time

Every step used to be its own model call, made after the step before it had
happened. On a drive run that is a call per step plus one to hear it is finished;
in the coach it is a pause of about two seconds before every step, because the
next one is not worked out until the person has done this one.

So a run with nothing remembered for it asks for the whole sequence in one call,
up front. The steps after the first are the model's expectation of windows it has
not seen, and they are treated exactly as a remembered step is (D-121): checked
against the real window when their turn comes, and the run worked out live from
the first one that no longer fits. A plan that is right about the first three steps
and wrong about the fourth still saved three calls.

The one difference from a remembered run is what running out means. A stored trace
was only kept for a run the model itself called finished, so reaching its end is
that answer (D-122). A plan written for this run promised nothing of the kind — the
model saw one window when it wrote it and there is a different one now — so when
it runs out the model is asked once whether that was all of it. That is the call
this does not save, deliberately: skipping it would mean a run that stops a step
short reports itself finished.

A plan is never a reason to stop. No planner, a failed call or an empty plan all
leave the run working itself out a step at a time, which is what it did before.

## D-140 — A step is shown before it is made, and the showing is the veto

A run that acts on screen used to reach its controls with nothing to say it was
coming — a field filled, a button pressed, and the person left to work out
afterwards what had happened. Each step is now announced just before it is
performed: the ghost cursor glides to the control and lands, and the step follows
about 850 ms later.

That pause is doing two jobs. It makes the run watchable, which is most of what
makes an agent acting on your own machine tolerable. And it is a veto: Escape
during it stops the run with that step not done. The confirmation gate protects
the plan as a whole, before anything starts; this protects each step, at the
moment it is about to happen, and costs nothing unless it is used.

The length is a dial rather than a detail — long enough for the ghost to arrive
and be seen, short enough that a six-step run does not feel staged — so it is a
named value rather than a number in the code. A step with nothing to draw is
performed without the pause, since waiting to show nothing is only slower, and a
build with no shell watching has no pause at all.

## D-141 — The coach listens for the window changing, and judges it once it is quiet

Between a click and the next step the coach used to spend about a second doing
nothing useful: it re-read the whole window on a timer to see whether the click had
landed, then waited a fixed 900 ms for the window to finish moving before it would
trust what it saw. Most windows finish moving in a fraction of that.

Two changes. The coach asks macOS to tell it when the coached application's window
changes (an accessibility observer on the application), so it hears the click land
instead of finding out a poll later. And "settled" now means the window has been
quiet for 250 ms, not that 900 ms have passed; 900 ms stays as the most it will
wait. Without the observer — a build without the accessibility API, or an
application that refuses it — the same quiet rule runs on re-reads every 150 ms.

A change heard is only believed if the window really differs from before the click.
A notification left over from before the click is not evidence of anything, and
taking it as one would bring back the step-shown-twice bug D-136 fixed.

The window read after a step is also the one the next step is checked against,
rather than reading it again straight away.

## D-142 — The coach keeps the steps of a question it answered, under its own key

A drive run that worked is kept and replayed for free (D-117). The coach walks the
same loop and can keep its runs the same way: the second time somebody asks how to
change the wallpaper, the steps come from last time and the model is not asked.

Kept under `coach: <shape>`, never under the bare shape. A coached question and a
drive request can reduce to the same shape, and a drive trace is VANS's clicks while
a coach trace is the person's, which is not the same claim about the window.

Every rule for keeping a drive run applies unchanged — the model called it
finished, every step changed the window, and nothing typed came from the model. A
coached step the person skipped with Next and did not do changes nothing, so a run
like that is never kept.

## D-143 — A plan that goes wrong is planned again, twice at most

D-139 planned the whole task once and, from the first step that did not fit, went
back to one call per step. The plan prompt already told the model that anything not
fitting "is planned again from that point", which was not true.

It is now. When a planned step does not fit, the rest is planned in one call with
the window in view, the steps already done and the reason the step did not fit. A
plan wrong at step two of six now costs one call to recover, not four.

Bounded at two re-plans a run and one per step. A task the model cannot predict at
all falls back to working it out a step at a time, as before.

## D-144 — A control out of view is scrolled to, not pointed at blind

The live coach test pointed at "Big Sur" in a list where it had scrolled out of
sight. The window lists every control whether it is on screen or not, and nothing
told the model which ones the person could see. The coach drew a ring round a
rectangle below the bottom of the window.

Three parts:

- The list the model reads marks such a control `[out of view]`, and both prompts
  say to prefer one in view that does the same job, and otherwise to name it
  anyway. Scrolling is never a step the model writes; it is handled underneath.
- A drive run scrolls it into view through accessibility (`AXScrollToVisible`)
  before the ghost is sent there. The pointer does not move, the same as a press
  (D-134). If the application does not support that action, the step fails as it
  did before, with the reason in the log.
- The coach does not scroll for the person, because in the coach the hand is
  theirs. It rings the list the control is in, says which way to scroll, and
  waits until the control is in view. Then it points at it as the same step.

Each step now reports the window as it stood just before the step was carried out.
Without that, the scroll itself counted as the step's effect, and a click that did
nothing would have been verified by the list having moved.

## D-145 — The ghost carries a caption, and the steps done leave a trail

Two additions to the overlay, both drawn from where the ghost already is.

**A caption rides with the ghost.** A short pill beside it that follows it through
the glide. In a drive run it is the step itself ("Type your Mac's name"), with
"Esc to stop" beside it, and it replaces the tip card: the person is watching, not
acting, so a card telling them what to do was the wrong message, and its hint
("⌥Space for Next or Stop") was wrong for a drive run anyway. In the coach the card
stays, because it is the instruction, and the caption is only the verb ("Click
here", "Type “Studio”", "Scroll down ↓"). It sits on the side of the control
away from the card so the two never overlap.

**A trail of the steps done.** Each finished step leaves a small numbered dot where
it happened, joined to the next by a dotted line that ends at the current step. It
shows where a run has been without anyone reading a list, and it makes a step that
went back to an earlier control visible as a line doubling back. The last eight are
kept. Points are held in screen coordinates and drawn only on the display they
belong to, since the overlay covers one display at a time. A lower step number than
the last starts a new trail, and dismissing the overlay clears it.

## D-146 — Touching the mouse pauses a run VANS is doing

A drive run acts through accessibility and leaves the pointer alone (D-134), so
the person's hand on the mouse is free during a run — and a hand moving is the
clearest sign they want to do something themselves. Carrying on regardless means
VANS and the person acting on the same window at once.

So before each step is carried out, the pointer's position is compared with where
it was after VANS's own last action. Moved by more than a few points, the step
waits: the overlay says "Paused", with ⌥Space to carry on and Escape to stop.
Carrying on reads the window again and checks the step against it before doing
anything, because the person may have changed what the step was aimed at. A step
that no longer fits goes through the usual path — re-planned, or worked out again —
as if it had been refused.

The baseline is taken again after every action of VANS's own. That is what stops
the mouse fallback, which does move the pointer, from pausing the run it belongs
to. A pause nobody answers stops the run after two minutes, with nothing done.

⌥Space now sends its own signal ("next") rather than the one a click inside the
ring sends. During a coached run the two mean the same thing; during a drive run a
click on the ring is the person touching things, and it must not count as carrying
on.

Only the pointer is watched. Typing is not, because VANS's own typing is
indistinguishable from the person's at the level this reads.

## D-147 — A web page is read as a window, so the step loop needs nothing new

Multi-step work existed only for Mac applications, because a browser publishes no
controls to the accessibility API (§8). Everything that makes those runs good — a
plan checked a step at a time, re-planning, kept traces, the coach, the ghost, the
pause — sat behind one interface, `uiact.Screen`, and none of it knows what is
behind that interface.

So a page gets its own `Screen` (`internal/webscreen`) rather than its own loop.
One script in the page lists what a person could act on — links, buttons, fields,
lists, anything with a role, inside open shadow roots too — with its name, what it
holds and where it is on screen. That becomes a `uiact.Tree`, with roles spelled
the way the accessibility API spells them so the same prompts and the same
validation apply. Acting sends click, type or a key to the element by its place in
that list; an element that has left the page since is a refusal, which the loop
already knows how to recover from.

Three details that matter:

- The tree is named for the **site**, not the browser. Kept steps are keyed by
  application, and steps learned on one site must never be replayed on another.
  The browser's name is carried beside it, for raising the window and for the
  overlay, which watches which application is in front.
- The page area is the first element, and every control sits inside it. That is
  what makes a control below the fold "out of view" by the existing rule (D-144),
  with no web-specific code.
- A read waits for the tab to stop loading. A click that navigates is a step that
  landed, and reading halfway through the load would compare against half a page.

A page task may take 24 steps where a window task takes 12; booking anything is
longer than changing a setting.

The single-shot page program stays as the fallback when no page screen is
configured or the page cannot be read.

Limits, stated rather than discovered: input goes through the page's script, so a
site that insists on a real gesture (a file picker, some payment buttons) will not
respond; frames from another origin cannot be read; Chrome only.

## D-148 — A site is opened after confirming, and its address is never guessed

"Book a flight on MakeMyTrip" is usually asked with something else in front. The
plan for that says, as its first line, that it will open the site in a new tab —
and does so only once confirmed. Nothing is opened to prepare a plan (the rule Ask
has always had: nothing touches the world before Commit). The steps are planned
after the page is up, because there is nothing to plan against before.

In the coach the person goes there themselves: "Go to youtube.com in Google
Chrome" is step one, and the coach waits for the tab to be on that site. The coach
shows; it does not act.

The address comes from a short list of known sites, or from an address the person
typed. A name that is on neither is refused with "open it in Chrome, then ask
again". Turning a name into a domain by guessing is how a task ends up signed in to
somebody else's copy of the site.

## D-149 — When a plan runs out, the planner is asked what is left

D-139 planned once and, when the plan ran out, asked the step writer whether that
was all. If it was not, the answer was one step, and every step after it was its
own call. The first live run on a real site showed the cost: the model planned one
step, because it could not see past the first click, and the rest of the run was a
call per step.

Running out of a plan now asks the planner again, with what has been done and the
window as it stands. It answers one of three ways: more steps, which are run as a
plan; `done`, which finishes the run; or nothing, which hands over to the step
writer as before. A task that crosses four pages costs about one call a page
instead of one a step.

`done` is only accepted with no steps beside it, and the prompt says to set it only
when the screen in front of the model shows the task complete. That keeps what
D-139 insisted on: a run is called finished by a model looking at the final window,
never by a plan predicting it would get there.

## D-150 — Waits are for something, not for a length of time

Measured after that run: reading a page takes about 0.1 s, so the page was never
the slow part. The time was model calls and waits written as fixed lengths.

- After acting on a page, the fixed 350 ms is gone. The page is re-read every 90 ms
  and the run carries on the moment it differs, or has started loading, with one
  second as the most it waits. Quicker when a page answers at once, and more
  patient than before with one that takes half a second, which used to be read too
  early and recorded as a click that did nothing.
- Reading a page was two script calls, one to ask whether it was loading and one to
  read it. It is one.
- The pause that shows a step before making it is 600 ms, from 850, and the ghost's
  glide is capped at 560 ms so it still lands before the step happens.
- Every model call logs how long it took, so the next "it is slow" starts from
  numbers.

## D-151 — A step that cannot be taken back waits to be told to go ahead

The confirmation at the start of a run agrees to a task. It cannot agree to the
last screen, because nobody has seen it yet: the fare changed, the wrong chat is
open, the form picked up a default. So a step that pays, orders, books, publishes,
posts, sends or deletes stops before it runs. The overlay turns amber and says what
is about to happen; ⌥Space goes ahead, Escape stops, and two minutes of nothing
stops too.

Two ways a step is known to be one of these, and either is enough:

- The model marks it `final`. It can see context a word list cannot — a "Continue"
  that is in fact the charge.
- The control's own name says so (Pay, Buy, Place order, Confirm booking, Publish,
  Post, Send, Submit, Delete and the like), or the step presses Enter in a message
  box. This is the floor the model cannot lower: a step it forgot to mark is still
  caught.

It reuses the pause from D-146, so going ahead reads the window again and checks
the step still fits. A kept trace stops at the same step every time it is replayed;
remembering how to do something is not remembering permission to do it.

Without a shell to ask through there is nobody to ask, and the step runs under the
confirmation given at the start, as it did before.

## D-152 — Secrets are the person's to type

A password, a card number, a CVV, a PIN, a one-time code and a captcha are never
entered by VANS. On a page such a field is published as `AXSecureTextField` — the
role macOS itself gives a password field — which is not a role that can be typed
into, so a step that tries is refused by the same check that refuses typing into a
button. The prompts say to stop there and say what the person needs to enter.

Found by type, by the browser's own autocomplete hints (`cc-*`, `one-time-code`),
and by name. Erring towards calling a field secret costs a step the person does by
hand; erring the other way puts a card number through a model.

## D-153 — "Unknown" is looked at once more before it is believed

"Send Good Morning to Joe on Whatsapp" was declined as something VANS has no kind
for. The same sentence in lower case was routed correctly, and so were five other
ways of saying it. Measured against the live model, that exact wording came back
"unknown" two times in three: a small model wobbling on a decision the descriptions
left room to wobble on.

Two changes, and the second is the floor.

- The descriptions say what the two general kinds cover. `app.act` is anything a
  person does by clicking and typing in a Mac application and `page.act` anything
  they do on a website, with sending a message, posting, booking and uploading
  named as examples. "Unknown" is for an instruction that asks for nothing to be
  done, found or shown.
- An "unknown" is asked about once more, with the model's own reason and those two
  kinds put in front of it. Only a second "unknown" is believed. It costs one call,
  and only when the first answer was a refusal.

Thirty-six live routings of twelve phrasings afterwards: none declined, three
needed the second look. The cost is that the router is now a little readier to
call something `app.act` — "summarise the PDF on my desktop" went there — which
fails later and more specifically, in the application, rather than at the door.

Routing decisions are also written to the log now. This one took a reproduction to
find because the router had never been given anywhere to write.

A related hole closed in the same change: an application that is installed but not
open is reported as not open, so the person opens it. It used to fall through to
opening the application's website instead, which is not what "on WhatsApp" means
to someone who has WhatsApp installed.

## D-154 — A name is compared by what a person would read

WhatsApp was open and in front, and VANS said it was not running. The application
is installed as `WhatsApp.app` with a left-to-right mark (U+200E) in front of the
W: invisible in the Finder, in the menu bar and in every log, and one character
longer than "whatsapp" to a string comparison. All three ways of finding the
process compared names exactly, so all three missed.

Names are now compared the way they are read. `internal/names` removes format
characters (the Unicode Cf class: direction marks, zero-width spaces, the byte
order mark), folds spacing, and ignores case. It is used where a process is looked
up, where the window list and the application in front are read, where installed
applications are listed, and in the shell where the overlay asks whether the
coached application is still in front. The path on disk keeps its real name; only
the name used for matching and for showing is cleaned.

The same application puts the same mark inside its control names ("New Chat",
"More"), so those are stripped as a window is read. A model asked to spell a name
exactly cannot be expected to reproduce a character nobody can see.

One step further in the same direction, and bounded: a control asked for by a name
that is not on screen is matched to the single control whose name is that name plus
decoration — "Suhani" for `Suhani 🐥`, "Core+Tech Team 2027" for the same with
", 3 unread messages" after it. Letters and digits are compared, whole words only,
and only when exactly one control fits. Two candidates is a question for the model,
not a guess (the rule Repair has always had).

Found only because the log line printed the mark. The lesson for the next one: an
"is not running" about something visibly running is a naming bug until shown
otherwise.

## D-155 — A task in a window is worked through, not written in one go

"Send this message to Suhani on WhatsApp" opened her chat and stopped, reporting
one step run. It had taken the older rung: a whole program written from a single
look at the window. From the chat list the only control that exists is the chat, so
the program was one click, and a program that ran is a task that is finished.

The step loop was already there and was only reached when the one-go program
failed to be written. A program that is too short does not fail. It succeeds at
less than was asked, which is worse.

So the order is reversed for an application with no scripting dictionary: the task
is worked through a step at a time first, and the one-go program is the fallback
for a build with no step writer. The loop reads the window after every step, so it
sees the message box that only exists once the chat is open, and it is the only
path with the stop before sending (D-151).

A one-step task now costs a plan and a check where it cost a single call. It is
kept after the first run and costs nothing after that, and the alternative was
being wrong quickly.

## D-156 — Only Escape stops a run; moving the mouse does not

D-146 paused a run when the pointer moved, on the reasoning that a hand on the
mouse means the person wants to do something themselves. Tried live, it was wrong:
people move the mouse while watching, to see what the ghost is pointing at or
simply because their hand is on it, and the run stopped for a reason they had not
given. Withdrawn. Escape is the way to stop a run, and it is the only one.

What stays from that work is the part that was asked for: a step that cannot be
taken back waits for ⌥Space (D-151), and ⌥Space has its own signal so a click on
the ring does not count as going ahead.

Two more things the same run showed:

- The check made just before acting — is the control under this point the one the
  step named — compared names exactly, and WhatsApp's carry the invisible mark
  D-154 was about. It read the window with the mark stripped and the point with it
  kept, and refused its own control. Both sides are now compared the same way.
- The model marked typing a message as the step that cannot be taken back. Typing
  commits nothing, so a `final` mark on a typing step is ignored, and the prompt
  says to mark the click or the Enter that sends.

## D-157 — A task is shown before it is done

Asked to do something in an application or on a site, VANS did it. The owner's
order is the other way round: showing how comes first, and doing it for someone is
the thing they ask for. The coach is the product being finished; acting on its own
is what follows (and, later, in the background).

So a request the router calls `app.act` or `page.act` is, by default, answered by
the coach. The same application or site is found the same way — the named
application if it is installed, the site if it is one, the chooser otherwise — and
a guide is planned there with the person's own words as the task. An application
that is not open becomes "Open WhatsApp" as step one, which is what the coach has
always done and what VANS acting could never do (D-032).

The choice is in the panel, not in the wording: "Show me how" or "Do it for me",
remembered between sessions, with ⌘↩ taking the other one for a single request. A
mode guessed from phrasing would be wrong some of the time in the direction that
matters, which is acting when someone wanted to be shown.

The mode is a field on the request. With none given the engine does what the route
says, so the command line and the tests are unchanged.
