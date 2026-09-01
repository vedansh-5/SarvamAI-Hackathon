# VANS — Roadmap

Where the project stands and what comes next. Written 2026-08-16, at the point
where V1 finished and the product shape changed.

Companion to [`decisions.md`](decisions.md), which holds the *why* for
everything already built. This file holds the *what next*.

---

## Where we are

**V1 is complete and works.** Ten steps, all merged except the last two.

| Step | Component | State |
|---|---|---|
| 1–2 | `.sdef` capability parsing, AppleEvents adapter | merged |
| 3 | CDP adapter (Chromium) | merged |
| 4 | Candidate finder (MDR) | merged |
| 5 | Page signature | merged |
| 6 | Synthesizer (the one LLM boundary) | merged |
| 7 | Runner + layered verifier | merged |
| 8 | Program store (the cache) | merged |
| 9 | Engine + failure ladder + CLI harness | merged |
| 9c | Generalized nodes (records spanning siblings) | merged |
| 10 | Desktop shell (Tauri) | merged |
| 11 | Overlay (hotkey panel) | merged |

**Measured live** on Hacker News, Chrome 151, free open-weight model:

- cold run: 9.5 s, 1 model call
- warm run: **344 ms, 0 model calls**

That is the whole thesis, demonstrated: the model writes a program once, the
runtime owns and replays it forever.

**Never done yet:** a real write through the GUI. Every run so far used
`-dry-run`. Extract → review → **Write to Notes** in the window closes the loop
on D-017 and proves V1 end to end.

---

## The pivot

V1's shape was a window you go to. The chosen shape is now an **overlay you
summon** — hotkey, over whatever you are working in — growing into something
that can see the screen, hear you, and act across any application, the way
[Clicky](https://www.heyclicky.com/) does.

**Scope decided 2026-08-16:** all of it — overlay, voice, screen context,
acting on the machine. Explicitly *"we can make it fast later on as well but it
should work properly first."* Correctness before speed.

### How this does not throw away V1

Vision is added as the **bottom rung of the interaction ladder**, not as a
replacement for the rungs above it. The ladder was already sketched in D-009
and D-010:

```
MCP → AppleEvents/.sdef → App Intents → CDP → Accessibility → Vision
 rare    structured        structured   web    to build       universal
                                                              fallback
```

- **Vision means it works on anything** — Telegram, DBeaver, any app with no
  scripting dictionary. That is the universality Clicky gets.
- **The structured tiers mean that where a fast path exists, it is taken** —
  344 ms and zero cost, instead of a screenshot and a model call.

Both properties hold at once. Nothing already built gets rewritten; the ladder
is extended downward.

**The cost, stated plainly.** Anything reached only by vision costs a model
call per action, is slower, and is not deterministically replayable. The
caching thesis (D-002, D-006) still applies to the structured tiers and does
*not* apply to vision until there is a way to sign and cache a screen the way
`extract.SignatureOf` signs a DOM. That is an open research problem, not a
task — see Q-15.

---

## Phases

### Phase 1 — Overlay shell — complete

Hotkey-summoned floating panel replacing the ordinary window. Shell only; the
engine was untouched.

- `ui/src-tauri/src/overlay.rs` — ⌥Space toggles the panel, dismiss on blur,
  `ActivationPolicy::Accessory` so there is no dock icon
- `tauri.conf.json` — borderless, transparent, always-on-top, starts hidden
- Escape and Cancel route through the `dismiss` command rather than closing the
  window, which would destroy state the user may come back to
- Focus moves to the intent input on `overlay://shown`
- `body` paints nothing; all colour belongs to `.panel`, which is translucent
  with a `backdrop-filter` blur. A painted body would show as a hard rectangle
  behind the panel's rounded corners, which is the usual way a transparent
  window goes wrong

Both of the deferred items are now closed:

- **The window follows the content.** The frontend measures `.panel-body` and
  reports its height; the shell clamps it to `[180px, 85% of the display]` and
  resizes. The fixed 720×460 was found the way these things usually are — a
  screenshot of preflight, opened scrolled into its own middle with the heading
  cut off the top and nothing on screen to suggest there was anything above it.
  Only `.panel-body` is measured, never `.panel`: `.panel` stretches to fill the
  window, so measuring it would report the window's height back to the window
  and the panel could grow but never shrink.
- **Preflight pins the panel.** Rather than a second window, the frontend asks
  the shell to suspend dismiss-on-blur while a screen is up whose instructions
  send the user elsewhere. Escape and ⌥Space still dismiss, so there is no way
  to get stuck. ⌥Space now distinguishes *visible* from *focused*: a pinned
  panel that was clicked away from is raised rather than hidden, or the hotkey
  could not recover the one panel that deliberately stayed up.

Fixed alongside them, found while reading the Phase 2 frontend: the active
context banner built itself with `innerHTML` from the frontmost window title.
A window title is attacker-controlled whenever the frontmost application is a
browser — `document.title` is writable by any script on the page — so a visited
page could put markup into the panel just by being frontmost when the hotkey was
pressed. It goes through text nodes now, like everything else that reaches this
DOM. The CSP blocked script execution, so this was a hole in the second layer
rather than the first, but the rule that everything reaching this DOM is a text
node is the layer that is supposed to hold.

### Phase 2 — Screen context ← implemented on phase-2-screen-context branch

Know what the user is looking at without token bloat.

- `internal/screen/context.go` & `internal/rpc/protocol.go`: Go runtime screen context models & `screen_context` RPC method.
- `ui/src-tauri/src/screen.rs`: Frontmost application, bundle ID, window title, PID & display geometry tracking via macOS native APIs (`NSWorkspace`, CoreGraphics).
- Low-latency screen capture to memory/temp buffer with `screencapture`/ScreenCaptureKit integration.
- `has_screen_recording_permission` & `request_screen_recording_permission` for non-intrusive preflight checks (D-037).
- Frontend UI updated with live Active Context banner (`🎯 Active: App — Title`), Screen Recording preflight check button, and `Escape` hotkey dismiss.

### Phase 3 — Acting on the machine

The hardest phase, and the one that makes it an agent rather than a reader.

- Read the accessibility tree (`AXUIElement`) to find real, addressable
  elements instead of guessing at pixel coordinates
- Post synthetic input (`CGEvent`) for clicks, typing, scrolling
- **Requires the Accessibility grant.** Recorded as not granted in
  `decisions.md` § Environment facts

**Architectural decision needed first.** These are C/Objective-C APIs. Go
cannot reach them without cgo, which D-030 deliberately avoided so the desktop
build stays cross-compilable. Proposed split:

- **Rust shell owns the native surface** — hotkey, overlay, screen capture,
  accessibility tree, synthetic input
- **Go runtime keeps reasoning** — planning, synthesis, caching, verification,
  the gate

This makes the stdio pipe **bidirectional**: today the shell calls the engine;
the engine will need to call back to ask the shell to act. `internal/rpc`
currently only handles shell→engine. Write this up as a decision before
building it.

**Verification gets much harder here.** D-004's three categories still hold,
but "did the click land" is a state change that has to be observed through the
accessibility tree, and every irreversible action still needs the D-031 gate.
An agent that clicks Send without a gate is the failure mode to design against
from the start, not to retrofit.

### Phase 4 — Vision tier

- A vision model reads a screenshot when no structured adapter applies
- Slots in as the last rung; everything above it is tried first
- Reuses the existing `synth.Synthesizer` interface — a VLM is another backend,
  and `OpenAICompatible` already speaks the shape most of them use
- Drawing annotations on screen (Clicky's "draws right on your screen") needs a
  second transparent, click-through, full-screen overlay window

### Phase 5 — Voice

- Speech to text for the intent
- Local (macOS Speech framework) or hosted (Whisper). Local avoids sending
  audio anywhere, which matters more here than for text
- **Does not touch the caching thesis** — the intent is text by the time it
  reaches the engine, so the cache key and the zero-call warm path survive
  unchanged

### Phase 6 — Multi-step plans

D-002 described plans as trees with lazily expanded, cached subtrees. V1 never
needed it, because one task was one extraction. Multi-app tasks need it for
real.

- Planner emits coarse goals; each is expanded on first encounter and cached
  by `(goal, app, page signature)`
- The failure ladder has to work per subtree rather than per task
- The gate has to work per irreversible step, not once per task

---

## Immediate next actions

1. **Do one real write through the GUI** to close D-017. Every run so far has
   been `-dry-run`; this is the last unproven step of V1.
2. **Wire screen context into the engine.** Phase 2 captures it and shows it in
   the banner, but nothing downstream consumes it — `internal/engine` has no
   reference to it. It is plumbing without a consumer until it reaches a task.
3. **Grant Accessibility** in System Settings before Phase 3 starts, and
   re-measure the environment facts in `decisions.md`
4. **Write up the bidirectional RPC decision** — see Phase 3. The engine cannot
   call back into the shell today, and it must be able to before it can act.

---

## Open questions carried forward

| # | Question |
|---|---|
| Q-6 | Windows strategy — nothing in D-009/D-010 ports |
| Q-8 | Re-verify the D-008 MCP landscape against live sources |
| Q-9 | Native apps with no scripting dictionary — **Phase 4 answers this via vision** |
| Q-11 | Structural-check thresholds need tuning against real pages |
| Q-12 | Paraphrased intents miss the cache |
| Q-13 | No cross-key eviction policy in the store |
| **Q-15** | **Can a screen be signed and cached the way a DOM is (D-006)? Without it, vision-tier actions cost a model call every time and the central thesis does not extend to them.** |
| **Q-16** | **What does the confirmation gate look like when the runtime is clicking through someone else's UI? A preview table works for extracted rows; it does not obviously work for "I am about to click this button."** |

---

## Running it today

```bash
# 1. Chrome, with a separate profile so your real session is untouched
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 --user-data-dir=/tmp/vans-chrome

# 2. Build the runtime the shell spawns
go build -o ui/vans-engine ./cmd/vans-engine

# 3. The app
export VANS_LLM_API_KEY=sk-or-v1-...     # openrouter.ai, free tier
cd ui && npm install && npm run tauri dev
```

Or without the GUI:

```bash
go run ./cmd/vans -url https://news.ycombinator.com \
  -intent "the title and link of each story"
```

Tests: `go test -race ./...`
