# Ghost cursor — design

Status: proposed. Not built. Read `docs/ideas.md` (Clicky-like on-screen
experience, and Acting on the person's behalf) first.

## What it is

A drawn pointer that glides to a control and lands on it *before* anything
happens to that control. On a drive run the sequence per step is: the ghost
arrives, the control gets a ring, a short pause, then the action.

It is the base for the rest of the on-screen work — hand-drawn marks, narration
captions, the step trail — because all of those hang off knowing where the ghost
is and when it got there.

## Why it is worth building first

Two things at once, which is why it beats the other items in that section.

**It is the look.** A cursor that travels is the difference between "the screen
changed" and "something is working here". Nothing else on the list changes the
feel as much per line of code.

**It is a gate that costs nothing.** The pause between landing and acting is a
veto: Escape during it cancels the step. VANS's stated tension is the
confirmation gate against smoothness (`ideas.md`, Principles in tension) — Clicky
is fluid because it skips approvals. This is the answer to that: the approval
stops being a dialog you dismiss and becomes a moment you let pass. Per step, and
only where a step is about to touch something.

## The decision that matters

**The real cursor must not move.**

Today a drive step clicks through `CGEvent` (`postClick`), which takes over the
person's mouse. That is the thing that makes an agent feel like it has pushed you
out of your chair.

The accessibility API can press a control directly — `AXPress`, and setting a
value for typing. It does not move the pointer, and it works on a window that is
not in front. So: **act through accessibility, draw the ghost to show where a
mouse would have gone, and leave the real cursor where the person left it.**
`CGEvent` stays as the fallback for controls that publish no action.

This is what makes the ghost honest rather than decorative. It is not a cartoon
of the click — it is the only pointer involved.

Consequences to plan for, not to discover:

- `VerifyTargets`' covered-window check (D-112) is about a click at a screen
  point. An `AXPress` does not care what is on top, so the guard's meaning
  changes for AX-acted steps and it should not simply be skipped — being covered
  still means the person cannot see what VANS is doing.
- "Landed" is still read back from the window (D-004). Unchanged.

## Shape of the work

**Draw it in the coach overlay window.** `coach.html` is already transparent,
click-through, full-screen, on all Spaces, and positioned from accessibility
geometry with a screen-to-window conversion (`cover()` in `coach.rs`). A second
window would repeat all of it. The ghost is one more element beside `#corners`,
`#arrow`, `#ripple`.

**Motion:** `requestAnimationFrame`, a curved path, ease-out landing. A straight
linear tween reads as a bug.

**The drive loop has to announce a step before performing it.** The coach already
has this shape — `RunGuide` takes `show func(GuideStep)`. `runDrive` has no
callback at all, so it needs the equivalent: a notification carrying the target
rectangle, the verb and the description, then a wait, then the action. That
notification is the whole engine-side change.

**Escape during the pause cancels the step.** The run is already cancellable
through its context; this wires Escape in the overlay to that.

**Pause length:** 300–400 ms to start, and it should be a named constant rather
than a literal, because it is the dial between "too fast to veto" and "slow".

## Known hazard, from `ideas.md`

Accessibility points are global across all displays. The overlay needs
per-display coordinates and the right backing scale factor. `cover()` does some
of this already for the ring. **Test on two displays with different scale factors
early** — a ghost that glides to the wrong monitor is worse than no ghost.

## Deliberately not in the first version

- The coach. It already points and the person does the clicking; the ghost is for
  runs where VANS acts.
- Hand-drawn marks, narration captions, the step trail. All of them sit on top of
  this and none of them are blocked by leaving them out.
- "Taking over" — pausing when the person moves the real mouse. Wanted, but it is
  a second input path and belongs after the first one works.

## Done when

A drive run on TextEdit shows the ghost travelling to each control, landing, and
the action following; the person's own cursor never moves; and Escape during the
pause stops the run with the step not performed.
