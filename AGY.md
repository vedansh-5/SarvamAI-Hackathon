# AGY — the working record

Everything VANS is, everything that has been built, every fact that cost
something to learn, and what happens next. Written to be read cold, six months
from now, by someone who has forgotten all of it.

Companion documents: `docs/decisions.md` is the reasoning behind each choice
(68 records, D-001 to D-068); `docs/roadmap.md` is the phase plan;
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
   click it where it actually is. Works for native applications.
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

### Not proven

- **A full click-to-result loop on a real application has never been watched
  end to end.** Every piece is verified separately — capture, click point,
  occlusion guard, CoreGraphics event posting — and the pieces have never been
  stitched together on a live window successfully. The closest run reached the
  input layer and died there, which is exactly what #44 replaced.
- **The drive loop has never completed a task.** It could not, because until
  #44 no click could land.
- **The coach overlay has been verified as a window on screen** — full display,
  window level 26 (one above the panel), click-through — and it is now in the
  repo. Its visual design has not yet been reviewed, and nothing drives it: the
  probe that proved it draws was removed before the commit.

---

## 6. Facts that cost something to learn

These are the ones worth keeping. Every one of them was a wrong assumption held
for at least an hour.

### macOS and the accessibility API

- **Only the frontmost application publishes AX windows.** Capture of a
  background application returns nothing. Everything that reads a window has to
  bring it forward first (D-051).
- **Being frontmost is not the same as being on screen.** A process can be
  frontmost while Stage Manager keeps its window off-stage. Forwarding now waits
  for a window to actually appear and says what is probably hiding it (D-064).
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

1. **Stage Manager breaks the accessibility tier on this machine.** Activating
   an application brings the process forward but leaves its window off-stage,
   so capture fails. The failure is now legible rather than mysterious, but the
   tier does not work while Stage Manager is on.
2. **Chrome's accessibility tree comes back empty** (0 elements), so anything
   routed to the accessibility tier for a browser fails. Browsers should route
   to the page tier consistently; the app sink now falls back to typing where
   the cursor is, which works, but the ladder could be more decisive.
3. **Clearing a restyle acts on the tab in front.** Highlight one tab, switch
   tabs, clear — and the first tab stays highlighted.
4. **The API key lives only in the launching shell's environment.** Nothing
   persists it.
5. **Nothing is packaged.** There is no built `.app`; it runs from a checkout
   through `npm run tauri dev`.
6. **The coach overlay has no engine behind it yet.** It draws where it is told
   to; nothing decides what to point at.

---

## 9. What happens next

In order. Each item says what it is and how it gets built.

### 8.1 The coach flow, end to end — "show me what to do"

The overlay exists and draws. What is missing is the part that decides what to
point at.

- Add a route kind `app.guide` — "how do I X in Y", "where is the setting for Z".
- On that kind: bring the application forward, capture its window (already
  ~130 ms), and hand the model the control list — roles and labels only, the
  same withholding the acting tier does.
- The model returns a control selector and one line of instruction. Both are
  checked: the control must exist in the tree, and its visible rectangle must be
  on screen.
- Resolve the rectangle through `Tree.ClickPoint` / `visibleBox`, and call
  `point_at` with it. Clear on Escape, on the panel dismissing, or when the
  window changes underneath.

This is wiring, not new capability: the rectangles already come out of capture,
and the overlay already takes a rectangle and a label.

### 8.2 Multi-step coaching

Point at the first control; the *person* clicks it; re-read the window; point at
the next. This reuses the drive loop's structure with the click removed — and
it is the honest version of the agent workflow, because the user stays in
control and can stop at any step.

`Changes(before, after)` (D-067) already provides the "did that do anything"
signal the loop needs.

### 8.3 Prove the full click loop

The single most important missing piece of evidence. Needs a target application
that is willing to be touched and a desktop where the window is actually
visible. The run to make: bring the app forward, capture, plan, verify, click,
read back, report what changed.

### 8.4 The app agent workflow

With input posting and read-back both working, the drive loop can finally be
exercised. Each step already carries its effect into the next request, so the
model is told "nothing in the window changed" rather than assuming success.

### 8.5 Smaller, worth doing

- Persist settings and the key, so the app does not depend on the launching
  shell.
- Package a real `.app` so it does not need a checkout.
- Route browsers to the page tier decisively rather than letting them fall
  through the accessibility tier first.
- Clear a restyle across every tab that carries one, not just the front tab.
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
