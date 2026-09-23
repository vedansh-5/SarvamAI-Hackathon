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
G8. Decided, and parked for later

---

# Part I — The record

## 1. What VANS is

**VANS — Vedansh's Agent Navigation System.** (It began as "Virtual Agent
Navigation System" and was renamed on 2026-08-16.) A macOS runtime that sits between
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

### Not proven

- **A full click-to-result loop on a real application has never been watched
  end to end.** Every piece is verified separately — capture, click point,
  occlusion guard, CoreGraphics event posting — and the pieces have never been
  stitched together on a live window successfully. The closest run reached the
  input layer and died there, which is exactly what #44 replaced.
- **The drive loop has never completed a task.** It could not, because until
  #44 no click could land.
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
6. **Multi-step coaching has not been exercised across real state transitions.**
   The engine points at controls on screen, but nobody has clicked through a
   several-step task and watched it keep up. The reason it could not work until
   now is in §9.0: summoning the panel activated VANS, and under Stage Manager
   that withdraws the target window from the accessibility API, so every re-read
   after the first step came back empty.
7. ~~The keyboard cannot be driven from outside.~~ **Wrong, settled on
   2026-09-23.** It always could; the measurement was broken, not the path. See
   the entry under *Proven live* and D-098.

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
2. **`TestLiveDriveTask` needs a safe target and has still never run.** The test
   itself was rewritten in #78 — it asks a real model, performs the steps, and
   reads the window back independently of the receipt. What it needs is a window
   with nothing to lose. Nothing open on 2026-09-23 qualified: Chrome and Notion
   are Chromium-shaped and hold real work, dbeaver is a database tool, Clock is
   out by standing constraint. An empty TextEdit document is the target to use,
   and VANS does not open it (D-032) — so it has to be opened by hand:

   ```bash
   VANS_LIVE=1 VANS_LIVE_DRIVE_OK=1 VANS_LIVE_APP=TextEdit \
     VANS_LIVE_TASK="type the word hello" \
     go test -tags integration -v -run TestLiveDriveTask ./internal/engine/
   ```
3. ~~A real keypress reaching the panel.~~ **Done in #77**, see *Proven live*.

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

### 9.4 The app agent workflow (built; one live run left)

With input posting and read-back both working (#63), the drive loop is wired end
to end (#64): progress reaches the window while a plan is committed, a step the
window refuses is re-asked with the reason in hand, and the receipt is verified
only when every step performed was seen to change the window (D-076). Each step
carries its effect into the next request, so the model is told "nothing in the
window changed" rather than assuming success. `TestLiveDriveTask` drives it
against a real application behind `-tags integration` and `VANS_LIVE_DRIVE_OK=1`;
that run has not happened yet.

### 9.5 Smaller, worth doing

- Persist settings and the key, so the app does not depend on the launching
  shell.
- Package a real `.app` so it does not need a checkout.
- Draw the coach's rings and arrows over elements inside a web page, using
  geometry from the DOM. The refusal in #77 is honest but it is still a refusal.
- A second look at the coach's visual design by someone who can see it.

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
4. **No local model runtimes.** Hosted only.
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
- **Name:** VANS — Vedansh's Agent Navigation System.

## G7. UI and design

- **A consumer product, not developer tooling.** It should look like something
  a person would buy, not a debugging console.
- **Apple design language.** System font, a clear type scale, a consistent
  spacing rhythm, restraint.
- **The panel design from the design prototype is final.** Do not revert to an
  earlier design.
- **Summoned, not launched.** Alt+Space, no dock icon. It opens **on top of
  full-screen applications**, on whichever Space the user is on, and **stays
  in front** once triggered.
- **Dismissed only by clicking outside it** — not by hovering elsewhere, not by
  scrolling, not by typing, not by moving the mouse to the menu bar of the
  application behind it.
- **Draggable.**
- **Light theme by default, with a theme switcher.**
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
