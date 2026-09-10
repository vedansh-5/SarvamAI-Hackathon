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

async function runPreflight() {
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

  if (result.ready) {
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
  const url = ($("url") as HTMLInputElement).value.trim();
  const intent = ($("intent") as HTMLInputElement).value.trim();
  const title = ($("title") as HTMLInputElement).value.trim();

  if (!url || !intent) {
    $("status").textContent = "A page and a description are both required.";
    return;
  }

  const run = $("run") as HTMLButtonElement;
  const progress = $("progress");

  run.disabled = true;
  $("status").textContent = "Working…";
  clear(progress);
  progress.hidden = false;

  try {
    const plan = await invoke<Plan>("prepare", { url, intent, title });
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

function renderPlan(plan: Plan) {
  const cost = plan.from_cache
    ? "from cache, no model call"
    : `${plan.synthesis_calls} model call${plan.synthesis_calls === 1 ? "" : "s"}`;

  $("plan-meta").textContent =
    `${plan.rows.length} rows from ${plan.page_url} — ${cost}, ${plan.elapsed_ms} ms. ` +
    `Will be written as “${plan.title}”.`;

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

function renderFailure(e: unknown) {
  const detail = $("failure-detail");
  clear(detail);

  if (!isRpcError(e)) {
    $("failure-title").textContent = "That didn't work";
    $("failure-message").textContent = String(e);
    return;
  }

  $("failure-message").textContent = e.message;

  switch (e.code) {
    case "unresolved": {
      $("failure-title").textContent = "Could not read that page";
      renderUnresolved(detail, e.data as Unresolved);
      break;
    }
    case "no_browser": {
      $("failure-title").textContent = "Chrome is not listening";
      detail.appendChild(
        el("p", "meta", "Start it yourself, then try again — VANS does not launch applications."),
      );
      // The setup screen holds the exact command, so send them there rather
      // than making them remember it.
      const toSetup = el("button", "secondary", "Show me how") as HTMLButtonElement;
      toSetup.addEventListener("click", () => {
        show("preflight");
        runPreflight();
      });
      detail.appendChild(toSetup);
      break;
    }
    case "permission_denied":
      $("failure-title").textContent = "macOS blocked the write";
      detail.appendChild(
        el("pre", "fix", "System Settings › Privacy & Security › Automation, then try again."),
      );
      break;
    case "plan_spent":
      $("failure-title").textContent = "Already written";
      detail.appendChild(
        el(
          "p",
          "meta",
          "That confirmation was already used. VANS never repeats a write, because a write that may have succeeded must not happen twice. Extract again if you want another note.",
        ),
      );
      break;
    default:
      $("failure-title").textContent = "That didn't work";
  }
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

listen<{ for_request: number; message: string }>("engine://progress", (event) => {
  const progress = $("progress");
  progress.hidden = false;
  progress.appendChild(document.createTextNode(event.payload.message + "\n"));
});

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

// Escape dismisses the overlay panel
window.addEventListener("keydown", (e) => {
  if (e.key === "Escape") {
    invoke("dismiss").catch(console.error);
  }
});

$("theme").addEventListener("click", () => applyTheme(!isDark(), true));

$("run").addEventListener("click", extract);
$("recheck").addEventListener("click", runPreflight);
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
  runPreflight();
});
$("again").addEventListener("click", () => show("task"));

// The shell starts light so the first frame is already frosted correctly.
// This re-asserts it either way rather than assuming the two sides agree.
applyTheme(storedTheme() === "dark", false);

show("preflight");
runPreflight();
refreshActiveContext();
