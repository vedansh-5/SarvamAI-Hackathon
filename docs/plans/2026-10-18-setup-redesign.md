# Setup, regrouped

The plan for redesigning the Setup screen (`ui/index.html`, section `preflight`;
behaviour in `ui/src/main.ts`). Logged in AGY.md G15 on 2026-10-16. Nothing here
is built yet: Vedansh approves the grouping first.

## Every control Setup has today, in order

| # | Section | Control | What it is for | Shown to |
|---|---|---|---|---|
| 1 | (top) | "Grant these once. This window stays put while you switch apps." | Tells a new person what the screen is | Everyone, always |
| 2 | Permissions | List of macOS permissions, each "Granted" or "Needed" | What VANS may do on this Mac | Everyone |
| 3 | Permissions | Allow all | Asks for every permission in one go | Everyone |
| 4 | Permissions | Reopen VANS | Relaunch so Screen Recording takes effect | After asking for it, until granted |
| 5 | Permissions | Show / Hide | Folds the list once all are granted | When all granted |
| 6 | Ready | List of checks (such as the invite code and the browser), each "Granted" or "Needed" | Whether VANS can work right now | Everyone |
| 7 | Ready | Check again | Runs the checks again | Everyone |
| 8 | Invite code | Code box, Save and check | Lets this copy ask the VANS server | Testers |
| 9 | Model | Key, model name | Own key and model | Own-key builds |
| 10 | Model, More | Base URL, quicker model, fallback model, prices in and out | Another provider, and what each task cost | Own-key builds |
| 11 | Model | Save and test, Remove key | Saves and tries the key; forgets it | Own-key builds |
| 12 | Rules | Never act in | Apps and sites VANS only shows, never acts in | Everyone |
| 13 | Rules | Ask before every step in | Apps and sites where each step needs a yes | Everyone |
| 14 | Rules | Spending limit | Never take a step that pays more than this | Everyone |
| 15 | Rules | Save rules | Saves all three | Everyone |
| 16 | Tell Vedansh | Text box, Send | Feedback to the server | Testers |
| 17 | Record | What VANS has done | The history, with undo | Everyone |
| 18 | Record | What VANS can do, The welcome again | Capability list and the first-run welcome | Everyone |
| 19 | Start | Open VANS when you log in | Login item | Everyone |
| 20 | Start | Quit VANS | Quits | Everyone |
| 21 | Updates | Check for updates, Restart now | Updater | Everyone |
| 22 | Learning | Learn from what I do, See, Clear | Habit notes and their removal | Everyone |
| 23 | Appearance | Accent colours, Glass, Frosted, Dark | Look of the panel | Everyone |
| 24 | (bottom) | Done | Back to the ask field | Everyone |

## Proposed grouping

Three groups, in this order. A group whose items are all done folds to one line,
the way Permissions folds today (D-254).

**1. Getting started** (rows 1 to 11). Folds to "VANS is ready" when every
permission and check passes. The line in row 1 shows only while something is
still needed.

**2. How VANS behaves** (rows 12 to 15, 22). Rules and learning: what VANS may
do, and what it remembers.

**3. About this copy** (rows 16 to 21, 23). Feedback, history, start, updates,
look, quit.

Done stays at the bottom, always visible.

## Fixes inside the groups

- **Tags say what is true.** "Granted" only for permissions. The invite code
  says "Saved", the browser "Connected", other checks "Ready".
- **One invite code.** The Ready check for the code links to the code box
  instead of being a second place for it. Once saved, the box folds to "Saved"
  with a Change link.
- **Rules read as settings.** Each rule is a line with its current value ("Never
  act in: none") and an Edit link; the box opens only when editing. Each saves on
  its own, so there is no separate Save rules.

## Checks before calling it done (G11)

Setup is a screen in the panel, so rules 1, 4, 5, 7 and 12 apply. Rule 5 matters
most: Allow all and every "Open" button must still step aside for System
Settings.

## Built

First as the `setup` trial (D-260), then given to everyone in 0.4.0 (D-263).
