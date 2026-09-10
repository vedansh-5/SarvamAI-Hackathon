# VANS — design brief

A prompt for a design tool, and the reference products worth stealing from.
Written 2026-09-10, when the panel worked but still looked like developer
tooling.

---

## The prompt

> Design a macOS overlay panel called **VANS** — an AI assistant summoned with
> ⌥Space over whatever the user is working in, then dismissed. Think Spotlight
> or Raycast, not a window you leave open.
>
> **What it does.** You type one plain sentence — "the top stories on Hacker
> News", "search for X on Google", "reply to the last message in Telegram" —
> and it works out what you meant, does it, and shows you the result. It can
> read web pages, search, and (soon) act inside other Mac applications. Repeat
> requests are instant and free, because it remembers how it did them.
>
> **The one idea the design must carry.** Most of what you ask for you only
> want to *read* — that changes nothing and needs no permission. Some of what
> you ask for *changes the world* — writes a note, replaces your clipboard,
> sends a message. Those two must never look alike. A confirmation in front of
> something harmless teaches people to dismiss confirmations, which is exactly
> what must not happen in front of the ones that matter.
>
> **Constraints.**
> - Floats over a live blur of the user's desktop (NSVisualEffectView), so
>   surfaces are tints, not fills. Transparency is user-adjustable.
> - ~720pt wide, height grows with content, up to 85% of the display.
> - Light and dark, light by default. Not tied to the system setting.
> - Keyboard-first: summon, type, Return, Escape. The mouse is optional.
> - Appears in under a second and is often dismissed in three.
>
> **Screens to design.**
> 1. **Ask** — one field, and a choice of where the answer goes (show it here /
>    clipboard / a note / an app). Empty state should suggest what is possible
>    without becoming a menu.
> 2. **Working** — a second to twenty seconds. Must not look frozen, must not
>    narrate internals.
> 3. **Results** — a list of records: title, source, description, sometimes a
>    picture. Search results, products, articles. Needs a dense alternative for
>    numbers.
> 4. **Confirm** — "I am about to write this to Notes." The only irreversible
>    moment in the product. Should feel weighty without being alarming.
> 5. **Didn't understand** — says so, and says what it *can* do. Must not be a
>    dead end.
> 6. **Setup** — permissions and a command to run elsewhere. The one screen the
>    user must be able to read while doing something in another app.
>
> **Tone.** Quiet, fast, sure of itself. The interface should disappear behind
> the answer. No mascot, no chat bubbles, no gradients-for-their-own-sake.
>
> Deliver: the six screens, light and dark, plus a type scale, spacing rhythm,
> and the treatment for glass surfaces and controls.

---

## References worth studying

**Closest in shape**

| Product | What to take |
|---|---|
| **Raycast** | The category's best craft. Result rows, inline actions, how a command palette stays dense without feeling cramped. |
| **ChatGPT for macOS** | Also ⌥Space-summoned. Watch how the field is the whole interface and everything else appears only when earned. |
| **Spotlight** | The baseline everyone's muscle memory already has. Deviating from it costs something. |
| **Highlight / Cluely** | Screen-aware assistants — how to show "I can see what you're looking at" without it feeling surveillant. |
| **Clicky** | The one this pivoted towards. Acting on your behalf, drawn over your work. |

**Craft to steal, different shape**

| Product | What to take |
|---|---|
| **Linear** | Command palette, and the best restraint in software. Type scale and spacing especially. |
| **Superhuman** | Keyboard-first as an actual position, not a feature list. |
| **Arc Max** | "Ask on this page" — an assistant that lives inside something else. |
| **Granola** | Warm and quiet where most AI products are loud. |
| **Things 3** | Native-Mac feel done better than Apple manages lately. |

**Worth looking at for what to avoid**

Most AI desktop apps ship a chat transcript. VANS is not a conversation — it is
a question and an answer, and a transcript would imply a history the runtime
does not keep and a back-and-forth it does not need.
