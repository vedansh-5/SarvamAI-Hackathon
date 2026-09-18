# VANS

**Vedansh's Agent Navigation System** — a macOS runtime that does things on your
machine when you ask it to, in your own words.

Press **Alt+Space**, say what you want, and it happens: on the web, inside an
application, or on the screen in front of you.

---

## The idea

Most agents think from scratch every time you ask. Ask twice, pay twice, and
wait twice — and there is no reason the second answer should cost anything at
all, because nothing about the task changed.

VANS separates **reasoning** from **execution**. A model writes a small program
once; the runtime owns it, checks it, caches it against the *shape* of the
request, and replays it with no model involved.

A cold run costs one or two model calls. A warm run on the same shape costs
**zero**, and takes milliseconds. Measured, not claimed — application actions go
from 2,970 ms and two calls to **11 ms and none**.

It gets faster the more you use it.

## Accuracy is a floor, not a prompt

Nothing trusts the model to have been careful. Every tier has a check the model
cannot lower or talk its way past:

- An extraction program is **run**, and its output verified, before it is trusted.
- A selector that matches nothing is **refused**, not retried hopefully.
- A click point is re-checked against the live window, and a click that would
  land in another application's window is refused.
- An answer with no citation into the results is thrown away.
- After acting, the window is **read back** — and a click that changed nothing
  is reported as not confirmed, because that is the signature of a click that
  missed.

Where possible, the model is given a set to choose from rather than a string to
write, so it cannot be wrong in a way anything has to catch.

## What it does today

| | |
|---|---|
| **Read the web** | Pull records off a page and put them where you want — the panel, the clipboard, a note. |
| **Search and answer** | Ask a question; it searches, reads the results, and answers with citations into them. |
| **Read what is in front of you** | "What does this say" works on a page *or* a native window. |
| **Act in applications** | Through an application's own scripting dictionary, through the page it is open in, or through the controls on its screen. |
| **Restyle a page** | "Highlight the headings" — it marks what you asked about, in a soft tint that adapts to the page's own background. Take it off again with "clear the highlighting". |
| **Type where the cursor is** | Send an answer into whatever you are looking at, even an application that publishes nothing about itself. |

Everything that changes something outside VANS passes a confirmation gate.
Everything that only reads does not — a gate in front of a harmless action
teaches people to dismiss gates.

## What it is becoming

**An agent that works your applications with you, and shows its work.**

- **Show me what to do.** Ask how to do something in an app and VANS points at
  the control on screen — dimming everything else, ringing the thing you need,
  with one line telling you what it is for. Then the next step, and the next.
  You click; it watches; it keeps up.
- **Do it for me, visibly.** The same loop with the clicks made for you, on
  screen where you can see every one of them and stop it at any point. Not a
  batch job that reports success afterwards — a thing you watch happen.
- **Know what it did.** Every action is checked against the window afterwards,
  and it says what changed in plain words rather than claiming success.
- **Ask about anything on screen.** Not just pages, and not just text.

The long shape: you stop learning where things are in applications, because
something that can see the screen and read the window will show you, or do it.

## Principles

1. **Accuracy above all.** When in doubt, refuse rather than guess.
2. **Minimum permissions.** Input is posted using the accessibility trust it
   already holds to read a window, rather than asking for a second, broader one.
3. **What leaves the machine is minimised per tier.** Acting on a window sends
   roles and labels, never the contents. Restyling a page sends its structure,
   never its text. The rule is not "is this sensitive" but "is the request
   about it".
4. **Speed is a feature**, not a nice-to-have.
5. **It never launches applications.** It works with what you already have open.

## Running it

```bash
# Chrome with a separate profile, so your real session is untouched
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 --user-data-dir=/tmp/vans-chrome

# The runtime the shell spawns — without this, the app will not open
go build -o ui/vans-engine ./cmd/vans-engine

# Any OpenAI-compatible endpoint
export VANS_LLM_API_KEY=...
export VANS_LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
export VANS_LLM_MODEL=gemini-flash-lite-latest

cd ui && npm install && npm run tauri dev
```

Summon it with **Alt+Space**. There is no dock icon — it is a panel, not an
application you switch to.

Without the GUI:

```bash
go run ./cmd/vans -url https://news.ycombinator.com \
  -intent "the title and link of each story"
```

## Requirements

macOS, Go 1.26+, Node, Rust. Accessibility permission for reading and driving
windows; Automation permission for the browser. Any OpenAI-compatible model
endpoint.

## Where the thinking lives

- `AGY.md` — the working record: everything built, everything learned, what is next.
- `docs/decisions.md` — 68 decision records, each with the reasoning and what it rules out.
- `docs/roadmap.md` — the phase plan.
- `docs/backlog.md` — decided but not built.
