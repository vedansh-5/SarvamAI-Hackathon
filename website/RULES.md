# Rules for the site

These are the standing rules for `summonvans.in`. They hold for every change, whoever makes it. Read them before touching anything in `website/`.

**How this file works**

- A rule is added here in the same pull request as the change that made it necessary, with the date. A rule that no longer holds is removed, not left to be argued with.
- If a rule here and the site disagree, the rule is right and the site is fixed.
- This file holds rules. What the page says, how each section looks and what is left to do are in `docs/plans/2026-10-13-website.md`. The words of the privacy page are in `content/privacy.md`. Nothing is decided anywhere else.
- Keep it short. A rule that needs a paragraph of explanation gets one sentence here and its reasons in `docs/decisions.md`.

## 1. Publishing

- **Merging to `main` publishes.** Cloudflare builds `website/` from `main` and the result is live within a minute or two. There is no staging step after the merge.
- Every change goes through a pull request. Nobody merges their own site change without Vedansh or a second reader having looked.
- Do not change the Cloudflare project, the domain or the DNS. Those are Vedansh's.

## 2. Only what is true

- Every claim on the site is something VANS does today. Check new copy against `AGY.md` and `docs/decisions.md` before writing it.
- No invented numbers, users, quotes, logos, awards or benchmarks. If a section needs something that does not exist yet, leave the section out.
- No claim about privacy, speed, price or what runs on the device unless it is already in `content/privacy.md` or the plan.

## 3. What VANS is

- **VANS teaches first.** You ask how, and it shows you on your own screen while you do it. That it can also do the job is said second, every time. Copy that makes VANS sound like something that takes over the Mac is rewritten.
- Demos start in "Show me".
- Clicking a request in section 3 scrolls to the live panel demo and answers there in-place. Requests do not jump the visitor away to the hero.

## 4. The name

- `VANS`, in capitals, and only that on the main page: not spelled out in the hero, footer, title, meta tags, share image or any demo.
- The full form, "Visual Agent Navigation System", appears once in `content/privacy.md` and once in the terms when they exist. The older "Vedansh's Agent Navigation System" appears nowhere.
- No shoes, vehicles or checkerboard patterns. The footer keeps the line saying VANS is not affiliated with any clothing or footwear brand.

## 5. Colour and shape

- **Volt is VANS.** The pointer, the ring, the primary button, one accent per screen.
- **Red means something outside VANS is about to change.** It appears only in the confirm demo. Never as decoration, never for errors in general.
- The illustration colours (blue, teal, green, violet) are for pictures only: inside drawn windows, as marks, as soft washes. Never for text, and no reds, pinks or oranges among them.
- **Light is the default theme.** A first visit is light whatever the system says. The switch is in the header.
- **Lines, not boxes.** A bordered card is used only for a thing that is a box in VANS: the drawn window, the panel, the confirm card, a sheet. Everything else is type on the page.
- The background dots are a quiet texture. If they can be noticed while reading, they are too strong.
- Colours, sizes, radii and easing come from the tokens at the top of `src/styles.css`. No literal colour is written anywhere else.
- Motion uses the one spring, arrives in about 300ms and never bounces. With reduced motion, everything holds still.
- Focus rings appear on keyboard navigation alone (`:focus-visible`), never left behind after pointer clicks.

## 6. Privacy

- No cookies, no analytics, no trackers, nothing loaded from another company. Fonts are served from the site.
- `content/privacy.md` is Vedansh's text. Render it as written. Do not reword, shorten or add to it.
- The privacy page carries no personal details and no dates: no name, city or address of the person behind VANS, no "last updated".
- **If the site starts receiving anything from a visitor, the privacy page says so in the same pull request.** That includes a form field, a log, a counter.
- Nothing a visitor sends is stored by the site. The Windows form's entries are passed on as an email and kept nowhere else.

## 7. This repository is private; the site is public

- Nothing under `website/` reads from outside it at build time: no imports from `ui/`, `internal/` or `docs/`, no reading of `.env`.
- No secret is written into the repository. The address the Windows list is delivered to is the secret `LIST_TO`, set in the Cloudflare project.
- Before a pull request, search `website/dist` for anything that should not be public. Source maps are not published.

## 8. One home for each fact

A fact that appears in more than one place has one home. Everywhere else imports or quotes it.

| Fact | Home |
|---|---|
| What VANS reads, sends and keeps | `content/privacy.md`; the three lines on the main page are its "short version", word for word |
| The contact address, the key names, the list of requests | `src/facts.ts` |
| Colours, type, radius, motion | the tokens in `src/styles.css` |
| The headline and one-line description | the hero in `index.html`; the title and meta tags repeat it |

A pull request that edits the same fact in two places is a sign one should be importing the other.

## 9. The two actions

- No `mailto:` links. On a computer with no mail application they do nothing.
- "Join the testers" opens the download sheet. When there is a build, only that sheet changes.
- The Windows link opens the form, which posts to `/api/windows`.

## 10. Code

- Vite, plain TypeScript, plain CSS. No UI framework, and no new dependency without a reason written in the pull request.
- The page reads and works with JavaScript off, apart from the demos.
- No comments beyond what a reader could not work out. Names say what a thing is for.
- The function in `worker/` answers only `POST /api/windows`. Its checks live in `worker/handle.ts` and are tested.

## 11. Commits and pull requests

- Commits are signed off (`git commit -s`) and written as Vedansh would write them. No tool or generator attribution in commits, pull requests, comments, this file or the page.
- Pull request bodies are short and end with a "Where to read" list.

## 12. Before every pull request

- `npm run build` passes.
- `node --test worker/handle.test.mjs` passes.
- Looked at 360, 390, 768, 1280 and 1600px wide, in light and in dark.
- Every demo and both sheets work by keyboard alone, with a visible focus ring.
- Script stays under 60KB gzipped.
- The matching item in the plan's "Work" list is ticked.
