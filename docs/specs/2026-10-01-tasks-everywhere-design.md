# Multi-step tasks everywhere — design

Status: proposed. Nothing here is built. Follows AGY §9.13.

## What is being asked for

Three examples, and they are three different problems:

- "Book a ticket on MakeMyTrip", "put this video on YouTube" — many steps on a
  **web page**.
- "Send Good Morning to Joe on WhatsApp" — a few steps in a **Mac application
  that can be read**.
- "How do I apply a LUT to this timeline" — coaching inside an **application that
  cannot be read** (Adobe's, and every Electron one).

What decides the work is not the application but how VANS can see it.

| Kind | Examples | How VANS sees it | State |
|---|---|---|---|
| A. Readable Mac apps | System Settings, TextEdit, Notes, Finder, WhatsApp | accessibility tree | multi-step works (coach and drive) |
| B. Web pages | anything in Chrome | the page itself, through the browser | single actions only |
| C. Unreadable apps | Lightroom, Premiere, Slack, VS Code, Discord | nothing | not reachable at all |

## Phase 1 — multi-step on web pages

The step loop (`stepThroughTask`) is already independent of where a step comes
from. What it needs is a second kind of screen: one whose `Capture` reads the
page's controls and whose `Act` clicks and types in the page.

- **Reading.** One script in the page lists what a person could act on — links,
  buttons, inputs, selects, anything with a role — with its label and its
  rectangle from `getBoundingClientRect`. That maps onto `uiact.Tree` as it is,
  so validation, repair, re-planning, the trace cache and the out-of-view rule
  all apply unchanged.
- **Acting.** Click and type in the page, through the same Apple Events path
  page actions already use.
- **Coaching.** The rectangle is in page coordinates; add the window's content
  origin and the ghost, ring, caption and trail work as they do now. This lifts
  `ErrNoControlsToPointAt` for browsers.
- **Navigation is a change.** A step that loads a new page is a step that landed.
  The loop waits for the load, then reads again.

### Which browser profile

Already the right default: the engine drives the Chrome the person has open
(`appleevents.Chrome`, tried first by `LiveFirst`), so tasks run signed in. The
debugging-port Chrome on a throwaway profile is the fallback.

To add: saying so in the request. "…in a new profile" or "…in a clean browser"
routes the task to the throwaway one; nothing said means the current one. A
route slot, not a setting, because it is a per-task choice.

Needs Chrome's *Allow JavaScript from Apple Events* to be on. Preflight should
say so in one line when it is off.

### Stop points

A task that pays, publishes, sends or deletes stops before that step and shows
exactly what it is about to do. Per step, not per plan: the confirmation at the
start cannot know what the final screen will say.

- The model marks a step `final: true` when it commits something that cannot be
  taken back. Words on the control are a second check (Pay, Buy, Publish, Post,
  Send, Delete, Confirm order) so a missed mark is still caught.
- The stop uses the pause that exists (D-146): the overlay says what is about to
  happen, ⌥Space carries on, Escape stops.
- Payment details, OTPs and captchas are never entered. The run hands over there.

### Questions mid-task

"Which flight?" is not the model's to guess. A step may be a question: the
overlay shows it with the choices read off the page, and the answer goes into
what has been done so far. Without this a booking task is a guess with a card
attached.

### Done when

On the current profile: a search-and-fill task on a travel site reaches the
payment page and stops there; a YouTube upload reaches the publish button and
stops there; the coach walks through the same two with the ghost on the page.

## Phase 2 — more readable apps

WhatsApp for Mac is a native application now, so the loop should already reach
it. "Should" is the word: every application publishes its controls differently.

- **A probe.** `vans probe <app>` reads the front window and reports how much of
  it is usable: how many controls, how many have names, how many can be pressed.
  It answers "will VANS work here" in a second, before anyone tries a task.
- **A sweep.** Run the probe and one coached task across the applications that
  matter (WhatsApp, Messages, Mail, Notes, Finder, Calendar, Music, Xcode). Fix
  what the sweep turns up; record what cannot be fixed.
- **Sending is a stop point.** "Send Good Morning to Joe" stops with the message
  typed and the send button ringed, the same rule as publish and pay.
- **The right person.** Picking a chat by name is where this goes wrong. The step
  that opens the chat is verified by the chat's title reading back as the name
  asked for, before anything is typed.

## Phase 3 — applications that cannot be read

The vision tier from AGY §9.6. A screenshot of the window goes to the model with
the question; it answers with a place on the screen.

Coach first, and only coach to begin with. Pointing at the wrong place costs a
moment of confusion; clicking the wrong place in someone's edit costs their work.
Driving through vision waits until pointing is measurably right.

It is slower and dearer per step than reading a tree, which is against *fast
model calls*. The trace cache is what makes that tolerable: a coached task that
finished is replayed from memory, and with vision the replay check is "does this
part of the screen still look the same", not a model call.

Screen Recording permission is needed for the screenshot.

## After these

Unchanged from what was agreed: voice in and out, then tasks that run in the
background. Background runs depend on acting without the window in front, which
`AXPress` already allows for kind A and the page script allows for kind B.

## Order

1. Web multi-step, drive and coach, with stop points and questions.
2. Probe and sweep of readable apps; WhatsApp end to end.
3. Vision coach for unreadable apps.
4. Voice.
5. Background runs.
