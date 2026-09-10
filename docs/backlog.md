# Backlog

Work that is decided but not built. Each entry says what it is, why it is not
done yet, and what has to be true before it can be. Anything that has been
reasoned about at length lands in `decisions.md` instead; this file is the
queue, not the argument.

---

## B-001 — "Try asking" should adapt, not be a fixed list

The four suggestions on the ask screen are hardcoded in `ui/src/main.ts`
(`SUGGESTIONS`). They are correct — each one maps to something the runtime can
actually carry out — but they are the same four every time, forever, for
everybody.

They should be recommendations: drawn from what this person actually asks for,
what they asked for recently, what they are looking at when they summon the
panel, and what is currently cheap because it is already cached.

**Why it is not built.** The signals do not exist yet in a usable form:

- **History.** Nothing records what was asked. The store holds extraction
  programs keyed by page signature and routing decisions keyed by instruction
  shape — both are about *shapes*, deliberately, and neither is a log of
  requests. A history table is a new thing, and it is the most sensitive data
  VANS would hold.
- **Screen context.** The frontmost application and window title already reach
  the panel (`overlay://shown`), but nothing consumes them for routing or
  suggestion. A suggestion that changes because Chrome is frontmost is the
  obvious first version and needs no history at all.
- **Cache state.** The store knows which shapes are warm. A warm shape is a
  suggestion that costs nothing and answers instantly, which is the strongest
  possible reason to offer it — and this signal needs no new storage.

**Order to build it in.** Cache-warm suggestions first (no new data), then
screen context (data already arriving), then history (new storage, needs a
decision about retention and whether it is ever sent anywhere).

**Constraint.** Suggestions must only ever offer what is runnable. The list is
a promise; offering `app.act` before the adapter exists makes the panel lie.
The current list is checked by hand against `runnable` in
`internal/engine/ask.go` — that check should be mechanical before this becomes
dynamic.

---

## B-002 — A destination that shows the answer in another application

The design canvas has four destinations: Here, Clipboard, Note, App. Three
exist. "App" — put this answer into whatever application makes sense, the way
Note puts it into Notes — does not.

The window asks the runtime what destinations exist rather than listing them
(`destinations` in `internal/rpc/server.go`), so this cannot be faked in the
frontend: the option appears the moment a sink is registered for it, and
cannot appear before.

**Why it is not built.** It needs the app-action tier underneath it, which is
the largest remaining piece of work:

1. Capability discovery — **done** (`internal/capability`, D-040). VANS can
   read what all 21 scriptable applications on this machine can do.
2. Program synthesis against a real dictionary, validated and cached by
   instruction shape. Cold: one model call. Warm: none. Same primitive as the
   selector program, one level up.
3. Execution through Apple events, and the confirmation gate for it.

**Open question.** Whether a write that leaves the machine — a message, an
email — gets a heavier confirmation than one that stays on it. The design
answers this implicitly: one confirm screen, one red treatment, the title and
the preview carry the difference. Worth revisiting once there is something
real to confirm.
