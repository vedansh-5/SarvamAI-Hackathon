# VANS — Reading List

Scoped to what's directly needed to build the runtime. Each item is tied to the decision it informs (see [`decisions.md`](decisions.md)). Ordered by when you'll need it.

Every external link below was fetched and confirmed to resolve on 2026-08-04, except the two academic papers, which are cited by title.

---

## Tier 1 — before writing code

### 1. The sdef grammar — `/System/Library/DTDs/sdef.dtd`

**Already on your disk. 8.6 KB. Read this first.**

This is the authoritative XML grammar for every scripting dictionary you'll parse in D-009. It is the schema of the thing you're building a parser for, and it's short enough to read end to end in fifteen minutes.

It already settled one design detail: `access` is declared `(r | w | rw)` — three values. Write-only properties exist, which the earlier read/write classification in D-012 would have mishandled.

Note `man 5 sdef` does **not** work on your machine — the man page ships with full Xcode, which you don't have. The DTD is the substitute, and it's better anyway.

### 2. Getting Started with Chrome DevTools Protocol
<https://github.com/aslushnikov/getting-started-with-cdp>

~1,500–2,000 words, by a Puppeteer maintainer. Covers the targets-and-sessions model, session hierarchy, and the stable-vs-experimental split.

Read it for the **targets and sessions** section specifically. That model is exactly the D-011 problem — an Electron app is a browser with several targets, and knowing which one holds the UI is the whole game.

### 3. CDP `Page` domain reference
<https://chromedevtools.github.io/devtools-protocol/tot/Page/>

Reference, not prose. Read only `Page.navigate` and the lifecycle events. That's all D-018 needs, and the load-state events are how navigation gets verified.

### 4. Electron command line switches
<https://www.electronjs.org/docs/latest/api/command-line-switches>

Short. This is the mechanism the whole Chromium adapter depends on. Worth reading alongside the D-011 finding that Discord silently dropped the flag during a self-update — the docs describe the happy path, not that failure mode.

### 5. Tauri — Embedding External Binaries
<https://v2.tauri.app/develop/sidecar/>

Exactly D-014: how the Go runtime ships inside the Tauri app. Covers `externalBin`, the target-triple naming requirement, spawning from Rust and JS, and permissions. The target-triple detail bites people on first build.

### 6. JavaScript for Automation Release Notes (Apple)
<https://developer.apple.com/library/archive/releasenotes/InterapplicationCommunication/RN-JavaScriptForAutomation/>

The canonical JXA document, and effectively the only good one. Underpins D-016. Archived but still accurate — JXA hasn't changed.

### 7. `man 1 osascript`

101 lines, on your machine. Read it for the flags that matter to D-016: `-l JavaScript`, `-e`, and how results are returned. This is the exact interface the AppleEvents adapter shells out through.

### 8. NSAppleEventsUsageDescription
<https://developer.apple.com/documentation/bundleresources/information-property-list/nsappleeventsusagedescription>

One page. The Info.plist key without which your app cannot send Apple Events at all. Part of D-007's onboarding flow — the string you write here is what the user reads in the permission prompt, so it's UX, not boilerplate.

---

## Tier 2 — the ideas underneath D-002, D-005, D-006

These matter because **the hard parts of VANS are a studied problem, not a novel one.** Reading them will stop you from rediscovering known failure modes at implementation cost.

### 9. "Mining Data Records in Web Pages" — Liu, Grossman, Zhai (KDD 2003)

*Cited by title; I did not verify a stable URL. Search the title — it's widely mirrored.*

Known as the MDR algorithm. **This is essentially the candidate finder from §4.4 of the spec**, published twenty years ago: detect data records by finding repeated similar subtrees among sibling nodes. Read it to get the tree-edit-distance similarity measure and the boundary cases, which is precisely where a naive implementation gets it wrong.

Highest-value item in Tier 2 by a wide margin.

### 10. "Wrapper induction: Efficiency and expressiveness" — Kushmerick (Artificial Intelligence, 2000)

*Cited by title; no verified URL.*

The theory of what an `ExtractionProgram` *is*. Formalizes the class of extractors that can be learned from examples and — more usefully for us — the ones that **cannot**, which tells you where synthesis will fail rather than degrade. Relevant to D-005 and to the failure ladder's rung 2.

Skim it. The taxonomy is the value; the proofs are not.

### 11. Playwright — Locators
<https://playwright.dev/docs/locators>

Practical counterweight to the papers. Playwright's entire locator philosophy exists because raw CSS selectors rot, which is exactly the D-006 problem. You don't have to adopt their approach, but you should know what they concluded and why before committing to bare selectors in the program format.

### 12. `mozilla/readability`
<https://github.com/mozilla/readability>

The algorithm behind the whole-page-dump approach that D-017 rejected. Worth knowing what it does well — its scoring heuristics for "this subtree is the content" are useful even inside a structured extractor, and it's a real fallback if synthesis fails.

---

## Deliberately not on this list

- **Go, SQLite, and Rust general documentation** — you'll pick these up in context; reading them upfront isn't efficient.
- **MCP specification** — D-008 puts it out of V1. Reading it now informs no decision you're making.
- **Accessibility / `AXUIElement` docs** — D-010 defers the tier. Revisit when it comes back.
- **Anything about LLM prompting technique** — the synthesis prompt is small and empirical; you'll learn more from three iterations than from any article.
