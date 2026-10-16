# AGY — the working record

Everything VANS is, everything that has been built, every fact that cost
something to learn, and what happens next. Written to be read cold, six months
from now, by someone who has forgotten all of it.

Companion documents: `docs/decisions.md` is the reasoning behind each choice
(75 records, D-001 to D-075); `docs/roadmap.md` is the phase plan;
`docs/backlog.md` is decided-but-unbuilt work. This file is the state of play.

**Part I — The record**

1. What VANS is · 2. How it got here · 3. Shape of the system · 4. The ladder ·
5. What is built · 6. Facts that cost something to learn · 7. Data and
permissions · 8. Known problems · 9. What happens next · 10. Running it ·
11. Standing constraints

**Part II — Working guidelines**

G1. Code · G2. Git, branches and PRs · G3. Testing and verification ·
G4. Documentation · G5. Communication · G6. Product rules · G7. UI and design ·
G8. Decided, and parked for later · G9. The domain and the website ·
G10. Testers, the server and the accounts

---

# Part I — The record

## 1. What VANS is

**VANS — Visual Agent Navigation System.** (It began as "Virtual Agent
Navigation System", became "Vedansh's Agent Navigation System" on 2026-08-16,
and has been "Visual" since 2026-10-13; D-222.) A macOS runtime that sits between
a person and their machine. You summon a panel with a hotkey, say what you want
in your own words, and it does it — on the web, inside applications, or on the
screen in front of you.

### The thesis

A language model writes a program **once**. The runtime owns it, checks it,
caches it, and replays it with no model involved.

A cold run on a new shape costs one or two model calls. Every warm run on the
same shape costs **zero**. That is the whole claim, and it is measured rather
than asserted: the synthesiser is wrapped in a counter so the test suite can
assert the count is exactly zero on a repeat.

The corollary is that VANS gets **faster and cheaper the more you use it**,
which is the opposite of an agent that thinks from scratch every time.

### The floors

Accuracy comes from deterministic checks the model cannot lower, never from
asking the model nicely:

- An extraction program is run and its output verified before it is trusted.
- A selector that matches nothing is refused, not retried hopefully.
- A click point is re-checked against the live window before the click.
- An answer with no citation into the rows is thrown away.
- A restyle whose selector matches nothing is refused.

Where the model can only choose from a set the runtime defines, it cannot be
wrong in a way the runtime has to catch. The restyle tier is the clearest case:
the model picks a selector and an emphasis, and never writes a colour, so there
is no value to validate at all.

---

## 2. How it got here

| When | What happened |
|---|---|
| 2026-08-01 | The vision written down: a runtime between the user and the operating system, where a model plans and the runtime executes. |
| 2026-08-04 | GUI first. Applications over browsers in V1. The design-decision log started, so nothing lives only in a session. The first code typed by hand, file by file, to learn it. |
| 2026-08-12 | Pushed to GitHub; branches from here on. A free hosted model, open-weight, never run locally. |
| 2026-08-16 | Renamed to Vedansh's Agent Navigation System. The pivot: not an app window but a hover panel summoned over whatever you are doing — Clicky as the reference. |
| 2026-08-27 → 09-01 | The overlay shell: hotkey, panel, light theme by default, theme switcher. |
| 2026-09-10 | The big build day. Destinations (panel, clipboard, Notes, the app in front). The five hard constraints. Apple design language; a consumer product, not tooling. Rich results, images, real links. The panel design from the design prototype adopted as final. Drives the real, signed-in Chrome. Dismiss on click outside only. The ladder decided: cheapest reliable path first, accessibility only as fallback. |
| 2026-09-12 | Standing permission to merge and keep going. Acting natively in applications, visibly on screen. Minimum permissions and no data leak made explicit. The C-API tree walker; the web-version route; remembering the last three instructions. |
| 2026-09-13 | Why nothing on screen ever worked: `delay` returning -1708, the middle of a tall control being off screen, Stage Manager owning the hit test (#40). |
| 2026-09-17 | Why VANS would not open (the engine was never built). Gemini wired in. Input moved from scripting to posted events (#44). Read-back verification (#46). CSS injection, softened and background-aware (#47). The on-screen coach overlay. |
| 2026-09-18 | This record and the README (#48); the working guidelines added. |
| 2026-10-13 | The domain `summonvans.in`, the site plan, and the name settled as Visual Agent Navigation System, shown only in official documents (D-222). VANS said plainly to be a teacher first. |

## 3. Shape of the system

Three processes, deliberately:

```
┌───────────────┐   hotkey    ┌──────────────────┐   stdin/stdout   ┌──────────────┐
│  macOS user   │ ──Alt+Space─▶│  Shell (Rust)    │ ◀──JSON lines───▶│ Engine (Go)  │
└───────────────┘              │  Tauri v2        │                  │ vans-engine  │
                               │  panel + coach   │                  └──────┬───────┘
                               └──────────────────┘                         │
                                                                    ┌───────┴────────┐
                                                                    │ Chrome (Apple  │
                                                                    │ Events / CDP)  │
                                                                    │ Accessibility  │
                                                                    │ CoreGraphics   │
                                                                    └────────────────┘
```

**The engine is a child process spoken to over pipes, never a port.** A
localhost port would let any process on the machine trigger `commit`, which
performs irreversible side effects. See D-036.

### Layout

| Path | What lives there |
|---|---|
| `cmd/vans-engine` | the runtime the shell spawns; JSON-lines RPC over stdio |
| `cmd/vans` | the same engine as a CLI, for running without the GUI |
| `internal/engine` | orchestration: routing, the ladder, plan/commit, sinks |
| `internal/route` | instruction → kind + slots (the router and its specs) |
| `internal/extract` | page signature, candidate roots, the distiller |
| `internal/runner` | executes an extraction program and verifies the result |
| `internal/synth` | every model call: extract, route, act, answer, drive, style |
| `internal/appact` | acting through an application's scripting dictionary |
| `internal/webact` | acting inside a page the user has open |
| `internal/webstyle` | restyling a page (CSS injection) |
| `internal/uiact` | reading and driving a window through accessibility |
| `internal/capability` | what each installed application declares it can do (sdef) |
| `internal/store` | SQLite: programs, routes, actions, instructions, settings |
| `internal/rpc` | the protocol the shell speaks |
| `internal/adapter/cdp` | Chrome DevTools Protocol |
| `internal/adapter/appleevents` | JXA through osascript |
| `ui/src-tauri` | the shell: panel window, coach window, engine client |
| `ui/src/main.ts` | the panel |
| `ui/src/coach.ts` | the on-screen coach overlay |

Roughly 24,400 lines of Go across 145 files, 423 tests, 68 decision records, 1,450 lines of Rust,
1,600 lines of panel and overlay front-end.

---

## 4. The ladder for acting in an application

When you ask VANS to do something in an application, it tries these in order and
stops at the first that works. Each rung is cheaper, more precise, or more
honest than the one below it.

The order was a choice between three options on 2026-09-10 — the accessibility
tree everywhere, System Events UI scripting, or not chasing unscriptable apps at
all. The answer was a mix: take the cheapest reliable path first, and fall back
to the accessibility tree only where nothing else reaches. Fast first, thorough
as the fallback.

1. **Scripting dictionary** (`appact`) — the application's own sdef. Fastest,
   and it says plainly which operations change state, so the confirmation gate
   can be decided from the dictionary rather than guessed. Notes, Reminders,
   Music, Mail.
2. **The page it is open in** (`webact`) — Telegram, Notion, Discord and Slack
   ship no dictionary, and every one of them has a web version already open in
   the browser. Costs no new permission.
3. **The accessibility tree** (`uiact`) — read the window, find the control,
   click it where it actually is, type into it, or press a key with modifiers
   held (Command-S, Shift-Tab). Works for native applications.
4. **Its web version** (`webversion`) — for an application that publishes
   nothing useful, open the site that does the same job.
5. **Driving it step by step** (`drive`) — read the window, take one step, read
   it again, up to 12 steps. The tier of last resort, and the one that can be
   watched happening.

---

## 5. What is built

### Merged this session

| PR | What it does |
|---|---|
| #40 | Click where the control actually is; ask the right thing about what is on top |
| #41 | Say which application an action happens in, and offer one |
| #42 | Wait for the window, and say when something is hiding it |
| #43 | Offer the application in front; stamp an action as an action |
| #44 | Post input instead of scripting it |
| #45 | Type where the cursor is when an application publishes nothing |
| #46 | Read the window back and say what changed |
| #47 | Restyle the page in front, and take it back off |
| #48 | Write down what this is (this file, the README), and add the on-screen coach |
| #49 | Add the working guidelines to the record, and bring it up to date |
| #50 | Show me what to do, a step at a time (coach loop, AX change polling) |
| #51 | Write the model plumbing once |
| #52 | Tell showing from styling, and drop decisions the kinds outgrew |
| #53 | Ask the coach a question it can answer (frame questions as tasks) |
| #54 | Shrink the panel while the coach is pointing |
| #55 | Bring the record and progress log up to date with the coach |
| #56 | Smoothly morph the coach spotlight and glide the tooltip |
| #57 | Keep the panel on the screen, and build the engine with the app |
| #58 | Bring the record up to date with the panel fix |
| #59 | Detect frontmost application via CGWindowListCopyWindowInfo without Apple Events |
| #60 | Multi-step coaching progression, session advance channel, and clean cancellation |
| #61 | Keep panel non-activating, forward screen context, and compact HUD |
| #62 | Update the record with #61 |
| #63 | Prove the full click loop: hermetic target resolution, click posting, context app inference |
| #64 | Progress while committing, retry a refused step with its reason, receipt rests on the window |
| #65 | Measure that the panel leaves the window readable; one name for the panel's two widths |
| #66 | Let a step hold modifiers down, from one key vocabulary instead of four |
| #67 | Coach a step at a time without taking the screen over |
| #68 | Keep reading the window passive; recovery belongs to the step that needs one |
| #69 | Point at the app, not from a window over it |
| #70 | Keep the lines the window shows, where they can be read after a run |
| #71 | Draw the first step, and stay up until there is one |
| #72 | Let the coach window use the event system, and give it something to draw |
| #73 | Draw the first step where it actually is, and refuse a step usefully |
| #74 | Draw what to do, not only where |
| #75 | Finish a step when it is finished, and say when nothing was pointed at |
| #76 | Stop waiting three minutes on a model that is not answering |
| #77 | Settle the keypress, refuse a browser early, and clear every tab |
| #78 | Make the live drive test drive something, and stop verifying nothing |



### Before this session

#14–#39 built the panel, web extraction, search, the answer tier, the
scripting-dictionary tier, the page tier, the accessibility tier, the drive
loop, suggestions, the distiller, the C-API tree walker, and the web-version
routing. `docs/decisions.md` carries the reasoning for all of it.

### Proven live, with numbers

- **Warm path is free.** Application actions: cold 2,970 ms / 2 model calls →
  warm 11 ms / **0 calls**, same shape.
- **A repeated drive run is free too.** `TestLiveDriveTaskTwice` on 2026-09-26,
  TextEdit, "type the word hello", a fresh store and the real model:

  | run  | steps | model calls | time     |
  |------|-------|-------------|----------|
  | cold | 1     | 3           | 6,052 ms |
  | warm | 1     | **0**       | 866 ms   |

  Seven times faster on a one-step task, and the whole difference is model calls
  not being made. The cold run's three calls for one step are worth reading: one
  to work the step out, one because the first answer was refused, one to hear the
  task was finished. The two call-saving items in §9.8 would take that to one.
- **The covered-window guard works in the wild.** During that same cold run another
  application's window was sitting over TextEdit, and the step was refused before
  it could click the wrong thing: `(508,379) is inside <other app>'s window, which
  is in front of TextEdit`. The window was then raised and the step landed (D-112).
  This is the first time that guard has been seen to fire outside a test.
- **Reading a window is cheap.** Notes: 119 controls in 132 ms through the
  C accessibility API. Before that, through Apple Events, System Settings took
  23.9 s — which is why D-057 moved it to cgo.
- **The distiller works.** A real page: 638 KB of HTML → 2,989 bytes sent to the
  model. 0.5%.
- **Typing into a page works.** Text typed into DuckDuckGo through the page
  tier, end to end.
- **Restyling works.** "highlight every paragraph" on a live page: 1,210 ms,
  **0 routing calls** (cached shape), 1 synthesis call. Verified on a dark page
  — it measured `rgb(0,0,0)` and chose the dark palette by measurement, not by
  guessing.
- **Clearing a restyle costs no model call at all.** It is recognised in Go.
- **Pointing at a control on question works.** "where is the search bar" in
  System Settings pointed at the search field at (459, 100 195x28) (D-071).
- **A posted keypress reaches the panel.** Settled on 2026-09-23 by reading the
  app's own log instead of asking the accessibility API. Both paths work: an
  `osascript` keystroke through System Events, and a bare `CGEventPost` of
  Option-Space to `kCGHIDEventTap` from a freshly compiled binary. Each produced
  `[vans] hotkey fired`; through Go it took 0.26s.
  `TestPostedHotkeyReachesThePanelLive` keeps it repeatable (D-098).
- **The model ladder works against a real provider.** A run whose first model
  does not exist fell through to the configured one and came back with a working
  program in 2.04s; a one-millisecond wait gave up in 2 ms saying the model did
  not answer, rather than after three minutes saying nothing (2026-09-23).
- **The coach sends you to the right application, and waits while you open it.**
  2026-09-23, driven end to end with a posted hotkey: Chrome in front, asked "how
  do I change the wallpaper". It chose System Settings over Chrome, drew
  `Step 1 — Open System Settings` as an instruction with no control to ring,
  noticed the application appear, then drew `Step 2` on the real search field at
  (459, 100) 195x28. The question the panel is summoned over is no longer mistaken
  for the question being asked (D-102).
- **The drive loop completed a task on a real window.** 2026-09-23, TextEdit, an
  empty document, "type the word hello into the document". 44 controls read; the
  model asked for a click on the text area; the occlusion guard refused it — "is
  inside DBeaver Community's window, which is in front of TextEdit" — the step was
  re-asked with that reason and came back as typing instead; the text landed; the
  window read back `the AXTextArea "" now holds "hello"`; receipt verified on that
  evidence. 5.54s, two model calls. The whole ladder in one run: capture, write,
  validate, refuse with a reason, re-ask, act, read back, verify.

### Not proven

- **A posted click has still never been watched landing on a real control.** The
  drive loop completed a task on 2026-09-23, but by typing: the one click it
  attempted was refused by the occlusion guard, correctly, because DBeaver was
  over TextEdit. Every piece is verified separately and the click itself remains
  the one that has never been seen through to an effect.
- **And the reason it was refused is a gap worth closing.** `windowToWorkFrom`
  only raises the target when `Capture` returns `ErrNoWindow`
  (`internal/engine/drive.go:194`). TextEdit was publishing 44 controls while
  sitting behind DBeaver, so it was readable, never raised, and every click into
  it was unreachable. A step refused purely because another window is on top has
  an obvious recovery — raise the target once, read again, re-ask — and the drive
  loop does not take it. D-080a restricted raising to the callers that mean it,
  and a caller about to click is one of them. This is the next thing to fix.
- **The panel can no longer be stranded off screen** (#57), which is what made
  the coach look broken: the keystrokes were reaching a window above the menu
  bar.
- **The coach overlay is now wired to the engine** (`app.guide` route, #50) and
  responds to single-step questions (#53) with HUD shrinking (#54). A multi-step
  interaction across changing window states has not yet been watched live, and its
  visual design has not been reviewed.

---

## 6. Facts that cost something to learn

These are the ones worth keeping. Every one of them was a wrong assumption held
for at least an hour.

### macOS and the accessibility API

- **An application whose window is not on screen publishes nothing.** This was
  recorded as "only the frontmost application publishes AX windows" (D-051) and
  that is false: Finder published its window while another application was
  frontmost. What matters is whether the window is on screen, not whose it is.
- **Being frontmost is not the same as being on screen.** A process can be
  frontmost while Stage Manager keeps its window off-stage. Forwarding now waits
  for a window to actually appear and says what is probably hiding it (D-064).
- **A read must not raise.** Recovering from a withdrawn window inside `Capture`
  meant the coach raised the coached application twice a second while waiting for
  someone to act, undoing every switch away from it, and the guard that ends a
  guide when the window is gone could never fire (D-080a).
- **With Stage Manager on, only the *active application* publishes anything.**
  Measured on 2026-09-20 with Chrome active: System Settings 0 elements, Preview
  0 elements, Finder 1 (the desktop), each with `onScreen = false` and
  `ErrNoWindow`. Activate any other application and the window the user is
  looking at stops existing as far as the accessibility API is concerned. This
  is the whole reason coaching failed: anything that makes VANS the active
  application blinds it.
- **A non-activating panel must never be given focus through Tauri.**
  `WebviewWindow::set_focus` reaches tao's `util::set_focus`, which calls
  `makeKeyAndOrderFront` *and* `activateIgnoringOtherApps: YES`
  (tao 0.35.3, `platform_impl/macos/util/async.rs:231`). The second call
  activates VANS, which defeats `NSWindowStyleMaskNonactivatingPanel` entirely.
  `show()` alone already calls `makeKeyAndOrderFront`, and a non-activating panel
  takes the keyboard from that without its application becoming active.
- **Stage Manager owns the system-wide hit test.** With it enabled,
  `AXUIElementCopyElementAtPosition` returns an element belonging to
  `WindowManager` at every point on screen. An ownership check against the
  target application is therefore false *everywhere*, which silently refused
  every click VANS was ever asked to make (D-062).
- **Window stacking is a window-list question.** `CGWindowListCopyWindowInfo`
  answers it correctly; the accessibility API does not.
- **The window list has traps.** Some applications keep their only window at the
  desktop layer rather than layer 0, and a Chromium window belongs to a
  different process than the application name resolves to. A naive "does this
  pid own a window here" check returned false for every application tested.
- **Chromium and Electron publish windows with nothing inside them.** WhatsApp:
  two windows, zero children, across five consecutive passes. VS Code: two
  windows, zero children. Neither `AXManualAccessibility` nor
  `AXEnhancedUserInterface` changes it. The page tier is the answer for these,
  not the accessibility tier (D-056, D-059).
- **The middle of a control is not a place you can click.** A note longer than
  its window makes the text area 22,468 points tall, putting its centre ten
  screen-heights below the desktop. The click point must be the centre of the
  *visible* rectangle: the frame clipped by every ancestor that encloses it, and
  by the display bounds (D-062).
- **`pgrep -f` lies.** `pgrep -f 'Visual Studio Code'` returned the wrong pid
  (16466 instead of 50219). Use `pgrep -x` on the exact process name — about
  12 ms — and fall back to System Events (about 100 ms) when the application
  name and the process name differ (#36).

### JXA and Apple Events

- **`delay` does not work.** The current application's standard-additions
  `delay` answers `-1708` on this machine, with or without
  `includeStandardAdditions`. It sat after every click, keystroke and key press,
  so the on-screen renderer failed on its first step in every run it ever made.
  Replaced with `NSThread.sleepForTimeInterval` (D-065).
- **`se.click({at:[x,y]})` does not work.** Answers `-1700`, in both parameter
  forms. No click VANS rendered has ever landed.
- **System Events puts the permission on the wrong binary.** Driving the screen
  through it requires the Accessibility grant on `osascript` — a binary any
  script on the machine can use — rather than on VANS. That is the opposite of
  asking for as little as possible, and it is why input moved to CoreGraphics
  (D-065).
- **Never stringify an AX specifier.** `String(specifier)` throws `-1708`.
- **An aborted `osascript` loses all stdout.** Log to a file through
  `doShellScript` when debugging a script that may throw.
- **`proc.frontmost = true` works.** An earlier reading blamed it for a `-1708`
  that `delay` was actually causing.

### Windows, and losing them

- **A window that is entirely off screen belongs to no monitor.**
  `current_monitor()` returns nothing, so every correction that starts by asking
  for one gives up before it does anything. A panel dragged above the menu bar
  could not be brought back by any means the application had, and summoning it
  did nothing visible while the keyboard went to it anyway — which reads exactly
  like a hotkey that has stopped working.
- **Dragging had no bounds at all.** Position was set from the drag delta with
  nothing holding the edges, which is how the panel got somewhere it could not
  come back from. It stops at the edge now, the correction falls back to the
  primary display, and it runs on every show rather than only on resize.
- **A dev command that does not build the runtime will hide every fix you make.**
  `npm run dev` started the app against whatever engine binary happened to be on
  disk, so several rounds of testing ran against code that did not contain the
  change being tested. It builds the runtime first now. A rule in a document
  asking someone to remember this is not the same as the command doing it.

### Tauri

- **A window in no capability is refused the event system.** Capabilities are
  granted per window label, and `capabilities/default.json` listed only `main`.
  The coach window was in none, so `listen` in its page was denied and no event
  ever arrived — which is why the ring never appeared, through every round of
  work on levels, ordering, activation and timing. A window permitted nothing
  and a window drawing nothing look identical from outside (D-086). A new window
  needs a capability before it needs anything else.
- **A page built on demand cannot be sent anything until it says it is
  listening.** The window is created the moment there is something to point at,
  and `emit_to` immediately after reaches no listener — the event is dropped, not
  queued. The coach page emits `coach://ready` and the last pointer is drawn
  again (D-084).
- **A webview that cannot speak has invisible failures.** Both pages write into
  the shell's log through `note`, and the draw path reports the display it
  covered and the rectangle it drew. Everything above was diagnosed from that log
  rather than guessed at.

### AppKit

- **Building a window off the main thread kills the process.** Silently, with
  exit code 0. `point_at` marshals to the main thread itself rather than
  trusting its caller.
- **Floating over a full-screen app needs three things**, not one: the
  collection behaviour, a raised window level, *and* a non-activating panel
  class — because an ordinary window becoming key activates its application,
  which pulls macOS out of the full-screen Space to get to it.

### Models and schemas

- **A free-form `object` in a structured-output schema comes back empty.**
  Gemini returned `{}` for `declarations` twice in a row. A list of
  `{property, value}` pairs works everywhere. Prefer arrays of named fields over
  maps in any schema a model must fill.
- **"OpenAI-compatible" is a claim about the endpoint shape, not about
  structured-output support.** Hence `VANS_LLM_SCHEMA_MODE`, which steps down
  from `json_schema` to `json_object` to prompt-only.

### Verification discipline

- **`cmd && echo OK` printed "BUILD+VET OK" over a failing build.** Every
  verification since checks exit codes explicitly.
- **`go build ./...` does not build the no-cgo variant.** The `!cgo` build was
  broken for two PRs without anyone noticing. Check it explicitly.
- **A probe that skips the guard is not a test of the system.** A raw
  CoreGraphics click posted without `VerifyTargets` landed on the wrong control
  and closed an application. The guard exists for exactly that.

---

## 7. Data and permissions

Two standing constraints, both load-bearing.

**Minimum permissions.** VANS asks for Accessibility (to read and drive windows)
and Automation for the browser. Input synthesis deliberately uses the
Accessibility trust it already holds rather than requiring a second, broader
grant on `osascript`.

**No data leak.** What leaves the machine is minimised per tier, and the rule is
not "is this sensitive" but "is the request about it":

- Acting on a window sends roles and labels, never the values controls hold
  (D-058).
- Answering *about* a window sends the text, because the text is the answer
  (D-061).
- Restyling a page sends selectors and counts, never page text — the page in
  front of someone is as likely to be their mail as a shop (D-068).
- A restyle may not carry `url()`, because a stylesheet can exfiltrate which
  elements its selector matched by requesting a background image. The emphasis
  design removed the question entirely: the model writes no values.

---

## 8. Known problems

1. **Chromium and Electron applications cannot be read at all.** Chrome, VS
   Code, Telegram and Slack publish a window with zero children.
   `AXManualAccessibility` is unsupported on Chrome and accepted-but-useless on
   Electron — the tree is still empty eight seconds later. Native applications
   are fine. Reaching these needs a vision tier or their web versions (D-060).
2. **Chrome's accessibility tree comes back empty** (0 elements), so anything
   routed to the accessibility tier for a browser fails. Reading and suggesting
   already route around it, and since #77 the coach refuses before raising the
   window and says what to do on the page instead (D-099). What is still missing
   is the positive version: drawing the coach's rings and arrows over elements in
   a web page, which needs geometry from the DOM rather than from the
   accessibility API. That is a feature, not a rough edge.
3. ~~Clearing a restyle acts on the tab in front.~~ **Fixed in #77.** Clearing
   sweeps every tab of every window and reports how many carried a restyle; a tab
   that cannot run anything is skipped rather than failing the sweep (D-100).
4. **Credentials load from a `.env` file** next to the runtime, so the app no
   longer depends on the shell that launched it.
5. **Nothing is packaged.** There is no built `.app`; it runs from a checkout
   through `npm run tauri dev`.
6. ~~Multi-step coaching has not been exercised across real state transitions.~~
   **Done on 2026-09-21** — six steps through "how do I change the wallpaper" in
   System Settings, with the overlay drawing each one. What blocked it for so long
   is in §9.0: summoning the panel activated VANS, and under Stage Manager that
   withdraws the target window from the accessibility API, so every re-read after
   the first step came back empty.
7. ~~The keyboard cannot be driven from outside.~~ **Wrong, settled on
   2026-09-23.** It always could; the measurement was broken, not the path. See
   the entry under *Proven live* and D-098.
8. **Nothing VANS does can be undone, except a restyle.** `grep -i undo` over the
   runtime returns one file. Typing, clicking, a dictionary write, a twelve-step
   drive: all one-way, and the store keeps no record of what was done, so there is
   nothing to undo from. This is the project's own rule broken everywhere but one
   place, and it is what §9.5 is about.

---

## 9. What happens next

In order. Each item says what it is and how it gets built.

### 9.0 The panel was stealing the front window (fixed in #61; one measurement left)

Summoning the panel called `win.set_focus()`, which on macOS reaches
`activateIgnoringOtherApps` and makes VANS the active application — and with
Stage Manager on, every other application then withdraws its windows from the
accessibility API. That is why the coach pointed at nothing and why multi-step
never advanced: each re-read came back empty until the three-failure guard
reported the window closed. Recorded as D-075.

Fixed: `set_focus` is non-macOS only, and `show()` alone makes the non-activating
panel key without activating the process. `Capture` retries once through
`bringForward` when the window list comes back empty. `appInQuestion` takes the
application from the context the shell captured when the hotkey was pressed —
`prepare` carries it end to end — and refuses VANS itself; `planGuide` and
`readWhatIsInFront` share it. The panel shrinks to a 380pt strip while the coach
points and goes back on every exit path, and guide progress is written into the
coaching line, the only place it is visible while the coach is up. 176 controls
captured from System Settings in 868ms.

**Left:** that capture has not been measured with the panel actually on screen,
which is the condition that used to break it.
`TestLiveTheSummonedPanelLeavesTheWindowReadable` does it properly — it calls
`walkApp` rather than `Capture`, because `Capture` now recovers and would hide the
failure, and it skips unless VANS is running *and* its panel is on screen, so it
cannot pass for the trivial reason that there is no panel. Summon the panel over
System Settings, leave it up, then:

```bash
VANS_LIVE=1 VANS_LIVE_APP="System Settings" \
  go test -tags integration ./internal/uiact -run TestLiveTheSummonedPanel -v
```

**Reading Clicky is not a problem and it does not need installing.** The DMG in
Downloads was already inspected: ScreenCaptureKit plus a Screen Recording grant,
ten `AXUIElement*` symbols, frames posted to `api.heyclicky.com`, 787MB,
`LSUIElement=true`. Mounting it again would only help for watching its runtime
behaviour, which is not what was blocked.

### 9.1 The coach flow, end to end — "show me what to do" (Built)

Wired and working in #50, #52, #53, #54. The engine brings the application
forward, captures controls, passes roles/labels to the step writer, checks the
target's visible rectangle, and commands the overlay while shrinking the panel
HUD. Verified single-step against System Settings ("where is the search bar").

### 9.2 Multi-step coaching (built; the live run is what is left)

Point at the first control; the *person* clicks it; re-read the window; point at
the next. This reuses the drive loop's structure with the click removed (#50) and
polls every 500ms for window changes (`Changes(before, after)`, D-067, D-073).
Session-scoped advance channel behind the "Next" button, clean cancellation behind
"Stop" and Escape, three-failure window guards (#60). A locational question
("where is X") stops once the control has been pointed at; a procedural one ("how
do I X") continues up to 12 steps.

Four things then stood between it and being usable, all fixed in #67: the panel
closed itself on the very click it had asked for, taking Next and Stop with it
(coaching is now a pinned screen); the application being coached did not hold the
keyboard, so the first keystroke went to VANS and the window stopped publishing
controls (D-079); a step nobody had done yet failed the whole run after forty
seconds instead of being pointed at again (D-078); and naming an application fell
through to System Events, which is the Automation prompt that had been blocking
runs (D-080).

Driven live on 2026-09-21 against System Settings through the real stdio
protocol, with real model calls, which turned up three things the unit tests
could not: the panel sat on top of the application it was pointing into (D-081),
a value ticking over in the window was read as the person acting and skipped a
step nobody had touched (D-082), and a window that stopped publishing — which is
what happens whenever someone glances at another application — ended the run
(D-083). The recorded run after the fixes: point at (459,100), wait through the
application not being in front, re-point the same step after the quiet spell,
stop cleanly on Stop.

The drawing was then tested by hand and did not appear. Two causes, neither of
them about levels or ordering. The coach window is built on demand and its page
takes a moment to load, so the event carrying the first step was emitted before
anything was listening and dropped (D-084) — the page now says when it is
listening. And, underneath that, the coach window was in no capability at all, so
Tauri refused it the event system outright (D-086). The panel also waits for the
first step before leaving the screen, since an empty screen reads as the app
closing (D-085).

What the overlay draws is in D-087: the screen dimmed except for the control,
corner brackets on its edges, a pointer cursor resting on it with a click ripple,
an arrow curving from the caption, and the caption with the step number and the
instruction.

**What the overlay draws now** (D-087, D-090, D-091, D-092): the screen dimmed to
34% with the hole fading out around the control rather than cut square; corner
brackets on its edges; the verb the step asked for — a pointer cursor and ripple
for a click, a caret where the text will go for typing, neither for a key
combination since there is nothing on screen to press; an arrow curving from the
caption; and the caption itself with the step number, the instruction, and the
text to type or the keys to hold spelled out. A finished run marks the last
control with a tick and says so before taking itself away, so finishing does not
look like being stopped.

**Coached live through a real task on 2026-09-21.** "How do I change the
wallpaper" ran six steps in System Settings — click the search field, type
Wallpaper, press Enter, click the wallpaper — with the overlay drawing each one.
Three things that run showed, all fixed in #75:

- The coach moved on from "type Wallpaper" after the first keystroke, then asked
  for "type paper" as a step of its own. Typing is done when the text is in the
  window, not when the field first changes (D-093).
- It asked for Enter, the person pressed it, the Wallpaper section appeared, and
  the next step was to press Enter. A step listed under what has been done is
  done, whatever the screen looks like afterwards (D-094).
- A run in front of the wrong application returned success with no steps, and the
  panel came back saying "that is all of it" having shown nothing. Zero steps now
  carries the model's own account of why, and the panel says it (D-095).

**Stopped working on 2026-09-21 06:48, and it is not the code.** The panel sat at
"working out step 1" and nothing came back. Measured from the terminal with the
same key and base URL the runtime uses:

| Request | Result |
| --- | --- |
| `GET /models` | 200 in 0.68s, 58 models listed — key and network are fine |
| `POST /chat/completions` with `gemini-flash-lite-latest` | no response at all; `http=000` after 25s and again after 45s |
| `models/gemini-flash-latest` | 503 |
| `models/gemini-2.5-flash-lite`, `models/gemini-2.5-flash` | 404 through this endpoint |
| **`models/gemini-3.1-flash-lite`** | **200 in 1.5s** |

So the configured model is stalling on Google's side, not refusing — the
connection opens and never answers. The HTTP client allows three minutes, which
is why it looks hung rather than failed.

**The stall was temporary, and the run is no longer at its mercy.** Re-measured
on 2026-09-23 with the same key and base URL: `gemini-flash-lite-latest` answered
200 in 1.7s, and `models/gemini-3.1-flash-lite` in 5.1s. The configured model was
never the problem, so nothing was switched — what was wrong is that a silent
provider was indistinguishable from a hang, and that is what #76 fixed:

- One request gets 45 seconds, not three minutes, raisable with
  `VANS_LLM_TIMEOUT` for a model slow enough to need the room (D-096).
- Silence is its own failure. It is not retried, because re-asking a model that
  never answered spends the whole wait again with nothing new; it reaches the
  panel as "The model didn't answer" rather than "something went wrong inside
  VANS", under its own error code.
- The person pressing Stop is told apart from the model going quiet by whether
  the caller's context is still alive. Both arrive at the same line looking
  alike.
- `VANS_LLM_FALLBACK_MODEL` names a second model, tried when the first goes
  silent, stays overloaded past the retry budget, or comes back 404 — the three
  ways a run dies with a working key. Not tried for a 400, which is our own
  request being wrong (D-097). Unset by default.

Both halves are proven against Google's endpoint, not only against a stub: with
`-tags integration`, `TestFallbackModelAnswersLive` names a model that does not
exist and gets a working program back from the fallback in 2.04s, and
`TestShortWaitIsReportedAsSilenceLive` gives up in 2 ms with
`gemini-flash-lite-latest did not answer in 1ms`. Both live in
`internal/synth/live_test.go`.

**To pick up:**

1. **Set a fallback and watch it fire.** `VANS_LLM_FALLBACK_MODEL=models/gemini-3.1-flash-lite`
   in `.env`. The ladder is covered by hermetic tests against a server that never
   answers; it has not yet been seen switching models against a real provider,
   which needs an outage to happen again.
2. **Raise a window that is only covered, not missing.** The first live drive run
   exposed it: `windowToWorkFrom` raises the target only when `Capture` returns
   `ErrNoWindow`, so a window that publishes controls while sitting behind another
   application is read happily and every click into it is then refused as
   unreachable. The recovery is one raise and one re-read. This is also what
   stands between the run and the last unproven thing in the system — a posted
   click seen landing on a real control.
3. **A live drive run needs a target opened by hand** (VANS does not open
   applications, D-032). An empty TextEdit document is the one to use; Chrome and
   Notion are Chromium-shaped and hold real work, and Clock is out by standing
   constraint.

   ```bash
   VANS_LIVE=1 VANS_LIVE_DRIVE_OK=1 VANS_LIVE_APP=TextEdit \
     VANS_LIVE_TASK="type the word hello into the document" \
     go test -tags integration -v -run TestLiveDriveTask ./internal/engine/
   ```

   Done for the first time on 2026-09-23; #78 is what made the test capable of
   failing.
4. ~~A real keypress reaching the panel.~~ **Done in #77**, see *Proven live*.

**Deferred, on purpose:**

1. **A resting place on screen.** Clicky sits as a pill at the top edge and
   expands on hover. Parked: Clicky is staying installed as a reference, and VANS
   stays hotkey-summoned for now.
2. **Speech.** Clicky reads each step aloud. Writing the instruction is enough.
**It drew, on 2026-09-21.** The first run in which anything reached the screen:
step 1 pointed at the Search field at (459, 100), the person clicked it, step 2
pointed at Desktop & Dock at (464, 246) — with the dimming, the ring, the corner
brackets, the cursor, the ripple and the arrow all rendering over System
Settings. The whole tier is proven end to end for the first time.

It stopped at step 3, which turned up two more things, both fixed in #73: the
first step of a session was drawn at (-196, -133) because the window's origin was
read back before it had moved (D-088), and "click Wallpaper" was refused twice
for naming an AXButton when the sidebar row is an AXRow — the refusal now says
what is on screen under that name, since a refused step is re-asked with the
reason in hand (D-089). The run
to make, on System Settings:

1. Ask something with more than one step — "how do I turn on Dark Mode".
2. The panel disappears; the ring, the corner brackets and the arrow land on the
   first control, with the caption beside it.
3. Click what it points at: the shell sees the click land inside the ring and the
   coach moves to step 2 without waiting for a poll.
4. Option-Space brings the panel back with Next and Stop; the panel also comes
   back on its own when the run ends or fails.

### 9.3 Prove the full click loop (done in #63)

Bring the application forward, capture, plan, verify, click, read back, report
what changed — with hermetic target resolution and click posting under test.

### 9.4 The app agent workflow (built, and run live)

With input posting and read-back both working (#63), the drive loop is wired end
to end (#64): progress reaches the window while a plan is committed, a step the
window refuses is re-asked with the reason in hand, and the receipt is verified
only when every step performed was seen to change the window (D-076). Each step
carries its effect into the next request, so the model is told "nothing in the
window changed" rather than assuming success. `TestLiveDriveTask` drives it
against a real application behind `-tags integration` and `VANS_LIVE_DRIVE_OK=1`.
**That run happened on 2026-09-23 and passed** — TextEdit, an empty document,
"type the word hello into the document", 5.54s. The numbers and what it exposed
are under *Proven live* and *Not proven*.

### 9.5 The next feature — a ledger, and an undo built on it

**The ledger is built (#85).** One row per committed change, written from
`Commit` because every side effect passes through it: what was asked, which tier
acted, where, the sentence the person agreed to, whether it was seen to happen,
and how to put it back. Failures are recorded too. Two hundred rows, then the
oldest go. `history` returns it and the panel shows it under Setup — "what has
this done to my machine" has an answer now.

**The undo machinery is built (#86).** `Undo(id)` reads a change back, carries
out the tier's inverse, and marks it off — only once it is actually back, because
a change recorded as undone while it is still there is a wrong answer to the
question the ledger exists to answer. Reached by `undo` over the protocol and a
"Put it back" button in the panel's record. No confirmation gate: undoing is not a
side effect anyone needs protecting from.

A restyle is the one tier with a real inverse today, so it is the one that offers
the button. Everything else says "cannot be undone", which is the honest answer
and is what the gate will use next.

**What remains:**

1. **Typing's inverse.** The prior value of the field is already in the tree
   captured before the step, so the evidence is in hand; what is missing is a way
   to put a value back. `AXUIElementSetAttributeValue` is already used in the cgo
   layer for `kAXFrontmostAttribute`, so setting `kAXValueAttribute` is the same
   call with a different attribute and an element addressed by its path. That is
   exact, where a keystroke restore would need select-all — and the key table
   deliberately holds only Delete, Enter, Escape, Space and Tab, because it is
   shown to the model.
2. **The gate learning the difference.** It treats every write the same today, so
   it has to be equally alarming about all of them. With the ledger it can say
   *this can be undone* or *this cannot*, and only be frightening about the
   second. That is the change that alters how VANS feels to use, and it is why the
   ledger was worth building first.
3. **A multi-step run's inverse**, which is the composition of its steps' inverses
   in reverse order. Worth doing only once a single step has one.

**The gap.** VANS changes things in six ways — a scripting dictionary, a page, a
window through accessibility, a web version, a multi-step drive, a restyle — and
exactly one of them can be undone. `grep -i undo` over the runtime returns
`internal/engine/webstyle.go` and nothing else. Typing into an application,
clicking a button, driving twelve steps through a task, writing a note into Notes
or Reminders: all of it is one-way.

This is not an oversight to be tidied up later, it is the project's own rule
broken everywhere but one place — *every feature ships with its undo, and VANS
knows about it*. And the store cannot help, because it keeps caches, instructions
and settings but no record of what was actually done. There is nothing to undo
*from*.

**Why it is the next thing rather than a bigger capability.** The confirmation
gate protects before the fact, and nothing protects after it. A gate makes a
person cautious; an undo makes them willing to let VANS act at all. Everything
more autonomous than a single confirmed step depends on it — a twelve-step drive
is not a thing anyone should be asked to approve without a way back. And it needs
no new model call, which the hard constraints care about.

**What it is.**

1. **A ledger.** One row per committed change: what was asked, which tier acted,
   the program that ran, what the window or page looked like before, what changed
   after, and whether it can be taken back. `uiact.Changes(before, after)` already
   computes the diff for the drive loop's own verification — it is the raw
   material, currently thrown away.
2. **An inverse per verb, where one exists.** Typing has one: the field's prior
   value is in the before-tree. A restyle has one already. A dictionary write
   often has one in the dictionary itself. A click usually has none.
3. **Honesty where there is no inverse.** A click that sent a message is gone.
   This is the part that improves the product rather than the bookkeeping: the
   gate treats every write the same today, so it has to be equally alarming about
   all of them. With a ledger it can say *this one can be undone* or *this one
   cannot*, and only be frightening about the second. That is a better
   confirmation dialog than any wording change.
4. **Reachable by asking.** "undo that", "put it back" — a route kind, and a
   button on the receipt. Undoing needs no confirmation of its own (D-100's
   reasoning): it puts things back.

**The order to build it in:** the ledger first, because it is useful on its own —
"what has VANS done to my machine" is currently unanswerable. Then typing's
inverse, which is the common case and the one with the evidence already in hand.
Then the gate learning to distinguish the two kinds of write. Clicks stay
one-way and say so.

### 9.6 The bigger one after that — seeing what cannot be read

**What Clicky does here, since it is kept installed as a reference.** Its bundle
says a lot: `Resources/CodexRuntime/vendor/*/bin/codex` is OpenAI's Codex CLI
shipped whole, and `Helpers/cua-driver` runs it in a `computer-use-compat` mode.
The model names inside the bundled Codex are the GPT-5.x family, with
`gpt-5.1-codex-max` named explicitly. Those are the models that binary knows
about, not proof of what it asks for at runtime — settling that needs the network
or its own logs.

Two things worth taking from it. It reaches the screen through a **computer-use
agent** rather than the accessibility API, which is how it works inside
applications that publish nothing — the same hole this section is about, solved
the way this section proposes. And it runs Codex with
`--dangerously-bypass-approvals --no-permissions-gate`, which is the opposite of
the confirmation gate here. That is a deliberate difference, not an oversight to
copy: it is why Clicky feels smooth, and why §9.5 argues an undo is what buys
smoothness honestly.



Chromium and Electron publish nothing to the accessibility API (§8), which is
Chrome, VS Code, Slack, Telegram, Notion and Discord — a large share of what
anyone actually uses. Browsers are covered for pages by the page tier, so the hole
is really the Electron applications, and for those a vision tier is the only path
left.

Worth being honest that it fits the hard constraints badly: it means a screenshot
to a model on every step, which is slow and costly, against *accuracy first* and
*fast model calls*. Coaching inside a web page is the cheap half of the same
want — the DOM gives exact geometry through `getBoundingClientRect`, no vision
needed — and it should be done first and separately.

### 9.7 Smaller, worth doing

- ~~Raise a window that is only covered rather than missing.~~ **Done in #84.** A
  step refused only because another application is on top now raises the window
  and tries the same step again, without asking the model a question it cannot
  answer. What is still unproven is the thing this unblocks: a posted click
  watched landing on a real control.
- **A step still advances without anyone acting on it.** This is the next thing
  to fix on the coach, and it is diagnosed rather than guessed at.

  The coach decides somebody acted by comparing the window before and after. When
  nothing about the target itself changed, it falls back to counting controls that
  came or went — `upheaval >= minimumUpheaval`, which is 2. An application that is
  still drawing produces that easily.

  A settling period was added (`settleBeforeJudging`, 900ms: whatever the window
  does to itself in that time becomes the baseline). It fixes the raise-and-settle
  case and is not enough. Measured on 2026-09-24: System Settings' Wallpaper pane
  loads thumbnails for several seconds, so the count keeps climbing long after any
  reasonable settling window, and the coach walked from step 1 to step 3 with
  nobody touching the machine.

  **The fix is to stop inferring.** The shell already watches for the real thing —
  `watch_for_the_click()` is a global mouse-down monitor that emits `coach://acted`
  when a click lands inside the ring, and it is already wired to the `advance`
  channel. The coach has a direct observation and is overriding it with a guess.
  `ActedOn` should keep the upheaval fallback for the drive loop, where VANS itself
  acted and any change is evidence, and the coach should require either the
  observed click or evidence about the target — it disappeared, it took focus, its
  value changed. A `press` step has nothing to observe and is the one case that
  still needs a fallback.

- **A failure inside a step still reads as "Something went wrong inside VANS".**
  Seen again on 2026-09-24: `engine: no part of the AXButton "Tahoe" is visible on
  screen`, under that heading. The message is accurate and the framing is wrong —
  nothing went wrong inside VANS. The codes added for the model going quiet and for
  a browser having nothing to point at (§9.5's pattern) should extend to the step
  failures a coached run can end on.
- ~~**A refused step is being re-asked and answered identically.**~~ **Fixed in
  #93.** `step 2 rejected: nothing on screen is a AXButton named "Wallpaper"`,
  twice, then the run stopped — while the window said the whole time that the name
  was on screen as an `AXRow`. It is now corrected in Go rather than asked about,
  the refusal that remains says how many of each control there are and that an
  index is wanted, and the covered-window raise waits for the raise to land
  instead of retrying into it.
- ~~**The coach showed one step as two cards.**~~ **Fixed in #95.** The caption
  beside the ring says ⌥Space means Next; ⌥Space toggled the panel, which still
  held the step it had shown. Pressing it put a second copy of the step on screen
  with its own Next and Stop. The hotkey now advances the step while the coach is
  showing, and summons the panel only when it is not (D-132).
- ~~**The coach followed you to other Spaces.**~~ **Fixed in #95.** The window
  joins all Spaces so it can draw over a window wherever that window is, and the
  same flag made it follow the person when they left. It now watches which
  application is in front and takes itself off the screen while that is not the one
  being coached, redrawing the same step on return (D-133). The panel is untouched:
  being summonable from any Space is what that flag is there for.
- ~~**⌥Space could not open the panel during a coached run.**~~ **Answered in #95**,
  by the coach taking itself off the screen. While the ring is up the hotkey
  advances the step; once the person has gone elsewhere the coach is hidden and the
  hotkey is what it always was. The mode follows what is actually in front of them
  rather than a flag they cannot see (D-132).
- **Noticing a step was done still takes up to about 1.4 seconds.** 900 ms before
  watching starts (`settleBeforeJudging`) and 500 ms between looks (`guidePoll`).
  The shell reports a click directly (D-109) and that path should make it
  immediate, so a delay this long means the direct signal is not arriving and the
  poll is doing all the work. Find out which before tuning either number.
- ~~**A drive step moved the real mouse.**~~ **Fixed in #96.** Clicking by posting
  a mouse event at the control's centre moves the actual cursor across the actual
  screen, which is the single thing that makes a runtime feel like it took the
  chair rather than pulled one up. Steps now reach their control through
  accessibility — `AXPress` for a click, asking for the keyboard for a typing step
  — and the mouse is the fallback for controls that publish nothing (D-134). This
  is also the foundation the ghost cursor needs: a drawn pointer beside a real one
  that jumps is a cartoon, and with the real one left alone the drawn one is the
  only pointer in the room. See
  `docs/specs/2026-09-26-ghost-cursor-design.md`.
- **Found in the live test on 2026-09-29, fixed in #97:**
  - *"type hello" with TextEdit in front went to a LinkedIn tab.* The router decides
    "this is about a page" from the words alone, and the page tier acted on whatever
    tab the browser had. A page action now goes to the application in front when that
    is not a browser (D-135).
  - *The panel sat on the control it was about to reach.* A step in TextEdit failed
    with "nothing is on screen" because the thing on screen was VANS. The panel now
    steps aside while an action runs and comes back with the result.
  - *Every coached run showed its first step twice.* The click was reported the
    instant it happened, the window was read before the application had moved, the
    step was written down as having changed nothing, and the model asked for it again.
    That repeat was most of the 4–5 seconds between steps (D-136).
  - *The ring vanished as soon as it was drawn.* The coach hid itself whenever the
    coached application was not in front — including when VANS itself was, and on the
    "open System Settings" step, where the application is by definition not in front
    yet.
  - *⌥Space on "open System Settings" skipped the step* and ran into an application
    that was not open. Next is now checked before it is believed (D-137).
  - *Escape did not stop the coach*, because the panel is put away while coaching and
    never heard it. The coach now holds Escape for as long as it is on screen.
  - *Errors that were not faults said "something went wrong inside VANS"*: an
    application not running, and a model saying a page cannot do what was asked. Both
    now say what they are.
  - *"Displays number 14, and there are 1"* cost a model call. A number pointing past
    the only control of that kind is now dropped without asking.
- **One model call for the whole task, #99.** A drive run or a coached run with nothing
  remembered for it now asks for every step at once, then checks each one against the
  real window as it comes up and works out the rest live from the first that no
  longer fits. Two steps cost two calls instead of three; six cost two instead of
  about seven. For the coach it is what removes the pause before each step: the next
  one is already known when the person finishes this one (D-139).
- **Still open from that test:** the model pointed at "Big Sur", a wallpaper scrolled
  out of view. It should prefer what is visible, or say to scroll.
- Draw the coach's rings and arrows over elements inside a web page, using
  geometry from the DOM. The refusal in #77 is honest but it is still a refusal.
- Persist settings and the key, so the app does not depend on the launching
  shell.
- Package a real `.app` so it does not need a checkout.
- A second look at the coach's visual design by someone who can see it.

### 9.8 Caching a drive run — the thesis reaching the tier that needs it

Every tier follows the thesis except one. Extraction programs are cached by page
signature, routing decisions by instruction shape, dictionary actions and
single-step window actions by `(app, shape)`. A warm run on any of them costs
zero model calls.

The drive loop caches nothing. `stepThroughTask` calls `WriteStep` once per step,
once more to hear "done", and again on every refusal. A six-step task is about
seven calls, every time, forever, even when the task, the app and the window are
identical to yesterday's. Multi-step work is where VANS is heading, so this is the
tier where the thesis matters most and the one place it does not apply.

**The idea.** A drive run that finished and landed every step *is* a program. The
model wrote it a step at a time, but what came out is an ordered list of
`uiact.Step` that got the task done in that app. Keep it under `(app, shape)`, the
same key `window_actions` uses, and replay it with no model involved:

    window_actions:  (app, shape) -> 1 step    validated against one reading
    drive_traces:    (app, shape) -> n steps   each validated against a fresh one

**Why it does not lower the accuracy floor.** The trace is a suggestion, not an
authority. It passes the same checks a fresh answer passes: the window is captured
again before each step, the step goes through `uiact.Validate` against *that*
tree, the occlusion guard and the covered-window raise apply unchanged because
replay calls the same `handle`, and `describeEffect` must report a change
afterwards. A replayed step that changed nothing is a divergence, not a success.
That the steps came from the cache is never itself the evidence.

**Divergence resumes, it does not restart.** If replayed step *k* fails validation
or lands without changing anything, the live loop picks up at *k* with `Done`
already holding steps 1..*k*-1 and their effects — exactly the state it would have
been in. Only the diverging tail costs calls. A run that diverged and then
finished live produces a new full trace, which replaces the old one. Two
divergences running and the entry is dropped (`MaxDivergences`), because by then
the trace is wrong about the task rather than unlucky.

**What makes a trace cacheable.** All four: the model said `Done` rather than the
run hitting the step limit; `Landed` was true for every step; every literal from
`route.Shape` was lifted back out into its placeholder so `Fill` can rebuild it;
and it carries no content the model composed. The last is the one that matters —
replaying a sentence the model wrote for a different request would be wrong in a
way no check downstream catches, so such a run is not cached and the log says why.

**Where a literal will not lift out.** `uiact.Baked` ignores literals under three
characters because it looks for one *inside* a longer string, where blind
substitution would corrupt the step. But a step whose text is *exactly* the
literal can be swapped whole at any length, and that is the common case. What is
left — a short literal buried inside other text — is stored *pinned* to the values
it was recorded with, and answers only for those. Narrower than a templated trace
and still free for asking the same thing twice.

**Measured, not asserted.** The receipt and the progress lines say how many steps
came from the cache and how many model calls were made, so warm and cold are
visible in the panel rather than claimed here.

**Beyond the cache — the other ways to spend fewer calls.** Caching only helps the
second run. These help the first:

1. **Repair a refused step instead of re-asking.** A refusal costs a second call
   today, and most refusals are mechanical: "nothing on screen is a AXButton named
   Wallpaper" when an `AXRow` of that exact name is right there. `nearMiss` already
   works the right role out in order to word the message — using it to correct the
   step costs no call at all. This is §9.7's "refused step answered identically",
   solved from the other end.
2. **One call for the whole plan.** Ask for the entire sequence upfront instead of
   a step at a time. It is the same machinery as replay: a speculative plan is an
   uncached trace, so it validates per step and resumes live where it diverges.
   Steps past the first are guesses about a window the model has not seen, so it
   will diverge on branchy tasks — and lose one call when it does, against saving
   six when it does not.
3. **Drop the "done" call.** One call per run exists only to hear that the task
   finished. Let a step be marked as the last one instead. The engine still
   verifies by evidence rather than by the model's word, so this changes what stops
   the loop, not what counts as success. A warm replay needs none of it: a trace
   knows its own length.
4. **Provider-side prompt caching.** Every step call resends the same system
   prompt, schema and task, and only the window tree changes. This cuts time per
   call rather than the number of them — and whether the OpenAI-compatible Gemini
   endpoint honours it is a thing to check, not to assume.

**Out of scope.** Vision has no accessibility tree to key or validate against
(Q-15), so nothing from that tier gets cached this way. Fuzzy shape matching stays
out: exact `(app, shape)`, like every other cache here.

**Done when** the same TextEdit task run twice shows the second run at zero model
calls with a verified receipt, and both numbers are in *Proven live*.

**Built so far:** the store (`drive_traces`, #87), recording (#88), replay (#89)
and the counts (#90). A repeat of a known request now asks the model nothing at
all, and the panel says how much of a run came from last time and how many calls
it made.

**Measured, on 2026-09-26.** `TestLiveDriveTaskTwice` (#92) runs the same task
twice against a real window with a real model, on a store of its own so the first
run is genuinely the first. TextEdit, "type the word hello": cold 3 model calls in
6,052 ms, warm **0 calls** in 866 ms. The numbers and what they say are under
*Proven live*. To run it again — TextEdit has to be open with an empty document,
and the run types into it twice:

```bash
VANS_LIVE=1 VANS_LIVE_DRIVE_OK=1 VANS_LIVE_APP=TextEdit VANS_LIVE_TASK="type the word hello" go test -tags integration -v -count=1 -run TestLiveDriveTaskTwice ./internal/engine/
```

**The clock moved more than expected, and the prediction here was wrong.** This
section used to say a warm run would be faster but not dramatically, because the
window reads a replay still has to do would dominate a short task. The measurement
says otherwise: 6,052 ms to 866 ms on one step. Reading and acting on the window
takes well under a second; a model call takes about two. So the calls were most of
the wall time even on the shortest task there is, and skipping them is most of the
saving.

The call count is still the number to assert on, because it is exact and does not
move with the network. But the clock is not the disappointing measure it was
predicted to be.

**Then** the four call-saving items above, which help the first run rather than the
repeat.

### 9.9 Three calls for one step — fixing it without asking again

The live measurement of the step cache (§9.8) showed a one-step task costing three
model calls: one to work the step out, one because the first answer was refused,
one to hear the task had finished. Only the first is work. **#93** takes out the
second.

**The raise was being retried into.** When another application's window is over
the one being driven, the step is refused before it can click the wrong thing, the
window is brought forward, and the step is tried again. It failed again, because
`waitForWindow` asks whether the application *has a window on screen* — and a
covered window has one. So it returned the instant it was asked, while the raise
was still happening, and the second refusal spent a model call. It now also waits
for the application to actually be in front. Best effort, not an error: whether the
target is really reachable is settled by `VerifyTargets` against the window, and
this only gives the raise time to happen.

**A wrong role is corrected, not asked about.** The model asks for the `AXButton`
"Wallpaper" and System Settings has an `AXRow` of that name. `nearMiss` already
works the right role out in order to word the refusal, and then the step was
re-asked and came back identical — twice, live. `uiact.Repair` now rewrites the
role when the name matches exactly one control and only the role is wrong. Several
matches is a question about which one was meant, and guessing that is how something
gets clicked that nobody asked for, so that still goes to the model. A corrected
step is trusted no further than an asked-for one: it goes through `Validate`,
`VerifyTargets` and the read-back exactly as it would have.

**What is left to ask about is worded for answering.** With the unambiguous case
handled in Go, what reaches the model is a name matching several controls. The
refusal now says how many of each there are and that an index is wanted, rather
than only describing the problem.

A corrected step on a replayed run counts as a divergence, so the stored sequence
is replaced by what actually worked — a trace that has drifted repairs itself
instead of being thrown away.

### 9.10 What is big, next — in order

Ordered by what would change most about using VANS, not by effort. The first two
are the ones worth arguing about.

**1. Undo for the tiers that have none, and a gate that knows the difference.**
The project's own rule is that every feature ships with its undo, and one tier of
six has one. `grep -i undo` over the runtime still finds `webstyle.go` and nothing
else. Typing into a window, clicking a button, driving twelve steps, writing into
Notes: all one-way. Typing's inverse is already half-built and parked on a branch —
the prior value is in the tree captured before the step, and
`AXUIElementSetAttributeValue` is the same call already used for
`kAXFrontmostAttribute`.

Then the gate. It is equally alarming about everything today, so it has to be, and
that trains people to dismiss it. With the ledger it can say *this can be put back*
or *this cannot*, and only be frightening about the second. That is the change that
alters how VANS feels rather than what it can do, and it is why the ledger was
built first.

**2. Something that can be installed.** VANS needs a checkout, a terminal, and
environment variables inherited from the shell that launched it. Settings and the
key do not persist. So it cannot be used without a terminal open, and nobody else
can run it at all. Everything else on this list is a better version of a thing that
only one person on one machine can start.

**3. One model call for the whole task.** The remaining big lever for cold runs:
ask for the entire sequence up front instead of a step at a time. It is the same
machinery as replay — a speculative plan is an uncached trace, validated per step,
resuming live where it diverges — so most of it already exists. Steps past the first
are guesses about a window the model has not seen, so it will diverge on branchy
tasks and lose one call when it does, against saving several when it does not.
Together with dropping the finished-call, a one-step task goes from three calls to
one.

**4. Seeing what has no accessibility tree.** The ceiling on what VANS can do at
all. A canvas, a game, an application that publishes nothing useful — the ladder
has no rung for it. The cheap half first and it needs no vision model: draw the
coach's rings over elements inside a web page using geometry from the DOM. The
refusal in #77 is honest and it is still a refusal.

**5. A task that spans two applications.** The chooser picks one application and
the run happens there. "Put this in Notes and then message it to him" is two, and
it is a normal thing to ask for. This is a capability that does not exist rather
than one that is slow.

**6. A run that stopped halfway.** Six steps, dead at four: the world is
half-changed, nothing says so beyond a log line, and there is no way back. Needs
the inverses from item 1 before it can mean anything.

**7. Re-verify the coach advancing without anybody acting.** The mechanism was
fixed in #82 by waiting for the click the shell already sees, and the live flow was
never run again to confirm it end to end. A known-unverified claim, and small.

**8. Longer tasks.** `MaxDriveSteps` is 12 and `MaxSteps` is 8, both chosen when
every step cost a model call. A warm run costs none, so the limits are now about
patience rather than money and could be raised on the strength of a trace.

**Also worth doing, smaller:** provider-side prompt caching, so the unchanged part
of each step prompt is not resent — that cuts time per call rather than the number
of them, and whether the OpenAI-compatible Gemini endpoint honours it needs
checking rather than assuming.

### 9.11 The ghost cursor — and the order it changes

Full spec: `docs/specs/2026-09-26-ghost-cursor-design.md`. The direction and the
reasoning behind the ordering live in `docs/ideas.md`, which is a local working
file and deliberately not in the repository — so what matters from it is repeated
here rather than referred to.

**Built so far:** the foundation, in #96 — steps reach their control through
accessibility and the real pointer is left alone. Nothing is drawn yet, and it
has not been run live: a class of control that quietly ignores `AXPress` would
show up there first.

**Drawn, for the coach, in #98.** A glowing rounded triangle that glides along a
curve to each step, faces the way it is going, and presses with a ripple as it
lands (D-138).

**And for runs where VANS acts, in #100.** Each step is announced just before it is
performed: the ghost glides to the control, lands, and about 850 ms later the step
happens. Escape during that pause stops the run with the step not done — the
approval stops being a dialog and becomes a moment you let pass (D-140).

**What it is.** A drawn pointer glides to a control and lands on it before
anything happens to that control. Per step on a drive run: the ghost arrives, the
control gets a ring, a short pause, then the action. It is the base for the rest of
the on-screen work — hand-drawn marks, narration captions, the step trail — because
each of those hangs off knowing where the ghost is and when it got there.

**The decision it turns on: the real cursor must not move.** A drive step clicks
through `CGEvent` today, which takes over the person's mouse, and that is what makes
an agent feel like it pushed somebody out of their chair. The accessibility API can
press a control directly — `AXPress`, and setting a value for typing — without
moving the pointer, and on a window that is not even in front. So act through
accessibility, draw the ghost to show where a mouse would have gone, and leave the
real cursor where its owner left it. `CGEvent` stays as the fallback for controls
that publish no action.

That is what makes the ghost honest rather than decorative: it is not a cartoon of
the click, it is the only pointer involved.

**It is also a gate that costs nothing.** The pause between landing and acting is a
veto — Escape during it cancels the step. The tension `ideas.md` names is the
confirmation gate against smoothness, and Clicky is fluid because it skips
approvals. This is the answer to that: an approval stops being a dialog to dismiss
and becomes a moment to let pass, per step, and only where something is about to be
touched.

**What it needs.** The overlay is mostly there — `coach.html` is already
transparent, click-through, on every Space, and converts screen points into window
coordinates (`cover()` in `coach.rs`). The engine-side gap is that `runDrive` has no
way to announce a step before performing it: `RunGuide` takes a `show` callback and
the drive path has nothing equivalent. That notification, a wait, and Escape wired
to the run's context is the whole of it.

**The hazard to test early.** Accessibility points are global across displays, so
the overlay needs per-display coordinates and the right backing scale factor. A
ghost that glides to the wrong monitor is worse than no ghost.

**Consequences to plan for rather than discover.** An `AXPress` does not care what
is on top of the window, so the covered-window guard (D-112) means something
different for AX-acted steps — being covered still means the person cannot see what
VANS is doing, so it should not simply be skipped. "Landed" is still read back from
the window (D-004), unchanged.

**Not in the first version:** the coach (it already points and the person clicks),
hand-drawn marks, captions, the step trail, and pausing when the person grabs the
real mouse.

### 9.12 Where the order stands now

§9.10 was written from the engineering side. `docs/ideas.md` reorders it from the
product side, and the product order is the one being followed. The change that
matters: shipping something installable is ranked above undo, on the grounds that
nobody but one person can run VANS today, so there is no feedback to learn from.

1. ~~Measure the warm drive run live.~~ **Done 2026-09-26**, numbers under *Proven
   live*: cold 3 calls / 6,052 ms, warm 0 calls / 866 ms.
2. ~~**One model call for the whole task**~~ **Built in #99.** (§9.10 #3). The model call and its prompt
   are written and parked on `one-call-plan`; nothing is wired up. A plan made up
   front is an uncached trace, so it validates per step and resumes live where it
   diverges — the machinery from #89 is most of it. One caution that the product
   note does not make: a *stored* trace is only kept for a run the model called
   finished, so running out of it means the task is done (D-122). A plan guessed
   this run promises nothing of the kind, so running out of it is not evidence and
   still costs the one confirming call. Cold one-step task: three calls to two.
3. **Ghost cursor** (§9.11).
4. **Voice, local.** Hold ⌥Space to talk, live transcript, coach steps read aloud.
   On-device speech to text; the intent is text by the time the engine sees it, so
   it touches nothing here.
5. **Something installable.** Settings that persist, the key in the Keychain, a
   signed `.app`.
6. **Undo for typing, and a gate that knows the difference** (§9.10 #1). Half built
   on `put-typing-back`, committed locally and unpushed.

Later, unchanged: tasks across two applications, the vision ladder, raising the step
limits now that a warm run is free.

**A tension to settle deliberately.** `docs/ideas.md` proposes revisiting D-032 —
that VANS never launches an application — on the grounds that doing tasks on
someone's behalf eventually means opening things, and that launching belongs behind
the gate rather than banned. #94 has just made the rule *stricter* by deleting the
fallback that opened applications by accident. Those are not in conflict yet:
removing an accidental launch is not the same as refusing a deliberate one. But if
launching is ever allowed, it should arrive as a gated action with the ghost
cursor's pause in front of it, not by restoring a fallback. Worth deciding rather
than drifting into.

### 9.13 Six fixes from the first live coach test (2026-10-01)

Picked over voice after the live test in #97 showed the coach itself was the weak
spot. In order:

1. **Faster coach steps.** It listens for the window changing instead of
   re-reading it on a timer, and calls a window settled once it has been quiet for
   250 ms instead of always waiting 900 ms (D-141). **Built.**
2. **The coach remembers.** A coached question that finished is kept, and asked
   again it costs no model calls (D-142). A guessed plan that goes wrong is planned
   again in one call rather than walked a step at a time (D-143). **Built.**
3. **Controls that are off screen.** The "Big Sur" run pointed at a row scrolled
   out of view. The model is now told which controls are out of view, a drive run
   scrolls to them first, and the coach asks the person to scroll (D-144).
   **Built.**
4. **Captions** next to the ghost saying what it is about to do. In drive runs the
   caption replaces the tip card (D-145). **Built.**
5. **A step trail** showing the steps already done: numbered dots joined by a
   dotted line (D-145). **Built.**
6. ~~Pause when the person touches the mouse.~~ Built in #104 and **withdrawn
   after the first live use** (D-156): it stopped runs nobody wanted stopped. Only
   Escape stops a run.

All six are built (#101–#104) and none has been tried live yet. What to watch
for in the first live run: whether the application accepts the change observer
(the log says "cannot hear … change" if not), whether `AXScrollToVisible` works in
System Settings, and whether a pause fires when nobody touched anything.

Voice, the installable app and undo for typing (§9.12 #4–6) come after these.

### 9.14 Multi-step tasks everywhere (proposed 2026-10-01)

Asked for after the first live run of §9.13: booking on a travel site, uploading
to YouTube, sending a WhatsApp message, coaching inside Lightroom. Spec in
`docs/specs/2026-10-01-tasks-everywhere-design.md`. The short of it: what decides
the work is how VANS can see an application, and there are three kinds.

1. **Web pages** — multi-step is missing. Give the step loop a screen that reads
   and acts on the page. Runs in the Chrome profile already open, which is the
   default today; "in a new profile" in the request picks the throwaway one.
   Stops before pay, publish, send. Asks when a choice is the person's.
2. **Readable Mac apps** — the loop works; coverage is unknown. A probe, a sweep,
   and WhatsApp end to end.
3. **Unreadable apps** (Adobe, Electron) — a vision tier, coach only at first.

Then voice, then background runs. This replaces the order in §9.12 from #4 on:
voice moves behind the three above.

**Progress.**

- *Web pages, multi-step — built.* A page is read as a window
  (`internal/webscreen`), so the step loop, the coach, re-planning, the trace
  cache, scroll-to, the ghost and the pause all work on it unchanged (D-147). A
  named site that is not in front is opened as the first thing a confirmed run
  does, and its address is never guessed (D-148). Not yet tried on a real site.
- *First live run (2026-10-01):* "search for flights from delhi to goa on
  makemytrip" opened the site, clicked Flights, picked the route and was kept. Two
  steps, three model calls, and it felt slow. A page read measures 0.1 s; the time
  was calls and fixed waits. Fixed in D-149 (the planner carries on when a plan runs
  out) and D-150 (waits end when the page answers).
- *Stop points — built.* A step that pays, orders, publishes, sends or deletes
  waits for ⌥Space (D-151). Secret fields are never typed into (D-152).
- *Readable Mac apps, first one end to end (2026-10-01):* WhatsApp. Four bugs on
  the way, each fixed: the router declined one wording (D-153); the application's
  name starts with an invisible mark, so it was "not running" (D-154); the one-go
  program ran before the step loop and stopped after opening the chat (D-155); and
  the last check before acting tripped on the same mark (D-156).
- *The wording decides, and the switch is back (D-158, D-160).* "Show me", "how
  do I", "teach me" are coached; "send", "book", "put" are done. The switch
  follows the words until it is clicked, then it is a preference that clear
  wording still overrides. Every task says which it chose and offers the other.
  Moving the mouse no longer pauses anything (D-156).
- *The coach offers to enter values (D-159).* On a typing or value-picking step,
  ⌘↩ has VANS do that step and Escape leaves it to the person.
- *A probe, and the first sweep (D-163).* `go run ./cmd/vans-probe [app …]` says
  how usable an application is. WhatsApp and Notes are ready, Clock partly,
  Electron applications blind. Rows are now named after the text inside them.
- Next: run the probe across the rest of the installed applications, fix what is
  "partly", and start the vision tier for what is blind.
- *Questions mid-task (D-164), a separate profile or private window by saying so
  (D-165), and a coach that gives smaller steps and does nothing itself (D-166) —
  built, not yet tried live.*
- Known limits of this first cut: clicks and typing are sent through the page's
  script, which some sites ignore for things that need a real gesture (file
  pickers, some pay buttons); frames from another site cannot be read; only
  Chrome.

### 9.15 The to-do log (written 2026-10-01)

Everything agreed as still to do, in one place. Numbers are for referring to, not
a strict order; the order being followed is at the end.

**A. Fewer model calls.** More steps must not mean more calls. Calls happen when
the screen becomes one the model has not seen, so the measure is calls per screen.
No local model runtimes (§11 still holds); these come first.

- A1. A count of calls on every task, shown, so a drop can be seen. **Built**
  (D-167): `go run ./cmd/vans-probe -tasks`.
- A2. **Built (D-194) as hints, not as stored pieces.** Remember pieces, not only whole runs: "open the chat with a person", "search
  on this site", kept and reused inside new tasks.
- A3. **Built (D-167, and lists generalise, D-168), not yet measured live.** Learn each application's map: clicking X here leads to a screen with these
  controls. Lets the planner plan across screens it has not been shown this run.
- A4. **Built: scrolling, waiting, mending a name, and cookie notices (D-196).** Small decisions made by code, never by a model: scrolling to a control,
  closing cookie banners, waiting for a page, picking the field whose label fits.
- A5. **Built for two cases (D-195).** No "is it done" call when the screen proves it (the text asked for is in the
  field). Keep the call where code cannot tell.
- A6. **Built (D-181), off until `VANS_LLM_FAST_MODEL` is set.** A cheaper, faster model for the easy calls; the stronger one for planning.

**B. Reaching every application.**

- B1. **Built (D-169), and demoted to a last resort by D-183.** Tried live it was
  slow (7 s) and wrong about an icon. Electron applications are now asked to
  publish their controls and are read exactly; a picture is only used when that
  produces nothing. The vision tier, chosen automatically when the probe's verdict
  is blind.
- B2. **Built (D-170).** Real mouse and keyboard as the fallback on pages that
  ignore scripted input.
- B3. **Built (D-197), not yet tried against a real file chooser.** Choosing files, for uploads.
- B4. **Built (D-198), not yet tried live.** Tasks across two applications.
- B5. **Safari built (D-180), not tried against Safari itself.** Safari and other browsers.
- B6. **Menu bar built (D-179); volume, mute, appearance and Wi-Fi set directly (D-200); other system controls are not.** The menu bar and system controls.
- B7. **Built (D-199), not yet run on a real folder.** Files and Finder tasks.
- B8. Several displays, tested.

- B9. **Built (D-206, D-207), not yet tried live.** More kinds of step: double-click,
  right-click, hover, drag, scroll, set a slider, wait.

- B10. **Built (D-209), checked on a sample page only.** Pages inside pages, and a
  click that leaves for another site.
- B11. **Built (D-210), not yet tried on a real window.** Reading what a window shows
  to answer a question about it.

**C. Voice.** C1. Voice in (hold ⌥Space and talk). C2. Voice out (steps read
aloud).

**D. Trust.**

- D1. **Built.** An activity log a person can read, with undo where there is one. Multi-step runs now record the steps taken (D-175).
- D2. **Built (D-175).** Permissions per application ("never act in my bank's").
- D3. **Spending built (D-175); ask-before-every-step per application built (D-181).** Limits on spending and sending that cannot be crossed.
- D4. **Built (D-176).** A dry run: what would be done, without doing it.
- D5. **Built (D-171).** Recovery when a task breaks partway: say where it got to, offer to carry on
  or back out.
- D6. **Built (D-203), not yet tried on a real field.** Undo for typing.
- D7. **Built (D-172).** Notes kept locally of what failed, so general faults surface from use.

**E. Smarter.**

- E1. **Built (D-177): told, not inferred.** Remembering the person: usual airline, home city, who "mom" is.
- E2. **Built (D-173, D-174): application, window and selected text, sent only when the request points at them.** What is on screen as context: "reply to this", "book the one I am looking
  at".
- E3. **Built (D-178).** Long tasks with checkpoints.
- E4. **Built (D-178).** Saved routines ("do my Monday report").
- E5. **Built (D-205): a task is brought up at its time and waits to be told to go.** Scheduled tasks.
- E6. **Built (D-214), not yet tried live.** "Yes to all" for value entry in a
  coached run. The offer is back on: this one, the rest like it, or no for the
  whole run.
- E7. **Built (D-192).** Working on text: translate, rewrite, summarise or reply to
  the selection, given back as words with a Copy button.
- E8. **Built (D-213).** The remembered row: the tasks that finished, out of the
  three that are kept, each forgettable from its chip.

**F. A product.**

- F1. Something installable and signed, with the key in the Keychain. **Parked on
  2026-10-03 until testers are close.** The outline agreed: a compiled app with the
  repository kept private; an updater so a published build reaches testers without a
  reinstall; no paid Developer ID for friends, who allow it once in System Settings;
  every build signed with one self-signed certificate so the Accessibility permission
  survives an update, which has to be proven on this macOS before anyone gets a
  build. Open: where release files are hosted, and when the Developer ID is worth
  paying for.
- F2. **Built (D-201).** First-run setup without a terminal.
- F3. **Built (D-201, D-202).** A choice of model, and what each task cost.
- F4. Background runs. Last on purpose: not safe before D1.

**Order being followed:** A1–A3 now; then B1; then C; then F1; with D5 and E2
pulled forward when there is a gap, because both are small and felt every day.

---

## 10. Running it

```bash
# 1. Chrome with a separate profile, so a real session is untouched
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 --user-data-dir=/tmp/vans-chrome

# 2. Build the runtime the shell spawns — without this the app does not open
go build -o ui/vans-engine ./cmd/vans-engine

# 3. Point it at a model
export VANS_LLM_API_KEY=...
export VANS_LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
export VANS_LLM_MODEL=gemini-flash-lite-latest

# Optional: a second model for when the first cannot answer, and how long one
# request gets before VANS stops waiting for it (45s by default)
export VANS_LLM_FALLBACK_MODEL=models/gemini-3.1-flash-lite
# Optional: a quicker model for the two easy calls (what kind of request, which app)
export VANS_LLM_FAST_MODEL=...
export VANS_LLM_TIMEOUT=45s

# 4. Run it
cd ui && npm install && npm run tauri dev
```

Summon with **Alt+Space**. There is no dock icon by design, and the hotkey fails
silently if another application already owns the combination — the log line
`hotkey Alt+Space registered = false` is how you find out.

Without the GUI:

```bash
go run ./cmd/vans -url https://news.ycombinator.com \
  -intent "the title and link of each story"
```

Checks that must pass before anything ships:

```bash
go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...
cd ui && npx tsc --noEmit
```

---

## 11. Standing constraints

1. **Accuracy above all.** Prefer a deterministic floor the model cannot lower
   over a prompt asking nicely. When in doubt, refuse rather than guess.
2. **Minimum permissions, no data leak.** Ask for as little as possible, and
   send as little as the request actually needs.
3. **Speed is a feature.** The warm path is zero model calls; when a call does
   happen it should be a fast hosted model.
4. **No local model runtimes.** Hosted only. One exception, decided
   2026-10-14: macOS's own text recognition may run on the Mac to find words
   and their exact places in applications that publish nothing (D-231).
5. **VANS never launches applications.** It acts on what is already open
   (D-032).
6. **Every side effect passes a gate.** Reading changes nothing and needs no
   confirmation; writing does. A gate in front of a harmless action teaches
   people to dismiss gates.

---
---

# Part II — Working guidelines

How work on VANS is done. Every rule here was stated directly at some point,
and most were stated because the opposite had just happened. They hold for
every change unless a later instruction says otherwise.

## G1. Code

- **No comments in code.** No block comments explaining rationale, no doc
  comments on functions, no inline asides. Code should read on its own. When
  something genuinely needs explaining, it goes in `docs/decisions.md`.
  - New code ships without comments.
  - Editing a file that already has comments: keep the existing ones, add none.
- **The engine is Go.** Runtime logic belongs in the Go engine, not in the panel.
  The panel is TypeScript only because it is a web view; it stays thin and asks
  the engine for everything it shows (destinations, suggestions, capabilities).
- **Programs are data, never model-written code.** A model chooses from what the
  runtime makes expressible; the runtime renders it deterministically.
- **Prefer a closed set to a free string.** When a model output can be an enum,
  make it one. The restyle tier's emphases are the model for this: nothing to
  validate because nothing free-form is accepted.
- **Temporary probe code is never committed.** Diagnostics, debug triggers and
  throwaway tests are removed before the commit, and live-machine probes run
  from outside the repo where possible.
- **Marshal to the main thread yourself.** Anything touching AppKit windows
  must not trust its caller to already be on the main thread.

## G2. Git, branches and PRs

- **Work on branches.** Nothing lands on `main` directly. One branch per piece
  of work, named for what it does.
- **A PR for every new feature and every fix.** Separate concerns get separate
  PRs; a finished, self-contained fix ships on its own rather than waiting for
  the next feature.
- **PR descriptions are brief — about 20 lines, never more than 30**, however
  large the change:
  - One line saying what it does.
  - Bullets for changes, one sentence each, no sub-bullets.
  - A table for bugs: `bug | cause`, the cause in one sentence.
  - One line of verification and a pointer to the decision records.
  - No block quotes, no pasted output, no section headers.
  - Anything that feels too important to cut belongs in `docs/decisions.md`.
- **Commit messages may carry more reasoning** than a PR body, but nothing that
  does not change a reader's understanding.
- **Merge your own PRs and keep going.** Standing permission since 2026-09-12:
  open it, verify it, merge it, move on to the next piece without stopping to
  ask.
- **After a change, update the PRs** it affects.
- **Do not delete the dead branches** until explicitly told to.
- **Everything in the repo reads as written by its developer.** No attribution
  lines, footers or co-author trailers in commits, PR titles, PR bodies,
  comments, docs, issues or branch names — this overrides any tool default that
  appends one. Reread the exact text before every commit and every PR
  create, edit or comment, and strip anything of the kind.
- **Keep tool-specific instruction files and config directories out of the
  repo.** Working rules live outside it.

## G3. Testing and verification

- **Verify with exit codes, never with `&& echo OK`.** That once printed a
  success line over a failing build. Use `if cmd; then ...; else ...; fi`.
- **The full gate before anything ships:**
  ```bash
  go build ./... && CGO_ENABLED=0 go build ./... && go vet ./... && go test ./...
  gofmt -l internal/ cmd/        # must print nothing
  cd ui && npx tsc --noEmit
  cd ui/src-tauri && cargo build  # when the shell changed
  ```
  The no-cgo build is checked explicitly — it broke for two PRs once because
  `go build ./...` does not cover it.
- **CI checks every pull request and every push to `main`** (D-230):
  formatting, vet, vet of the Mac build and every Go test; the panel's types
  and build; the site's build and form tests. A pull request is merged only
  after all three jobs pass. Watch for "no checks reported" straight after a
  push: the checks have not started yet, and merging then skips them. The
  Rust shell is not built in CI; it still needs `cargo build` here when it
  changes. "Workers Builds: vans" is Cloudflare's preview of the site for the
  branch and has failed on every pull request since at least #175; the live
  site builds from `main` and is unaffected.
- **Tests do not read the machine they run on.** CI runs on Linux, which has no
  Applications folder and no window in front. A test that needs installed
  applications gets a pretend folder (`aMac` in the engine's tests).
- **Hermetic tests for everything; live tests behind build tags.** Anything
  that touches a real window, browser or application runs only under
  `-tags integration` with an explicit opt-in (`VANS_LIVE=1`), and anything
  that types or clicks needs a *second* opt-in, so a test never acts on
  whatever happens to be on screen.
- **Measure claims as assertions.** "Warm runs cost zero model calls" is a test
  that counts calls, not a sentence in a README.
- **Seams for the machine.** Display bounds, hit tests, window ownership and
  window presence are variables tests can replace, so hermetic tests never
  depend on the monitor or the desktop they run on.
- **Check the result against reality, not against the code.** After a live
  change, read back what actually landed — the computed style on the page, the
  window list, the text in the field.
- **Never bypass the guard in a probe.** A click posted without the
  verification in front of it landed on the wrong control and closed an
  application. A probe that skips the safety check is not a test of the system.
- **Do not experiment on the user's live applications.** Ask for a target that
  is safe to touch. Never close, quit or disrupt something the user has running.
  Leave applications out of it when asked to.
- **Test it out, or say plainly that it was not.** Distinguish "verified live",
  "verified by tests" and "not verified" every time. When testing is deferred,
  keep a list and give a heads-up about what needs trying.

## G4. Documentation

- **`docs/decisions.md` records every design decision**, with the context, the
  decision, the tradeoffs and what it rules out — so none of it lives only in a
  session. Numbered `D-001` onwards.
- **`docs/progress.md`** records milestones and progress as they happen.
- **`docs/backlog.md`** records work that is decided but not built, with why it
  is not done yet and what has to be true before it can be.
- **`AGY.md`** (this file) is the state of play and the working rules.
- **`README.md`** says what VANS is and what it is becoming.
- Long content goes in a file; conversation gets a short summary and a path.

## G5. Communication

- **TLDR by default.** Lead with the answer in a few lines. Expand only when
  asked for details, reasoning or tradeoffs.
- **Simple language.** "The app was not running", not "the process was not
  resident". Jargon only when it is the actual identifier in the code.
- **No bloated answers.** Do not restate what is already known, list options
  nobody asked for, or narrate work as it happens — but always say the one
  thing that changes what happens next.
- **Walk through the architecture as it is built,** briefly and visually: a
  small table or an ASCII diagram beats a paragraph.
- **When asked "what are we doing now", answer that first,** then continue.
- **Say what is wrong plainly.** When something broke, or something was
  claimed that turned out false, say so directly and correct it — no hedging,
  no defending the earlier claim.
- **The developer sometimes writes the code by hand** to learn it, and
  sometimes wants it written. Infer which from the request, or ask in one line.
  Either way supply the traps and a way to check the result, and read a file
  before overwriting it — a partial hand-written version is worth diffing.

## G6. Product rules — what VANS must and must not do

**The five hard constraints** (set 2026-09-10):

1. **Accuracy above all.** A deterministic floor the model cannot lower beats a
   prompt asking nicely. When in doubt, refuse rather than guess.
2. **CSS injection into web pages** is a core capability: given a description,
   restyle a live page.
3. **Context matters.** What is on screen should inform what VANS does, not
   just sit in a banner.
4. **Model calls must be fast.** The warm path is zero calls; when a call does
   happen it is a quick hosted model.
5. **No local processing.** No local model runtimes. Hosted models only — free
   and open-weight where possible.

**And the rest:**

- **Minimum permissions, and no data leak** — so that people can trust it. Ask
  for as little as possible. Send only what the request actually needs: acting
  on a window sends roles and labels, never contents; restyling sends
  structure, never text.
- **Never launch applications.** Work with what is already open (D-032). When
  something is not running, say so and name the command.
- **Use the browser the user is signed into**, but **never take over the tab
  they are working in.** Work in a new tab.
- **Honour the chosen destination.** When the panel is selected, the answer
  goes in the panel — not a page load somewhere else.
- **Actions happen on screen, where they can be watched,** so that when
  something goes wrong it is visible at the moment it happens.
- **Not only native applications.** Everything that can be reached — native
  apps, Electron apps, web apps — with the fewest permissions possible.
- **Know what it did.** After acting, read the window back and say what changed
  rather than claiming success.
- **Every side effect passes a confirmation gate; reading does not.** A gate in
  front of a harmless action teaches people to dismiss gates. Undoing
  (clearing a restyle) is not a side effect and is not gated.
- **Every feature ships with its undo, and VANS knows about it.** A capability
  added to the runtime must appear in what VANS says it can do and in its
  suggestions — a feature the panel never mentions does not exist for the
  person using it.
- **The ladder for acting: cheapest reliable path first.** Scripting
  dictionary, then the application's web version, and the accessibility tree
  only where neither reaches. Fast first, thorough as the fallback.
- **Remember the last three instructions** and offer them back.
- **Hands-on findings (D-221).** Menu items, table rows, targets, file paths
  and applications named in a request are handled; the file window is open.
- **The guide never brings an application forward (D-219).** Opening it and
  switching to it are steps the person does. Leaving hides the overlay and the
  panel, and the step is held until they are back.
- **A run the planner cannot start is turned down, not offered (D-218).**
- **What is typed keeps the capitals it was asked with (D-217).** A request is
  lowercased only to recognise its shape; names and text go out as written.
- **Where a kind of task is done is remembered (D-226).** A guide shown
  before asks the model nothing; only one that finished is remembered, and only
  when it was not in the application in front.
- **Every key and click VANS posts states its modifiers outright (D-225).**
  Left unset they inherit the last shortcut's, and typed text becomes shortcuts.
- **Slow steps are the model provider being overloaded (D-224).** Retries are
  logged as `[model]`, and the guide says when the model is slow. The fix is a
  provider that is not overloaded, which is a cost decision.
- **The model is asked through the VANS server (D-229).** With
  `VANS_SERVER_URL` and `VANS_INVITE` set, the app carries no key and builds
  no prompt; `cmd/vans-server` does, behind invite codes and daily allowances.
- **A second provider is asked when the first is overloaded (D-227).** Set by
  four `VANS_LLM_BACKUP_*` settings; with one set, the first provider gets one
  retry per model instead of three before the backup is asked.
- **A step is acknowledged the moment it is noticed (D-223).** The ring ticks
  at once; the wait for the next step comes after, not before.
- **Name:** VANS. The full form is Visual Agent Navigation System, and it is
  used only in official documents: this file, the README, the privacy policy
  and terms. The product, the panel and the site's pages say VANS and do not
  spell it out. It grew out of "Vedansh's Agent Navigation System", which is
  kept on record as the origin and no longer used (D-222).

## G7. UI and design

- **A consumer product, not developer tooling.** It should look like something
  a person would buy, not a debugging console.
- **Apple design language.** System font, a clear type scale, a consistent
  spacing rhythm, restraint.
- **The v3 design is the panel and the guide (D-212).** One field at the top,
  a foot with destinations and key hints, volt for VANS and red only for what
  changes something. Do not revert to an earlier design.
- **Summoned, not launched.** Alt+Space, no dock icon. It opens **on top of
  full-screen applications**, on whichever Space the user is on, and **stays
  in front** once triggered.
- **Dismissed only by clicking outside it** — not by hovering elsewhere, not by
  scrolling, not by typing, not by moving the mouse to the menu bar of the
  application behind it.
- **Draggable.**
- **Dark by default since the v3 design (D-212), with a theme switcher in the
  foot.** It was light by default before; a stored choice still wins.
- **Hold to confirm.** A write or an action takes holding Return or the button,
  never a single press.
- **One living pill (D-216).** The Ask button in the field is the only control
  with the drifting gradient and the pointer glow. Do not spread it to other
  buttons; the listening pill gets it when voice exists.
- **Glass, done properly.** Transparency and blur that look like the system's
  own materials. Appearance controls are tucked away, not always visible;
  sliders are short. The panel makes itself readable over whatever is behind it.
- **Rich results.** Cards with images and previews, links that are real
  hyperlinks, a dense table view as an alternative.
- **Restyling looks like a highlighter, not a debugger.** Soft tints with alpha,
  no borders, no outlines, no rings. Colours adapt to the page's own background
  by measurement. Always removable — by a button in the panel and by asking.
- **Clicky is the reference** for how guidance should feel: point at the thing
  on screen, say what it is for, keep up as the person works.

## G8. Decided, and parked for later

- **Harnesses / alternative agent approaches** — raised, and deliberately
  deferred: first finish what is in the plan.
- **"Try asking" should adapt** through a recommendation system rather than a
  fixed list (backlog B-001).
- **A destination that shows the answer in another application** (backlog
  B-002).


## G9. The domain and the website

Status on 2026-10-13. Nothing here is code in this repository yet.

Every account, where each secret is kept, what is planned and what it costs
is in `docs/infrastructure.md` (D-228).

- **Domain:** `summonvans.in`, registered at GoDaddy on 2026-10-13. First year
  paid; it renews in October 2027 at the full price. No add-ons were taken.
- **DNS:** on Cloudflare (free plan) since 2026-10-13. GoDaddy only holds
  the registration and the renewal.
- **Mail:** `vedansh@summonvans.in` is forwarded by Cloudflare Email Routing
  to Vedansh's personal inbox, where a filter files it under a `VANS` label
  and keeps it out of the inbox. Receiving only; sending as the address is a
  separate setup, not needed yet. A second address for the site
  (`hello@summonvans.in`) can be added as another rule when the site wants one.

- **Site:** live since 2026-10-13 at `https://summonvans.in` and
  `https://www.summonvans.in`. Source in `website/`. Cloudflare builds it from
  `main` (project `vans`, root directory `/website`, `npm run build`, then
  `npx wrangler deploy`), so merging to `main` publishes it. The standing
  rules are `website/RULES.md`; the plan and its work list are
  `docs/plans/2026-10-13-website.md`. A new rule is added to the rules file in
  the pull request that made it necessary.

Next, in order (updated 2026-10-14):

1. Vedansh reads the privacy page and rewrites the founder note.
2. The privacy page grows with the tester work; what it owes is listed in
   `docs/plans/2026-10-14-testers.md`. Terms are still owed.

The startup-program application was submitted and not accepted (2026-10-14).
The Windows form delivers: `LIST_TO` is the verified personal address, since
Cloudflare only sends to verified destinations.

Owed, not started:

- **A privacy policy page, and terms.** Raised 2026-10-13; the details are
  still to be talked through. It has to say what VANS reads, what it sends to
  a model, what it keeps and for how long, and how to remove it. It is a
  living page: any change to what VANS reads, sends or keeps changes the page
  in the same pull request. It is needed before testers are invited, and the
  site's privacy section should link to it.

Decided for the site so far: one page, in the panel's own look (ink, paper,
volt; D-212), no download button until there is a tester build.

## G10. Testers, the server and the accounts

Status on 2026-10-14. The plan, its order and what each step owes the privacy
page are in `docs/plans/2026-10-14-testers.md`; every account, key location
and cost is in `docs/infrastructure.md`. This section is the short record.

**Decided**

- **VANS stays closed source.** It is being built as a company. Testers get a
  built app, never the code. A hackathon that required a public repository and
  an open-source licence was turned down for this reason.
- **Testers use Vedansh's keys, through the VANS server** (D-229). The app
  carries no key and builds no prompt when `VANS_SERVER_URL` and `VANS_INVITE`
  are set. Each tester has an invite code with a daily allowance; all testers
  together have a total.
- **Only model providers that need no card:** Gemini, Groq (D-227) and
  Cloudflare Workers AI, each request going to the one best placed for it by
  measured accuracy and live speed. Mistral, NVIDIA, Cerebras and OpenRouter
  were looked at and left out (see the plan).
- **Cloudflare is the public face; AWS in Mumbai is what the app talks to**
  (D-228): the server on Lambda, DynamoDB, downloads on S3.
- **In the first tester build:** start at login, a pill in the notch, and
  watching and learning, which is off until the tester turns it on, keeps its
  notes on the Mac, never records typed text or pictures, and asks the model
  only about a repeat it has already found.
- **Applications that publish nothing** (DaVinci, Adobe) are guided through
  their menus and shortcuts first, then macOS's text recognition for exact
  places (D-231), then a picture-reading model for icons without words.
- **A console** on the VANS server for Vedansh alone: testers and codes,
  activity, feedback, an overview.

**Done**

| What | Where |
|---|---|
| A second provider when the first is overloaded | #176, D-227 |
| Where everything lives, written down | #177, `docs/infrastructure.md`, D-228 |
| The tester plan; provider choice and unreadable apps; the console | #178, #179, #182 |
| The VANS server and the app's side of it, tested end to end here | #180, D-229 |
| CI on every pull request | #181, D-230 |
| Each question to the provider best placed for it; `cmd/vans-eval` scores them | D-232 |
| Routing chosen for accuracy; Gemini latest first, 21/21 | D-233 |
| The server live on Lambda in Mumbai; testers, counts and feedback in DynamoDB; keys in Parameter Store; feedback mailed; `vans-testers` makes codes | D-234, `docs/infrastructure.md` |
| "Send this to Vedansh" on a failure and "Tell Vedansh" in Setup | D-235 |
| The console at `/console`: overview, testers, activity, feedback; one row kept per request for 30 days | D-236 |
| AWS: budget alert, `vans-deploy` user fenced to VANS, $143.49 credit | `docs/infrastructure.md` |

**Feedback mail:** the Gmail address is verified and the three DKIM records
are in Cloudflare (2026-10-14); mail flows once SES finishes checking them.
The hands-on suite through the live server passed fifteen of fifteen.

**Console:** `https://console.summonvans.in`, signed in with
`VANS_CONSOLE_KEY` from `.env` (D-236). The function's own address shows no
console.

**Open pull requests (2026-10-14):** #190, the privacy page for testers, for
Vedansh to read; merge it when the first tester build goes out. #191, the
console's own address, already deployed; merge when Vedansh says.

**Next, in order:**

1. Every permission asked for at first launch, on one screen, never in the
   middle of a step. About a day.
2. Start at login, and the notch pill. About a day.
3. Watching and learning, off until the tester turns it on. The largest piece:
   about four to five days.
4. The tester kit: the packaged app, a first run that takes the invite code,
   updates from S3, and the "Open Anyway" page. About a day.
5. Install from zero on a fresh macOS user, then the hands-on suite. About a
   day, on Vedansh's timing.
6. Merge the privacy page (#190) with the first build.

**When testers can start (estimate, 2026-10-14):** one to two weeks at this
pace; about five working days without watching and learning. The hands-on
runs need Vedansh's timing and are where it slips. Offered: ship the first
build without watching and learning and add it in an update; not decided.


i encountered this - two steps open at a time even though the previous step was already completed. 
Also it has a delay in reading whether the step is complete or not. 
Also it has another problem as well - If I change the screens like actually move to other apps by 3 finger slides it should close the window actively - more like hide the things (actions, multi step coach etc) and multi step coach should resume from that point itself when we move back to the window- option space should open the window in other apps as well
Also it automatically opens the apps that contains the steps. It shall not do that, it must instruct the user to open the required App as a part of the multi step guide.
## G11. Non-negotiables: check these before calling any change done

Vedansh should never be the one who finds these. Every change to the panel,
the dot, the coach, a guide or anything that puts a window on screen is
checked against this list before it is called done. Each check is a look or
one command, not a test suite: it costs seconds.

| # | Rule | How to check, cheaply |
|---|---|---|
| 1 | The panel opens in the same place every time: centred, about a fifth of the way down, on the screen with the pointer. Like Spotlight | Open it three times from different apps; it lands in the same spot |
| 2 | Nothing VANS puts on screen may stop the menu bar, the clock, Control Center or the Dock from appearing, in any app, full screen included | List VANS's windows (command below): only the panel, the coach and the 24-point dot, and nothing wider than the dot in the top strip. Then move to the top in a full-screen app |
| 3 | Normal pointer movement never opens anything. Only a deliberate rest on the dot (0.3 s), the hotkey or a click | Move to the top edge and the corners in a full-screen app and on the desktop; nothing opens |
| 4 | A screen that wants a key (hold Return, typing, a question) has the keyboard when it appears. No click first | Run a task to the confirm screen and hold Return without touching the mouse |
| 5 | VANS never covers what it sent the person to (System Settings, another app). It steps aside and comes back when the thing is done | Press every "Turn on" or "Open" button and look |
| 6 | A panel opened by hovering closes itself about a second after the pointer leaves it untouched; one opened by the hotkey or used never does | Hover, leave; hover, click, leave |
| 7 | The person sees only what a person needs. Never raw accessibility names (`AXMenuBarItem`), counts of what was read, internal numbers or citation numbers. An answer is the answer; the raw list hides behind "Show what VANS read" | Ask "what is this page about" in two apps |
| 8 | A guide never repeats the step it just gave, never goes back and forth, and never says "click X to type". If it would, it asks or says it is stuck | Read the step lines in the log (`[n] step k:`); no two the same in a row |
| 9 | A run that repeated or went in circles is never kept as a recipe | The log says "not keeping these steps" for such a run |
| 10 | Nothing shows in a full-screen app unless the person asked for VANS there | Open a full-screen app; only what was summoned appears |
| 11 | The first sign of an answer within 3 seconds; a whole step within 6. Slower than that is a bug to log, not a fact | `[vans] timing:` lines in the log |
| 12 | Say what was checked and what was not. "Builds" is not "works" | The PR body names each rule above that the change touched and how it was checked |

List VANS's windows on screen, with size and layer:

```bash
cat > /tmp/wins.swift <<'S'
import CoreGraphics
for w in CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as! [[String: Any]]
where (w[kCGWindowOwnerName as String] as? String ?? "").lowercased().contains("vans") {
  print(w[kCGWindowLayer as String] ?? "", w[kCGWindowBounds as String] ?? "")
}
S
swift /tmp/wins.swift
```

## G12. Handoff, 2026-10-15

**Merged today.** #191 (the console at `console.summonvans.in`). #192, every
permission asked for at once in Setup (D-237). #193, start at login and the
notch pill (D-238).

**In the pull request with this log, tried by Vedansh on his Mac unless said:**

- The pill is now only the yellow dot, in a 24-point window just left of the
  notch (centre of the menu bar without one). No black shape: the wide one
  looked wrong. Breathes while working, a soft ring while guiding.
- Resting on the dot for 0.3 s opens the panel; a panel opened that way closes
  itself 1.2 s after the pointer leaves the dot and the panel, unless it was
  clicked or typed in (`notch::keep`, `keep_panel`). Hover only counts when the
  dot is on the current desktop (`dot_in_view`), never in a full-screen app.
- The confirm screen takes the keyboard (`take_keyboard`), so holding Return
  works without clicking the panel first. Not yet tried by Vedansh.
- An answer shows alone: the app's name, the answer, and a "Show what VANS read
  (n)" link for the raw list. Citation chips only for real links.
- The panel opens in the same place every time (`put_at_home`), on the screen
  with the pointer. Not yet tried by Vedansh.
- Setup: a refused app is asked again after its old answer is cleared (`tccutil
  reset AppleEvents`, installed app only; run from the code it opens Settings).
  Accessibility and Screen Recording can only be switched on by the person:
  "Turn on" opens the pane, the panel steps aside and comes back when it is on.
  Vedansh parked the rest of Accessibility for later.
- Each request writes `[vans] timing: <ms>, <calls>, <app>: <what was asked>`
  to the shell's log, for the speed table.

**The menu bar glitch.** In full-screen apps the menu bar stopped coming down.
It was macOS: it stayed broken with VANS quit and cleared after switching
desktops. Rule 2 above stands anyway, because the first wide pill sat inside
the menu bar strip.

**Postman, "how do I set the baseUrl" (coach), what went wrong:**

1. "Click the Variable row" was given five times in a row, and "the
   Environment combobox" twice. `goingInCircles` (`internal/engine/circles.go`)
   only stops a step after it changed nothing twice, or after the same click 8
   times (`mostSameClicks`). Each click changed something small (focus), so
   nothing stopped it.
2. The step said "click the Variable row to type baseUrl" and never asked the
   person to type.
3. The run was kept as the recipe for next time (`rememberTheRun`,
   `internal/engine/trace.go`), loops included.
4. Each replan took 5 to 6 s, and the first answer 11.7 s (one call to route,
   then planning).

**Fixes, in order (for AGY):**

1. `circles.go`: the same step as the one just given is circling, unless it is
   a button that moves a wizard on (Next, Continue). `mostSameClicks` from 8 to
   3. A test with a Postman-shaped run where every click changes the focus.
2. The step prompt: when what is next is typing into a field, the step is
   `type` with the text, or a coach step that says "Type baseUrl here". Never
   "click X to type". A test on the parsed step.
3. `rememberTheRun`: do not keep a run in which any step repeats back to back
   or the same step appears more than twice; log "not keeping these steps:
   they went in circles".
4. Speed: find where the 11.7 s and the 5 to 6 s per replan go (route, plan,
   reading 376 elements). Rule 11 is the bar.
5. Then a timed run across apps, with Vedansh: Postman, AutoCAD
   (`/Applications/Autodesk/AutoCAD 2027`), Figma and Canva in the browser,
   Finder, Notes, Mail, Calendar, Numbers, Chrome, WhatsApp, VS Code,
   DaVinci. One table: app, task, time to first step, right or wrong.

**After that, the plan's order:** watching and learning (4 to 5 days), the
tester kit (a day), install from zero (a day), then the privacy page (#190).

**Done, same day (D-239).** Fixes 1 to 3 are in, with tests: the same step
straight after it worked is refused, clicks stop after three unless the button
moves on, a guide can point at a row to type in it and is told to type rather
than "click to type", a click on the field no longer ends a typing step, and a
looped run is neither kept nor replayed (the Postman recipe is ignored). Fix 4
is measured, not fixed: the model alone takes about 2 to 3 s on a Postman-sized
window, half what the run saw; every model call now logs its time and tokens
(`[model] ... answered in`), so the next run shows where the rest goes. Not yet
seen on Vedansh's Mac: the Postman question again, which is the first thing to
try in fix 5.

## G13. Watching and learning, and timing: where it stands, 2026-10-15

**Merged.** #196 noting clicks on controls (D-240); #197 finding repeated
clicks and offering them on the ask screen (Save as a routine, Show me, Not
now). On by default, switchable in Setup › Learning. Not yet seen running on
Vedansh's Mac: the switch, the noted list, the hollow dot when off, the white
centre and the habit card. Rebuild the engine and restart the app to try it; a
card needs a habit done three times across two sittings.

**In progress on the branch `watching-timing`: learning when.** Asked for by
Vedansh: notice what he does at the start of the day, after work, or right
after opening an app, and offer it at that moment ("Start your timer, like
usual?"). The plan is in `docs/plans/2026-10-14-testers.md`, "Timing: learning
when". His answers on 2026-10-15:

- The offer is a small line on its own background, just below the dot, so the
  notch cannot hide it. It never opens the panel.
- Note app switches, Mac wake / sleep / lock / unlock, and click times.
- Trust grows: Run / Skip the first three times it is taken up, then Always
  too. With Always on: "Starting … in 5 s · Cancel", then run it. Cancel turns
  Always off until it is chosen again.
- How Clicky does it (HeyClicky 1.0.53, reinstalled by Vedansh; read from its
  Info.plist and the labels in its binary only, nothing run or taken):
  - "Fresh task suggestions every 24 hours, announced through the morning
    notch", with a good-morning greeting there.
  - A suggestion is held while the person is focused, can be snoozed, and
    rests for a while after repeated dismissals ("ignored-suggestions break").
  - Routines run on a schedule; due ones are held until the Mac has settled
    after waking, and wait for the internet.
  - Its memory of the person is kept on its server (`/me/memory`), not on the
    Mac. Its Accessibility line says it is for focus tracking "not background
    watching".
  Worth taking: hold an offer while someone is busy typing or clicking, and
  show it at the next pause; rest after repeated dismissals rather than only
  stopping; hold offers until a few seconds after a wake. Different on
  purpose: VANS keeps what it learns on the Mac.

Done on the branch, with tests:

- The shell reports app switches and Mac moments (`ui/src-tauri/src/watching.rs`,
  `moment` RPC); the engine keeps them in `clicks` with role `front` or
  `moment` (`NoteMoment`), never for password managers or VANS, never while
  guiding. The click-habit finder ignores them.
- "See what VANS noted" reads them plainly ("Switched to Mail", "Mac unlocked").
- `FindTimed` (`internal/engine/timing.go`): at a time of day (same thing
  within twenty minutes on three days, weekdays told apart), after the Mac is
  opened (within ten minutes of the first unlock or wake, three days), after an
  app (within five minutes of its first coming forward that day, three days and
  at least 60% of the days it came forward). Lines like "Start in Clockify,
  like usual?".

**Built since (D-241), on the same branch:** the `timed` table and answers,
what is due and when, the line below the dot (`nudge`), trust that grows, good
morning, holding while busy and after a wake, quiet after three dismissals,
running a habit, and sending what was learned to the server for testers.
Setup says what is sent. The privacy wording is drafted in the tester plan for
Vedansh. Not yet seen running on his Mac; the server change is not deployed.

Was next, in order (all done above):

1. Keep timed habits and their answers in a `timed` table: yes count, skips in
   a row (three stops it), Always, the day last offered.
2. Decide what is due: a minute tick for times; the first unlock or wake of
   the day; the first time an app comes forward that day. Not if the action
   was already done today. Push `offer` to the shell.
3. The line below the dot: a new window `nudge` (needs its own capability file,
   D-086), non-activating, fades after eight seconds unless pointed at; Run,
   Skip, Always after three yeses; the countdown with Cancel.
4. Running: a click action goes through as a "do" request with every
   confirmation; opening an app is done by the shell on Run (record it as a
   decision, since D-032 and D-131 say VANS never opens an app by itself; here
   the person asked).
5. Decision record (D-241), privacy page text for Vedansh, non-negotiables 3
   and 10 checked (nothing opens by accident; nothing in full-screen apps).

**Decided by Vedansh after the Clicky look (2026-10-15):**

- Take all of it: good morning through the notch with what is usual today,
  hold offers while busy, Later (snooze), rest after repeated dismissals, wait
  after a wake and for the internet. Details in the tester plan.
- For testers, what VANS learned goes to the VANS server: each habit found and
  how it was answered. The raw click and app log stays on the Mac. May become
  Mac-only later. This changes D-240 ("nothing sent"): Setup's wording and the
  privacy page must say it before it ships.
- Logo: draft B, the spotlight ring opening for the pointer, which lands on the
  centre. Next: the final vector files (app icon set, favicon, the in-app
  mark) and redrawing the on-screen ghost cursor to the logo's pointer.

## G14. Where it stands, 2026-10-15 evening

Everything in G13 is merged, and so is what followed it:

| PR | What | Decision |
|---|---|---|
| #198 | Timing: the line below the dot, trust that grows, good morning, sending what was learned | D-241 |
| #199, #200 | The mark: app icon, ghost cursor, site favicon (live) | D-242 |
| #201 | Tester kit: tester build with no instructions in it, invite code, signing, updates through the server, Quit | D-243 |
| #203 | Marking text on a page with no model call; five colours from `internal/theme/themes.json` | D-244 |
| #204 | Offers wait for the internet; "before you finish" habits | D-245 |
| #205 | The flaky guide test steadied | — |
| #207 | Apps that publish nothing: menus, words read on the Mac (exact, clicked), the model only for icons | D-246 |

Deployed: the server with `learned` and the update route; the private bucket
`vans-releases` exists and the server reads it.

**Waiting on Vedansh, in this order:**

1. Read and merge #190 (privacy page: the server, watching and learning) and
   #202 (install page). Both publish the site.
2. Run `./infra/mac/signing-setup.sh` once (it asks for the Mac password to trust
   the certificate), then `./infra/mac/release.sh "first test build"`, then
   `./infra/mac/tester-link.sh` for a download link.
3. Name a moment for the hands-on runs: the app from the code (colours in
   Setup › Appearance, marks on a page, the line below the dot), the Postman
   retry, the multi-app timed run, and the hands-on suite with Groq and Workers
   AI writing.
4. Install from zero on a fresh macOS user (VANS cannot create the user).
5. Measure the words against real applications when convenient:
   `go run ./cmd/vans-probe -words Finder Notes "System Settings"` (brings each
   to the front for a moment). Rendered windows measured 98.4% found, 100% on
   target (D-246).
6. Cloudflare PR previews: the site, `wrangler.jsonc` and the account are fine —
   `npx wrangler versions upload` from this Mac made a working preview on
   2026-10-15. What fails is the dashboard's build for non-production branches.
   In Workers & Pages → vans → Settings → Build: either set the non-production
   deploy command to `npx wrangler versions upload` (root `/website`, build
   `npm run build`), or turn off builds for non-production branches. The GitHub
   `site` job already builds the site on every pull request.

Done and checked: SES DKIM (verified, signing on, DMARC passes on DKIM). SES is
still in the sandbox, which only matters for mail to addresses other than
Vedansh's own.

## G15. Where it stands, 2026-10-16

| PR | What | Decision |
|---|---|---|
| #209 | A welcome on first run, and "What VANS can do" from the engine | D-247 |

Vedansh read and approved #190 (privacy page) and #202 (install page) on
2026-10-16; they are merged after this. Everything else in G14's "Waiting on
Vedansh" list still stands, from item 2 on.
