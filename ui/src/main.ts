import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";

// --- shapes mirrored from internal/rpc/protocol.go ---

interface Check {
  name: string;
  label: string;
  ok: boolean;
  detail: string;
  fix?: string;
  required: boolean;
}

interface Preflight {
  ready: boolean;
  checks: Check[];
}

interface Plan {
  plan_id: string;
  page_url: string;
  signature: string;
  title: string;
  fields: string[];
  rows: Record<string, string>[];
  synthesis_calls: number;
  from_cache: boolean;
  elapsed_ms: number;
  destination: string;
  needs_commit: boolean;
  kind: string;
  decision_calls: number;
}

interface Destination {
  name: string;
  describe: string;
  touches_world: boolean;
}

interface Unroutable {
  instruction: string;
  why: string;
  can: string[];
}

interface Receipt {
  id: string;
  title: string;
  verified: boolean;
}

interface Attempt {
  source: string;
  root: string;
  span: number;
  reasons: string[];
}

interface Candidate {
  selector: string;
  span: number;
  record_count: number;
  samples: string[];
}

interface Unresolved {
  page_url: string;
  signature: string;
  attempts: Attempt[];
  candidates: Candidate[];
}

interface RpcError {
  code: string;
  message: string;
  data?: unknown;
}

interface ScreenContext {
  app_name: string;
  bundle_id: string;
  window_title: string;
  pid: number;
  bounds: { x: number; y: number; width: number; height: number };
  frame_path?: string;
  image_base64?: string;
}

// --- helpers ---

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;

/// Screens that send the user somewhere else, and so must outlive them going.
///
/// Preflight prints a command to run in a terminal. With dismiss-on-blur and
/// nothing else, clicking the terminal destroys the instructions telling you
/// to click the terminal.
const PINNED = new Set(["preflight"]);

function show(id: string) {
  for (const s of document.querySelectorAll("section")) {
    (s as HTMLElement).hidden = s.id !== id;
  }
  invoke("set_pinned", { pinned: PINNED.has(id) }).catch(console.error);
  setReading(id !== "task");
}

/// Every value that reaches the DOM goes through a text node.
///
/// Rows are scraped from arbitrary pages, so they are attacker-controlled in
/// the ordinary case. Building markup from them with innerHTML would let a
/// page put script into this window — the same reasoning that keeps the note
/// renderer from linking javascript: URLs (D-033).
function el(tag: string, className?: string, text?: string): HTMLElement {
  const n = document.createElement(tag);
  if (className) n.className = className;
  if (text !== undefined) n.textContent = text;
  return n;
}

function clear(node: HTMLElement) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

function prettyField(name: string): string {
  return name
    .split("_")
    .map((p) => (p ? p[0].toUpperCase() + p.slice(1) : p))
    .join(" ");
}

function isRpcError(e: unknown): e is RpcError {
  return typeof e === "object" && e !== null && "code" in e && "message" in e;
}

// --- preflight ---

/// `advance` is false when the user asked to see this screen.
///
/// Preflight sends you on to the task form the moment everything is ready,
/// which is right on launch and wrong when Setup was clicked deliberately: the
/// screen appeared and vanished in the same frame, so the button looked dead.
async function runPreflight(advance = true) {
  const checks = $("checks");
  clear(checks);
  checks.appendChild(el("p", "meta", "Checking…"));

  let result: Preflight;
  try {
    result = await invoke<Preflight>("preflight");
  } catch (e) {
    clear(checks);
    checks.appendChild(
      el("p", "bad", isRpcError(e) ? e.message : String(e)),
    );
    return;
  }

  clear(checks);
  for (const c of result.checks) {
    const box = el("div", `check ${c.ok ? "ok" : "bad"}`);
    box.appendChild(el("div", "check-label", `${c.ok ? "✓" : "✗"}  ${c.label}`));
    box.appendChild(el("div", "check-detail", c.detail));
    if (!c.ok && c.fix) {
      box.appendChild(el("pre", "fix", c.fix));
    }
    checks.appendChild(box);
  }

  if (result.ready && advance) {
    show("task");
  }
}

async function checkNotes() {
  const out = $("notes-result");
  clear(out);
  out.appendChild(el("p", "meta", "Asking Notes…"));

  try {
    const c = await invoke<Check>("check_notes");
    clear(out);
    const box = el("div", `check ${c.ok ? "ok" : "bad"}`);
    box.appendChild(el("div", "check-label", `${c.ok ? "✓" : "✗"}  ${c.label}`));
    box.appendChild(el("div", "check-detail", c.detail));
    if (c.fix) box.appendChild(el("pre", "fix", c.fix));
    out.appendChild(box);
  } catch (e) {
    clear(out);
    out.appendChild(el("p", "bad", isRpcError(e) ? e.message : String(e)));
  }
}

// --- the task ---

let currentPlan: Plan | null = null;

async function extract() {
  const instruction = ($("intent") as HTMLInputElement).value.trim();
  const destination = chosen;

  if (!instruction) {
    $("status").textContent = "Say what you want.";
    return;
  }

  const run = $("run") as HTMLButtonElement;
  const progress = $("progress");

  run.disabled = true;
  $("status").textContent = "Working…";
  clear(progress);
  progress.hidden = false;
  progress.textContent = "Thinking…";

  try {
    const plan = await invoke<Plan>("prepare", { instruction, destination });
    currentPlan = plan;
    renderPlan(plan);
    show("plan");
  } catch (e) {
    renderFailure(e);
    show("failure");
  } finally {
    run.disabled = false;
    $("status").textContent = "";
  }
}

/// What a column is for, worked out from the data rather than declared.
///
/// The extractor names fields in the user's language — "link", "url", "thumb",
/// "image" — and nothing upstream promises a vocabulary. So the values decide:
/// a field whose contents are image URLs is an image wherever it came from.
type Role = "image" | "link" | "text";

const IMAGE_URL = /^https?:\/\/[^\s]+\.(?:png|jpe?g|gif|webp|avif|svg)(?:[?#]|$)/i;
const IMAGE_NAME = /(image|img|thumb|photo|picture|avatar|icon|logo)/i;
const LINK_NAME = /(link|url|href|permalink)/i;

/// Decided across the whole column, not per cell.
///
/// One row with an empty description must not make that column a different
/// kind from its neighbours, and a single stray URL in a text column must not
/// turn every other row's prose into a link.
function roleOf(field: string, rows: Record<string, string>[]): Role {
  const values = rows.map((r) => (r[field] ?? "").trim()).filter(Boolean);
  if (values.length === 0) return "text";

  const share = (test: (v: string) => boolean) =>
    values.filter(test).length / values.length;

  if (IMAGE_NAME.test(field) && share((v) => /^https?:/i.test(v)) > 0.5) return "image";
  if (share((v) => IMAGE_URL.test(v)) > 0.5) return "image";
  if (LINK_NAME.test(field) && share((v) => /^https?:/i.test(v)) > 0.5) return "link";
  return "text";
}

/// A coloured initial standing in for a site's icon.
///
/// The hue comes from the domain, so the same site is the same colour every
/// time and a familiar result is recognisable before it is read.
function domainTile(host: string): HTMLElement | null {
  if (!host) return null;

  let hash = 0;
  for (let i = 0; i < host.length; i++) {
    hash = (hash * 31 + host.charCodeAt(i)) >>> 0;
  }

  const tile = el("div", "card-tile", host[0].toUpperCase());
  tile.style.setProperty("--hue", String(hash % 360));
  return tile;
}

function hostOf(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return "";
  }
}

/// A card per record: a picture if there is one, a headline that goes
/// somewhere if there is a link, the site it came from, and the rest as prose.
///
/// A table was the wrong shape. It is right for dense columns of short values
/// and wrong for the thing people actually ask for — a handful of results with
/// a sentence of description each, which a table renders as a wall of clipped
/// cells. The table is still there behind a toggle, because for prices and
/// scores it remains the better view.
function renderCards(into: HTMLElement, fields: string[], rows: Record<string, string>[]) {
  clear(into);

  const roles = new Map(fields.map((f) => [f, roleOf(f, rows)]));
  const imageField = fields.find((f) => roles.get(f) === "image");
  const linkField = fields.find((f) => roles.get(f) === "link");
  const textFields = fields.filter((f) => roles.get(f) === "text");
  const titleField = textFields[0];
  const bodyFields = textFields.slice(1);

  for (const row of rows) {
    const card = el("div", "card");

    const src = imageField ? (row[imageField] ?? "").trim() : "";
    const href = linkField ? (row[linkField] ?? "").trim() : "";
    const host = hostOf(href);

    if (src) {
      const img = el("img", "card-image") as HTMLImageElement;
      img.src = src;
      img.loading = "lazy";
      img.alt = "";
      // A dead URL would otherwise leave a broken-image glyph in every card
      // that had one, which looks worse than no picture at all. Falls back to
      // the tile rather than to nothing, so the row keeps its shape.
      img.addEventListener("error", () => {
        const tile = domainTile(host);
        if (tile) img.replaceWith(tile);
        else img.remove();
      });
      card.appendChild(img);
    } else {
      // No picture in the data. A letter tile from the domain rather than a
      // fetched favicon: an icon service would be told every domain that ever
      // appeared in a result, which is a decent picture of what someone has
      // been searching for, sent somewhere they did not ask for.
      const tile = domainTile(host);
      if (tile) card.appendChild(tile);
    }

    const body = el("div", "card-body");
    const titleText = titleField ? (row[titleField] ?? "").trim() : href;

    if (titleText) {
      if (href) {
        const a = el("a", "card-title", titleText) as HTMLAnchorElement;
        // Kept as a real href so it reads as a link and shows on hover, but
        // the navigation is handled rather than followed: a target="_blank"
        // inside a Tauri webview has nowhere to go, and letting the webview
        // navigate itself would replace the panel with the page.
        a.href = href;
        a.addEventListener("click", (e) => {
          e.preventDefault();
          invoke("open_url", { url: href }).catch(console.error);
        });
        body.appendChild(a);
      } else {
        body.appendChild(el("div", "card-title", titleText));
      }
    }

    if (host) body.appendChild(el("div", "card-host", host));

    for (const f of bodyFields) {
      const v = (row[f] ?? "").trim();
      if (v) body.appendChild(el("p", "card-text", v));
    }

    card.appendChild(body);
    into.appendChild(card);
  }
}

function renderPlan(plan: Plan) {
  const calls = plan.synthesis_calls + plan.decision_calls;
  const cost = calls === 0
    ? "no model calls"
    : `${calls} model call${calls === 1 ? "" : "s"}`;

  // A plan bound for the panel is the answer; one bound anywhere else is a
  // proposal. Saying "review before writing" over something already delivered
  // would be asking for permission that was never needed.
  const gate = $("gate");
  const gateNote = $("gate-note");
  const doneRow = $("done-row");

  gate.hidden = !plan.needs_commit;
  gateNote.hidden = !plan.needs_commit;
  doneRow.hidden = plan.needs_commit;

  $("plan-heading").textContent = plan.needs_commit ? "Review before writing" : "Here it is";

  // Said the way a person would say it. How many model calls it took and how
  // the page was fingerprinted are real and worth keeping, but they are the
  // runtime talking about itself, and they were the first line of the result.
  const n = plan.rows.length;
  const seconds = (plan.elapsed_ms / 1000).toFixed(1);
  let meta = `${n} result${n === 1 ? "" : "s"} from ${hostOf(plan.page_url) || plan.page_url}`;
  meta += calls === 0 ? ` · instant, remembered` : ` · ${seconds}s`;
  if (plan.needs_commit) {
    meta += ` · about to go to ${plan.destination}`;
    ($("confirm") as HTMLButtonElement).textContent = `Write to ${plan.destination}`;
  }
  $("plan-meta").textContent = meta;
  $("plan-meta").title = `${cost}, ${plan.elapsed_ms} ms — page signature ${plan.signature}`;

  renderCards($("cards"), plan.fields, plan.rows);

  const table = $("rows");
  clear(table);

  const head = el("tr");
  for (const f of plan.fields) {
    head.appendChild(el("th", undefined, prettyField(f)));
  }
  table.appendChild(head);

  for (const row of plan.rows) {
    const tr = el("tr");
    for (const f of plan.fields) {
      tr.appendChild(el("td", undefined, row[f] ?? ""));
    }
    table.appendChild(tr);
  }
}

async function confirmWrite() {
  if (!currentPlan) return;

  const button = $("confirm") as HTMLButtonElement;
  button.disabled = true;
  button.textContent = "Writing…";

  try {
    const receipt = await invoke<Receipt>("commit", { planId: currentPlan.plan_id });
    $("done-message").textContent = receipt.verified
      ? `“${receipt.title}” is in Notes, read back and confirmed.`
      : `“${receipt.title}” was written, but could not be read back to confirm it.`;
    show("done");
  } catch (e) {
    renderFailure(e);
    show("failure");
  } finally {
    button.disabled = false;
    button.textContent = "Write to Notes";
    currentPlan = null;
  }
}

// --- failures ---

const PLAIN: Record<string, { title: string; message: string }> = {
  unresolved: {
    title: "Could not read that page",
    message: "The page loaded, but nothing on it matched what you asked for.",
  },
  unroutable: {
    title: "Not sure what you meant",
    message: "Try naming a site, or asking for a search.",
  },
  no_browser: {
    title: "Chrome isn't ready",
    message:
      "VANS works through a Chrome that is already running, with at least one tab open. It will not start one for you.",
  },
  permission_denied: {
    title: "macOS blocked the write",
    message: "Automation access has not been granted yet.",
  },
  plan_spent: {
    title: "Already written",
    message: "That confirmation was used. VANS never repeats a write, so ask again for another.",
  },
};

function renderFailure(e: unknown) {
  const detail = $("failure-detail");
  clear(detail);

  if (!isRpcError(e)) {
    $("failure-title").textContent = "That didn't work";
    $("failure-message").textContent = "Something went wrong inside VANS.";
    detail.appendChild(raw(String(e)));
    return;
  }

  const plain = PLAIN[e.code];
  $("failure-title").textContent = plain?.title ?? "That didn't work";
  $("failure-message").textContent = plain?.message ?? "Something went wrong inside VANS.";

  switch (e.code) {
    case "unresolved": {
      renderUnresolved(detail, e.data as Unresolved);
      break;
    }
    case "unroutable": {
      const data = e.data as Unroutable;
      if (data?.can?.length) {
        detail.appendChild(el("h3", undefined, "What VANS can do"));
        const list = el("ul");
        for (const c of data.can) list.appendChild(el("li", undefined, c));
        detail.appendChild(list);
      }
      break;
    }
    case "no_browser": {
      const toSetup = el("button", "secondary", "Show me how") as HTMLButtonElement;
      toSetup.addEventListener("click", () => {
        show("preflight");
        runPreflight(false);
      });
      detail.appendChild(toSetup);
      break;
    }
    case "permission_denied":
      detail.appendChild(
        el("pre", "fix", "System Settings › Privacy & Security › Automation, then try again."),
      );
      break;
  }

  detail.appendChild(raw(e.message));
}

function raw(text: string): HTMLElement {
  const box = el("details", "raw");
  box.appendChild(el("summary", undefined, "Details"));
  box.appendChild(el("pre", undefined, text));
  return box;
}

/// Rung 3 belongs on screen in full: what was tried, which predicate failed,
/// and what the page actually contains. "Extraction failed" gives a person
/// nothing to do next.
function renderUnresolved(into: HTMLElement, data: Unresolved) {
  if (!data) return;

  if (data.attempts?.length) {
    into.appendChild(el("h3", undefined, "What was tried"));
    for (const [i, a] of data.attempts.entries()) {
      const box = el("div", "attempt");
      box.appendChild(
        el(
          "div",
          "attempt-head",
          `${i + 1}. ${a.source} — root ${a.root}${a.span > 1 ? `, spanning ${a.span} elements` : ""}`,
        ),
      );
      const list = el("ul");
      for (const r of a.reasons) list.appendChild(el("li", undefined, r));
      box.appendChild(list);
      into.appendChild(box);
    }
  }

  if (data.candidates?.length) {
    into.appendChild(el("h3", undefined, "What the page does contain"));
    const list = el("ul", "candidates");
    for (const c of data.candidates) {
      const item = el(
        "li",
        undefined,
        `${c.selector} — ${c.record_count} records${c.span > 1 ? `, ${c.span} elements each` : ""}`,
      );
      if (c.samples?.length) {
        item.appendChild(el("div", "sample", c.samples[0]));
      }
      list.appendChild(item);
    }
    into.appendChild(list);
    into.appendChild(
      el("p", "meta", "Try describing one of these instead."),
    );
  }
}

async function checkScreenRecording() {
  const out = $("screen-result");
  clear(out);
  out.appendChild(el("p", "meta", "Checking Screen Recording permission…"));

  try {
    const granted = await invoke<boolean>("check_screen_recording");
    clear(out);
    if (granted) {
      const box = el("div", "check ok");
      box.appendChild(el("div", "check-label", "✓  Screen Recording"));
      box.appendChild(
        el("div", "check-detail", "Permission granted. VANS can capture display context."),
      );
      out.appendChild(box);
    } else {
      const box = el("div", "check bad");
      box.appendChild(el("div", "check-label", "✗  Screen Recording"));
      box.appendChild(
        el("div", "check-detail", "Permission not granted. Click below to request access."),
      );
      const promptBtn = el(
        "button",
        "secondary",
        "Request Screen Recording Access",
      ) as HTMLButtonElement;
      promptBtn.addEventListener("click", () => {
        invoke("request_screen_recording").catch(console.error);
      });
      box.appendChild(promptBtn);
      out.appendChild(box);
    }
  } catch (e) {
    clear(out);
    out.appendChild(el("p", "bad", isRpcError(e) ? e.message : String(e)));
  }
}

/// The window title is whatever the frontmost application decided to call
/// itself, and when that application is a browser it is whatever the page
/// decided — `document.title` is writable by any script on it. Building this
/// banner with innerHTML let a visited page put markup into the panel just by
/// being frontmost when the hotkey was pressed. Text nodes, like everything
/// else that reaches this DOM.
function updateActiveContext(ctx?: ScreenContext) {
  const label = $("active-app-label");
  if (!label) return;
  clear(label);

  if (!ctx || !ctx.app_name || ctx.app_name === "Unknown") {
    label.textContent = "Active: Entire Desktop";
    return;
  }

  label.appendChild(document.createTextNode("Active: "));
  label.appendChild(el("strong", undefined, ctx.app_name));
  if (ctx.window_title) {
    label.appendChild(document.createTextNode(" — "));
    label.appendChild(el("em", undefined, ctx.window_title));
  }
}

async function refreshActiveContext() {
  try {
    const ctx = await invoke<ScreenContext>("get_screen_context", {
      includeScreenshot: false,
    });
    updateActiveContext(ctx);
  } catch (e) {
    console.error("failed to get screen context", e);
  }
}

// --- destinations ---

/// The window asks the runtime what destinations exist rather than listing
/// them, so adding a sink makes it appear here without a frontend change — and
/// so the window can never offer one that is not there.
let destinations: Destination[] = [];
let chosen = "";

async function loadDestinations() {
  const group = $("destination");
  try {
    destinations = await invoke<Destination[]>("destinations");
  } catch (e) {
    console.error("failed to load destinations", e);
    return;
  }

  clear(group);
  for (const d of destinations) {
    const label = el("label", undefined, d.name) as HTMLLabelElement;
    label.setAttribute("role", "radio");
    label.dataset.name = d.name;
    label.addEventListener("click", () => pickDestination(d.name));
    group.appendChild(label);
  }

  // The panel is first and safest, and starting anywhere else would mean a
  // hurried Return lands on something that changes the world.
  pickDestination(destinations[0]?.name ?? "");
}

function pickDestination(name: string) {
  chosen = name;

  for (const el of $("destination").children) {
    const label = el as HTMLElement;
    const on = label.dataset.name === name;
    label.classList.toggle("on", on);
    label.setAttribute("aria-checked", String(on));
  }

  const d = destinations.find((x) => x.name === name);
  if (!d) return;

  // Said plainly, before anything runs. Whether this changes something outside
  // VANS is the only consequential thing on the screen, and it should not have
  // to be learned by trying it.
  $("destination-note").textContent = d.touches_world
    ? `${d.describe} You will be asked to confirm first.`
    : d.describe;
}

// --- appearance ---

/// How the panel looks over your work: how opaque its surface is, and whether
/// what is behind it is blurred.
///
/// Two settings rather than one, because they answer different questions. The
/// blur decides whether you can make out what is behind the panel; the tint
/// decides how strongly the panel's own surface sits over it. Frosted-and-faint
/// and clear-and-solid are both reasonable, and one slider expresses neither.
const LOOK_KEY = "vans.look";

interface Look {
  tint: number; // 10..100, the panel surface's own opacity
  blur: boolean;
}

const DEFAULT_LOOK: Look = { tint: 62, blur: true };

function storedLook(): Look {
  try {
    const raw = localStorage.getItem(LOOK_KEY);
    if (!raw) return DEFAULT_LOOK;
    const v = JSON.parse(raw) as Partial<Look>;
    return {
      // Clamped on the way in. A stored value from an older build, or one
      // edited by hand, must not be able to make the panel invisible.
      tint: Math.min(100, Math.max(10, Number(v.tint) || DEFAULT_LOOK.tint)),
      blur: v.blur !== false,
    };
  } catch {
    return DEFAULT_LOOK;
  }
}

let look = DEFAULT_LOOK;

const READABLE = 0.94;

let reading = false;
let adjusting = false;

function paintTint() {
  const set = look.tint / 100;
  const solid = reading && !adjusting ? Math.max(set, READABLE) : set;
  document.documentElement.style.setProperty("--tint", String(solid));
}

function setReading(next: boolean) {
  if (next === reading) return;
  reading = next;
  paintTint();
}

function applyLook(next: Look, persist: boolean) {
  look = next;

  paintTint();

  ($("opacity") as HTMLInputElement).value = String(next.tint);
  $("opacity-value").textContent = `${next.tint}%`;
  ($("blur") as HTMLInputElement).checked = next.blur;

  // The blur is an AppKit layer the stylesheet cannot reach, so it has to be
  // told separately — and told the theme, since the material it uses differs
  // between light and dark.
  invoke("set_blur", { blurred: next.blur, dark: isDark() }).catch(console.error);

  if (persist) {
    try {
      localStorage.setItem(LOOK_KEY, JSON.stringify(next));
    } catch {
      // Not worth surfacing: it still applies for this session.
    }
  }
}

// --- theme ---

/// Light unless the user has said otherwise.
///
/// The stylesheet's bare :root is the light palette, so a first run, a cleared
/// store, or a failed read all land on light without this code having to run
/// at all. Only dark is ever written to the document.
const THEME_KEY = "vans.theme";

/// localStorage throws outright in some contexts rather than returning null,
/// and a panel that will not open because it could not remember a colour is a
/// worse failure than a panel in the wrong colour.
function storedTheme(): string | null {
  try {
    return localStorage.getItem(THEME_KEY);
  } catch {
    return null;
  }
}

function applyTheme(dark: boolean, persist: boolean) {
  const root = document.documentElement;
  if (dark) {
    root.setAttribute("data-theme", "dark");
  } else {
    // Removed rather than set to "light": the bare :root block is the light
    // palette, so absence is the default rather than a third state.
    root.removeAttribute("data-theme");
  }

  $("theme").textContent = dark ? "Light" : "Dark";
  $("theme").setAttribute(
    "aria-label",
    dark ? "Switch to the light theme" : "Switch to the dark theme",
  );

  if (persist) {
    try {
      localStorage.setItem(THEME_KEY, dark ? "dark" : "light");
    } catch {
      // Not worth surfacing: the theme still applies for this session.
    }
  }

  // The frosting behind the panel is an AppKit layer the stylesheet cannot
  // reach, and it carries its own light/dark rendering. A light panel over a
  // dark blur reads as a dark panel with a light patch in it.
  invoke("set_theme", { dark }).catch(console.error);
}

function isDark(): boolean {
  return document.documentElement.getAttribute("data-theme") === "dark";
}

// --- the window follows the content ---

const panel = document.querySelector(".panel") as HTMLElement;
const panelBody = document.querySelector(".panel-body") as HTMLElement;

/// Tells the shell how tall the panel wants to be.
///
/// A summoned panel is not a document viewer: it should be the size of what it
/// has to say. At a fixed height, preflight — the one screen whose entire job
/// is to be read — came up scrolled into its own middle with the heading cut
/// off the top, and nothing on screen suggested there was anything above it.
///
/// Only .panel-body is measured. .panel stretches to fill the window, so
/// measuring it would report the window's own height back to the window, and
/// the panel could grow but never shrink again.
let requested = 0;

function fitWindow() {
  const style = getComputedStyle(panel);
  const chrome =
    parseFloat(style.paddingTop) +
    parseFloat(style.paddingBottom) +
    parseFloat(style.borderTopWidth) +
    parseFloat(style.borderBottomWidth);

  const height = Math.ceil(panelBody.getBoundingClientRect().height + chrome);

  // Sub-pixel churn from font loading and scrollbar reflow would otherwise
  // resize the window continuously.
  if (Math.abs(height - requested) < 2) return;
  requested = height;

  invoke("resize_panel", { height }).catch(console.error);
}

new ResizeObserver(fitWindow).observe(panelBody);

// --- wiring ---

/// Progress is a status line, not a log.
///
/// The runtime narrates itself in its own terms — page signatures, cache hits,
/// programs rejected — and all of it is true and worth having when something
/// goes wrong. On screen, mid-request, it was a monospace dump of internals
/// where a person wanted to know whether anything was happening. The latest
/// line only, in plain type; the full trace is still on stderr for whoever
/// needs it.
listen<{ for_request: number; message: string }>("engine://progress", (event) => {
  const progress = $("progress");
  progress.hidden = false;
  progress.textContent = humanise(event.payload.message);
});

/// Turns the runtime's own vocabulary into something worth reading.
///
/// Deliberately a small set of rewrites rather than a general one: anything
/// unrecognised passes through untouched, so a message this does not know
/// about is still shown rather than swallowed.
function humanise(message: string): string {
  const url = message.match(/page (\S+) signature/);
  if (url) return `Reading ${hostOf(url[1]) || url[1]}…`;

  if (message.startsWith("cache hit")) {
    const n = message.match(/(\d+) rows/);
    return n ? `Found ${n[1]} — no thinking needed` : "Found it — no thinking needed";
  }
  if (message.startsWith("route cache hit")) return "Understood that already";
  if (message.startsWith("instruction shape")) return "Working out what you meant…";
  if (message.startsWith("routed to")) return "Got it — going to work";
  if (message.includes("synthesized a working program")) return "Worked out how to read it";
  if (message.includes("rejected")) return "That did not work — trying again";
  return message;
}

listen<ScreenContext | undefined>("overlay://shown", (event) => {
  if (event.payload) {
    updateActiveContext(event.payload);
  } else {
    refreshActiveContext();
  }

  // A panel that comes back scrolled where it was left looks broken — the top
  // of it is the heading that says what you are looking at.
  panel.scrollTop = 0;

  // Only when it is actually on screen. During preflight the task form is
  // hidden, and focusing into a hidden section takes the keyboard nowhere.
  const task = $("task");
  if (!task.hidden) {
    const intent = $("intent") as HTMLInputElement;
    intent.focus();
    intent.select();
  }
});

window.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!$("look-panel").hidden) {
    setLookOpen(false);
    return;
  }
  invoke("dismiss").catch(console.error);
});

// Cards read better for a handful of results with prose; the table is better
// for many short values. Which one is wanted depends on the answer, so it is
// the user's call and it is remembered.
const VIEW_KEY = "vans.view";

function applyView(table: boolean, persist: boolean) {
  $("cards").hidden = table;
  $("table-wrap").hidden = !table;
  $("view-toggle").textContent = table ? "Cards" : "Table";
  if (persist) {
    try {
      localStorage.setItem(VIEW_KEY, table ? "table" : "cards");
    } catch {
      // Applies for this session either way.
    }
  }
}

function tableView(): boolean {
  try {
    return localStorage.getItem(VIEW_KEY) === "table";
  } catch {
    return false;
  }
}

$("view-toggle").addEventListener("click", () => applyView($("table-wrap").hidden, true));

$("ask-again").addEventListener("click", () => show("task"));
$("to-task").addEventListener("click", () => show("task"));

// Return runs it. A summoned panel you have to reach for the mouse in is a
// window, not a shortcut.
$("intent").addEventListener("keydown", (e) => {
  if ((e as KeyboardEvent).key === "Enter") extract();
});

function setLookOpen(open: boolean) {
  $("look-panel").hidden = !open;
  document.documentElement.toggleAttribute("data-look-open", open);
  adjusting = open;
  paintTint();
}

$("look").addEventListener("click", (e) => {
  e.stopPropagation();
  setLookOpen($("look-panel").hidden);
});

document.addEventListener("click", () => setLookOpen(false));
$("look-panel").addEventListener("click", (e) => e.stopPropagation());

$("opacity").addEventListener("input", (e) => {
  applyLook({ ...look, tint: Number((e.target as HTMLInputElement).value) }, true);
});

$("blur").addEventListener("change", (e) => {
  applyLook({ ...look, blur: (e.target as HTMLInputElement).checked }, true);
});

$("theme").addEventListener("click", () => applyTheme(!isDark(), true));

$("run").addEventListener("click", extract);
$("recheck").addEventListener("click", () => runPreflight(false));
$("check-screen").addEventListener("click", checkScreenRecording);
$("check-notes").addEventListener("click", checkNotes);
$("confirm").addEventListener("click", confirmWrite);
$("cancel").addEventListener("click", () => {
  currentPlan = null;
  invoke("dismiss").catch(console.error);
});
$("back").addEventListener("click", () => show("task"));
$("setup").addEventListener("click", () => {
  show("preflight");
  runPreflight(false);
});
$("again").addEventListener("click", () => show("task"));

// The shell starts light so the first frame is already frosted correctly.
// This re-asserts it either way rather than assuming the two sides agree.
applyTheme(storedTheme() === "dark", false);
applyLook(storedLook(), false);
applyView(tableView(), false);

show("preflight");
runPreflight();
refreshActiveContext();
loadDestinations();
