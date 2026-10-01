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
  mode?: string;
  mode_why?: string;
  decision_calls: number;
  action: string;
  app: string;
  answer: string;
  answer_cites: number[];
  answer_calls: number;
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

  // Only present for a run that stepped through a task on screen. Absent and
  // zero mean different things: a run that asked the model nothing is the good
  // case, and no counts at all means this was not that kind of run.
  work?: StepWork;
}

interface StepWork {
  steps: number;
  replayed: number;
  calls: number;
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
// Screens that survive a click somewhere else. Preflight sends you to a terminal;
// coaching sends you to the control it is pointing at — and a panel that closes
// on that click takes Next and Stop with it, leaving the coach running with
// nothing to drive it.
const PINNED = new Set(["preflight", "guiding", "question"]);

// The application the current coached run is in. Sent with every step so the
// coach knows whose window it is drawing over.
let coachedApp = "";

// The application a confirmed action is running in, while it runs. Set for the
// length of the commit so Escape knows there is something to stop and the ghost
// knows whose window it is in.
let actingIn: string | null = null;

const ICONS: Record<string, string> = {
  rss: '<path d="M4 11a9 9 0 0 1 9 9"/><path d="M4 4a16 16 0 0 1 16 16"/><circle cx="5" cy="19" r="1.6"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>',
  chart: '<line x1="18" y1="20" x2="18" y2="10"/><line x1="12" y1="20" x2="12" y2="4"/><line x1="6" y1="20" x2="6" y2="14"/>',
  pencil: '<path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
  page: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z"/><path d="M14 2v6h6"/><path d="M8 13h8"/><path d="M8 17h5"/>',
};

interface Suggestion {
  icon: string;
  text: string;
  destination?: string;
}

const SUGGESTIONS: Suggestion[] = [
  { icon: "page", text: "what is this page about" },
  { icon: "rss", text: "the top stories on news.ycombinator.com" },
  { icon: "search", text: "search for noise-cancelling headphones under $200" },
  { icon: "chart", text: "search for today's biggest NASDAQ movers" },
  { icon: "pencil", text: "the top stories on news.ycombinator.com", destination: "notes" },
];

let runtimeSuggestions: Suggestion[] | null = null;

/// The runtime decides what to offer, because only it knows what this build can
/// carry out. The built-in list is the fallback for a runtime that has not
/// answered yet, never a second opinion.
function currentSuggestions(): Suggestion[] {
  return runtimeSuggestions?.length ? runtimeSuggestions : SUGGESTIONS;
}

async function loadSuggestions(appName?: string) {
  try {
    const got = await invoke<Suggestion[]>("suggestions", { appName: appName ?? "" });
    if (got?.length) runtimeSuggestions = got;
  } catch (e) {
    console.error("failed to load suggestions", e);
  }
  renderSuggestions($("suggestions"), currentSuggestions());
}

function icon(name: string): SVGElement {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("width", "15");
  svg.setAttribute("height", "15");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "2");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.innerHTML = ICONS[name] ?? "";
  return svg;
}

function renderSuggestions(into: HTMLElement, items: Suggestion[]) {
  clear(into);
  for (const s of items) {
    const writes = !!s.destination && destinationWrites(s.destination);
    const row = el("button", writes ? "suggestion write" : "suggestion");
    const wrap = el("span", "icon");
    wrap.appendChild(icon(s.icon));
    row.appendChild(wrap);
    row.appendChild(el("span", undefined, s.destination ? `${s.text} → ${label(s.destination)}` : s.text));
    row.addEventListener("click", () => {
      ($("intent") as HTMLInputElement).value = s.text;
      if (s.destination) pickDestination(s.destination);
      show("ask");
      extract();
    });
    into.appendChild(row);
  }
}

let toastTimer = 0;

function toast(message: string) {
  const node = $("toast");
  node.textContent = message;
  node.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    node.hidden = true;
    fitWindow();
  }, 2200);
}

function show(id: string) {
  for (const s of document.querySelectorAll("section")) {
    (s as HTMLElement).hidden = s.id !== id;
  }
  invoke("set_pinned", { pinned: PINNED.has(id) }).catch(console.error);
  adjusting = id === "preflight";
  setReading(id !== "ask");
  paintTint();
  $("write-bar").hidden = id !== "confirm";
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
    checks.appendChild(checkRow(c));
  }

  if (result.ready && advance) {
    show("ask");
  }
}

function checkRow(c: Check): HTMLElement {
  const box = el("div", c.ok ? "check ok" : "check");
  box.appendChild(el("span", "check-dot"));

  const text = el("div", "check-text");
  text.appendChild(el("div", "check-label", c.label));
  text.appendChild(el("div", "check-detail", c.detail));
  if (!c.ok && c.fix) text.appendChild(el("pre", "fix", c.fix));
  box.appendChild(text);

  box.appendChild(el("span", "check-tag", c.ok ? "Granted" : "Needed"));
  return box;
}

async function checkNotes() {
  const out = $("notes-result");
  clear(out);
  out.appendChild(el("p", "meta", "Asking Notes…"));

  try {
    const c = await invoke<Check>("check_notes");
    clear(out);
    out.appendChild(checkRow(c));
  } catch (e) {
    clear(out);
    out.appendChild(el("p", "bad", isRpcError(e) ? e.message : String(e)));
  }
}

// --- the task ---

let currentPlan: Plan | null = null;

/// Whether a restyle is sitting on a page right now.
///
/// The panel offers to take it off only while there is something to take off,
/// because a button that does nothing is worse than no button.
let styleApplied = false;

function showClearStyle(on: boolean) {
  styleApplied = on;
  const button = $("clear-style");
  if (button) button.hidden = !on;
}

async function clearStyle() {
  if (!styleApplied) return;
  try {
    await invoke("clear_style");
    showClearStyle(false);
    toast("Highlighting cleared");
  } catch (e) {
    renderFailure(e);
    show("failure");
  }
}

type Mode = "show" | "do";

const MODE_KEY = "vans.mode";

// Kept in step with asksToBeShown and askedToHaveItDone in the engine, which is
// what decides. This only moves the switch while someone types, so they can see
// which way their sentence will go before they send it.
const SHOW_START =
  /^\s*(please\s+)?(how\b|where\b|which (button|menu|setting|option)\b|(can|could|would) you (show|teach|tell|guide|walk|help)\b|(show|teach|tell|guide|walk|help|coach) (me|us)\b|(explain|demonstrate)\b|i (want|need|would like|'d like) to (learn|know|see|understand)\b|what('s| is) the (way|best way|steps?)\b)/i;
const SHOW_ANYWHERE =
  /\b(step by step|show me how|teach me|walk me through|how (do|can|should|would) (i|we)|how to)\b/i;
const DO_ANYWHERE = /\b(do (it|this|that)( for me)?|for me|on my behalf|go ahead and|yourself)\b/i;

function pinnedMode(): Mode | null {
  try {
    const saved = localStorage.getItem(MODE_KEY);
    return saved === "show" || saved === "do" ? saved : null;
  } catch {
    return null;
  }
}

let pinned: Mode | null = pinnedMode();

function modeFor(text: string): { mode: Mode | null; note: string } {
  const typed = text.trim();
  const shown = SHOW_START.test(typed) || SHOW_ANYWHERE.test(typed);
  const done = !shown && DO_ANYWHERE.test(typed);

  if (pinned) {
    if (pinned === "do" && shown) {
      return { mode: "show", note: "Your words ask to be shown, so this one is coached." };
    }
    if (pinned === "show" && done) {
      return { mode: "do", note: "Your words ask to have it done, so this one is done for you." };
    }
    return { mode: pinned, note: "Your choice. Click it again to let VANS pick from your words." };
  }
  if (typed === "") {
    return { mode: null, note: "VANS picks from your words. Click one to set it yourself." };
  }
  return {
    mode: shown ? "show" : "do",
    note: "Picked from your words. Click the other to change it.",
  };
}

function renderMode() {
  const { mode, note } = modeFor(($("intent") as HTMLInputElement).value);
  for (const button of Array.from($("mode").querySelectorAll("button"))) {
    const on = button.getAttribute("data-mode") === mode;
    button.classList.toggle("on", on);
    button.setAttribute("aria-checked", on ? "true" : "false");
  }
  $("mode-note").textContent = note;
}

function pin(next: Mode | null) {
  pinned = next;
  try {
    if (next) localStorage.setItem(MODE_KEY, next);
    else localStorage.removeItem(MODE_KEY);
  } catch {
    // Applies for this session either way.
  }
  renderMode();
}

let lastInstruction = "";
let switchTo: string | null = null;

function showHeadsup(where: "guiding" | "confirm", plan: Plan) {
  const box = $(`${where}-headsup`);
  const coached = plan.mode === "show";
  const right = (where === "guiding") === coached;
  box.hidden = !plan.mode || !right;
  if (box.hidden) return;
  const why = plan.mode_why ? `, because ${plan.mode_why}` : "";
  $(`${where}-headsup-text`).textContent = coached
    ? `Showing you how${why}.`
    : `Doing it for you${why}.`;
}

async function extract(sendAs?: string, text?: string) {
  const instruction = (text ?? ($("intent") as HTMLInputElement).value).trim();
  const destination = chosen;

  if (!instruction) {
    ($("intent") as HTMLInputElement).focus();
    return;
  }

  $("working-query").textContent = instruction;
  $("progress").textContent = "Working";
  setProgress(6);
  show("working");

  try {
    const plan = await invoke<Plan>("prepare", {
      instruction,
      destination,
      context: activeContext,
      mode: sendAs ?? pinned ?? "",
    });
    lastInstruction = instruction;
    setProgress(100);
    currentPlan = plan;
    if (plan.kind === "app.guide") {
      await showTheWay(plan);
    } else if (plan.needs_commit) {
      renderConfirm(plan);
      show("confirm");
    } else {
      renderResults(plan);
      show("results");
    }
  } catch (e) {
    renderFailure(e);
    show(isRpcError(e) && e.code === "unroutable" ? "notunderstood" : "failure");
  }
}

interface GuideResult {
  steps: number;
  stopped?: boolean;
  work?: StepWork;
}

/// Walks the person through the steps, pointing at each control in turn.
///
/// The runtime pushes one step at a time and waits for them to act before
/// working out the next, so this returns only once they are done or it gives up.
async function showTheWay(plan: Plan) {
  let finished = false;
  let rerunAs: string | null = null;
  switchTo = null;
  showHeadsup("guiding", plan);
  coachedApp = plan.app ?? "";
  $("guiding-where").textContent = plan.app ? `Showing you — ${plan.app}` : "Showing you";
  $("guiding-step").textContent = "Looking at the window…";
  show("guiding");

  // Narrow now, out of the way once there is something to see. The steps belong
  // on the application the person is working in, not in a window over the thing
  // being pointed at — but leaving before the first step is drawn just looks
  // like VANS closed, which is what it looked like.
  invoke("compact_panel", { compact: true }).catch(console.error);

  try {
    const res = await invoke<GuideResult>("guide", { planId: plan.plan_id });
    if (res?.stopped) {
      rerunAs = switchTo;
      show("ask");
    } else {
      // A tick on the last control, which takes itself away. Finished and stopped
      // should not look the same.
      finished = true;
      invoke("finish_pointer").catch(console.error);
      const how = howItWent(res?.work);
      toast(how ? `That is all of it · ${how}` : "That is all of it");
      show("ask");
    }
  } catch (e) {
    renderFailure(e);
    show("failure");
  } finally {
    invoke("compact_panel", { compact: false }).catch(console.error);
    if (!finished) {
      invoke("clear_pointer").catch(console.error);
    }
    // Back only now, with something to say.
    invoke("summon").catch(console.error);
  }

  switchTo = null;
  if (rerunAs) await extract(rerunAs, lastInstruction);
}


// Escape during a coached run. The panel is put away while coaching, so its own
// Escape handler never hears it; the shell holds the key for as long as the coach
// is on screen and reports it here.
listen("coach://stop", async () => {
  // A run that is acting on screen stops the same way a coached one does. The
  // result comes back through the commit, which reports it as stopped.
  if (actingIn !== null) {
    invoke("stop_guide").catch(console.error);
    return;
  }
  if ($("guiding").hidden) return;
  invoke("compact_panel", { compact: false }).catch(console.error);
  await invoke("stop_guide").catch(console.error);
  await invoke("clear_pointer").catch(console.error);
  show("ask");
});

// A step of a confirmed action, announced just before it is performed. Drawn with
// the same ghost cursor and ring the coach uses — the difference is only that here
// VANS does the step, and the ghost shows where, a moment before it happens.
listen<{
  params: {
    number: number;
    label: string;
    x: number;
    y: number;
    w: number;
    h: number;
    do?: string;
    detail?: string;
  };
}>("engine://drive_step", (event) => {
  if (actingIn === null) return;
  const s = event.payload.params;
  invoke("point_at", {
    x: s.x,
    y: s.y,
    w: s.w,
    h: s.h,
    label: s.label,
    step: `Step ${s.number}`,
    verb: s.do ?? "",
    detail: s.detail ?? "",
    appName: actingIn,
    mode: "drive",
  }).catch(console.error);
});

// The shell saw the person click what the coach pointed at. That is the step
// done, told to the runtime immediately rather than inferred from the window a
// poll later.
listen("coach://acted", () => {
  if ($("guiding").hidden) return;
  invoke("advance_guide").catch(console.error);
});

let cameFrom = "ask";

async function answerQuestion(answer: string) {
  const said = answer.trim();
  if (!said) return;
  ($("question-other") as HTMLInputElement).value = "";
  await invoke("answer_question", { answer: said }).catch(console.error);
  show(cameFrom);
  if (actingIn !== null || cameFrom === "guiding") {
    invoke("dismiss").catch(console.error);
  }
}

listen<{ params: { question: string; options?: string[] } }>("engine://question", (event) => {
  const q = event.payload.params;
  const visible = Array.from(document.querySelectorAll("section")).find(
    (s) => !(s as HTMLElement).hidden,
  );
  cameFrom = visible && visible.id !== "question" ? visible.id : "ask";

  $("question-text").textContent = q.question;
  const box = $("question-options");
  box.replaceChildren();
  for (const option of q.options ?? []) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "suggestion";
    button.textContent = option;
    button.addEventListener("click", () => void answerQuestion(option));
    box.append(button);
  }

  show("question");
  invoke("summon").catch(console.error);
  window.setTimeout(() => ($("question-other") as HTMLInputElement).focus(), 60);
});

$("question-send").addEventListener("click", () => {
  void answerQuestion(($("question-other") as HTMLInputElement).value);
});
$("question-other").addEventListener("keydown", (e) => {
  if ((e as KeyboardEvent).key === "Enter") {
    void answerQuestion(($("question-other") as HTMLInputElement).value);
  }
});
$("question-stop").addEventListener("click", async () => {
  await invoke("stop_guide").catch(console.error);
});

listen("coach://yes", () => {
  if ($("guiding").hidden) return;
  invoke("answer_guide", { yes: true }).catch(console.error);
});

listen("coach://no", () => {
  if ($("guiding").hidden) return;
  invoke("answer_guide", { yes: false }).catch(console.error);
});

listen("coach://next", () => {
  if (actingIn !== null) {
    invoke("resume_drive").catch(console.error);
    return;
  }
  if ($("guiding").hidden) return;
  invoke("advance_guide").catch(console.error);
});

listen<{ params: { label?: string; ask?: string } }>("engine://drive_paused", (event) => {
  if (actingIn === null) return;
  const p = event.payload?.params ?? {};
  invoke("pause_pointer", { ask: p.ask ?? "", label: p.label ?? "" }).catch(console.error);
});

listen<{
  params: {
    number: number;
    label: string;
    x: number;
    y: number;
    w: number;
    h: number;
    do?: string;
    detail?: string;
    offer?: boolean;
  };
}>(
  "engine://guide",
  (event) => {
    const s = event.payload.params;
    $("guiding-step").textContent = `${s.number}. ${s.label}`;

    // The step is on the screen now, so the panel can leave. Every step after
    // the first finds it already gone.
    invoke("dismiss").catch(console.error);

    invoke("point_at", {
      x: s.x,
      y: s.y,
      w: s.w,
      h: s.h,
      label: s.label,
      step: `Step ${s.number}`,
      verb: s.do ?? "",
      detail: s.detail ?? "",

      // So the coach can tell when the person has gone to another application
      // and take itself off the screen until they come back.
      appName: coachedApp,
      mode: "coach",
      offer: s.offer ?? false,
    }).catch(console.error);
  },
);

let progressAt = 0;

function setProgress(percent: number) {
  progressAt = percent;
  $("progress-fill").style.width = `${percent}%`;
}

function advanceProgress() {
  setProgress(progressAt + (92 - progressAt) * 0.35);
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

function renderResults(plan: Plan) {
  if (plan.action) {
    $("results-kicker").textContent = plan.app
      ? `${plan.app} — nothing to change`
      : "nothing to change";
    clear($("cards"));
    $("cards").appendChild(el("p", "card-text", plan.action));
    clear($("rows"));
    return;
  }

  renderAnswer(plan);

  const n = plan.rows.length;
  const source = hostOf(plan.page_url) || plan.page_url;
  $("results-kicker").textContent =
    `${n} result${n === 1 ? "" : "s"} from ${source}${plan.decision_calls + plan.synthesis_calls === 0 ? " · remembered" : ""}`;
  $("results-kicker").title = timing(plan);

  renderCards($("cards"), plan.fields, plan.rows);

  const table = $("rows");
  clear(table);

  const head = el("tr");
  for (const f of plan.fields) head.appendChild(el("th", undefined, prettyField(f)));
  table.appendChild(head);

  for (const row of plan.rows) {
    const tr = el("tr");
    for (const f of plan.fields) tr.appendChild(el("td", undefined, row[f] ?? ""));
    table.appendChild(tr);
  }
}

/// The answer, and what it rests on.
///
/// The citations are shown rather than kept as provenance: an answer written
/// off a page is only worth as much as the page, and the fastest way to judge
/// it is to see which results it used. Clicking one scrolls to it.
function renderAnswer(plan: Plan) {
  const box = $("answer");
  const cites = $("answer-cites");
  clear(cites);

  if (!plan.answer) {
    box.hidden = true;
    return;
  }

  $("answer-text").textContent = plan.answer;
  box.hidden = false;

  for (const n of plan.answer_cites ?? []) {
    const row = plan.rows[n - 1];
    if (!row) continue;

    const label = hostOf(firstLink(row, plan.fields)) || String(n);
    const chip = el("button", "cite", label) as HTMLButtonElement;
    chip.type = "button";
    chip.title = row[plan.fields[0]] ?? "";
    chip.addEventListener("click", () => {
      const card = $("cards").children[n - 1] as HTMLElement | undefined;
      card?.scrollIntoView({ behavior: "smooth", block: "center" });
      card?.classList.add("lit");
      setTimeout(() => card?.classList.remove("lit"), 1200);
    });
    cites.appendChild(chip);
  }
}

function firstLink(row: Record<string, string>, fields: string[]): string {
  for (const f of fields) {
    const v = (row[f] ?? "").trim();
    if (/^https?:/i.test(v)) return v;
  }
  return "";
}

function renderConfirm(plan: Plan) {
  showHeadsup("confirm", plan);
  $("confirm-tag").textContent = plan.action ? "ACTION" : "WRITE";
  if (plan.action) {
    $("confirm-title").textContent = plan.app ? `Do this in ${plan.app}?` : "Do this?";
    $("confirm-preview").textContent = plan.action;
  } else {
    const n = plan.rows.length;
    $("confirm-title").textContent =
      `Write ${n} result${n === 1 ? "" : "s"} to ${label(plan.destination)}?`;
    $("confirm-preview").textContent = documentText(plan);
  }
  ($("commit") as HTMLButtonElement).textContent = plan.action
    ? "Confirm & do"
    : "Confirm & write";
}

function documentText(plan: Plan): string {
  const lines = [plan.fields.map(prettyField).join("\t")];
  for (const row of plan.rows) {
    lines.push(
      plan.fields
        .map((f) => (row[f] ?? "").replace(/[\t\n\r]/g, " "))
        .join("\t"),
    );
  }
  return lines.join("\n");
}

function timing(plan: Plan): string {
  const calls = plan.synthesis_calls + plan.decision_calls;
  const cost = calls === 0 ? "no model calls" : `${calls} model call${calls === 1 ? "" : "s"}`;
  return `${cost}, ${plan.elapsed_ms} ms — page signature ${plan.signature}`;
}

async function confirmWrite() {
  if (!currentPlan) return;

  const acting = !!currentPlan.action;
  const where = acting && currentPlan.app ? currentPlan.app : label(currentPlan.destination);
  const button = $("commit") as HTMLButtonElement;
  button.disabled = true;
  button.textContent = acting ? "Doing…" : "Writing…";

  // Step aside while VANS acts in another application. The panel floats above
  // everything, so left where it was it sat on top of the very control a step
  // was about to reach — seen live on TextEdit, where the step failed with
  // "nothing is on screen" because the thing on screen was VANS. It comes back
  // with the result either way.
  //
  // The pause is for a remembered run, which acts almost at once: without it the
  // first step can reach for its control before the window server has taken the
  // panel off the screen.
  if (acting) {
    actingIn = currentPlan.app ?? where;
    await invoke("dismiss").catch(console.error);
    await new Promise((resolve) => setTimeout(resolve, 150));
  }

  try {
    const receipt = await invoke<Receipt>("commit", { planId: currentPlan.plan_id });
    if (acting) {
      actingIn = null;
      invoke("finish_pointer").catch(console.error);
      await invoke("summon").catch(console.error);
    }
    if (currentPlan.kind === "page.style") {
      showClearStyle(!/\b(clear|remove|undo|reset|revert)\b/i.test(currentPlan.title ?? ""));
    }
    if (acting) {
      const how = howItWent(receipt.work);
      const done = receipt.verified ? `Done in ${where}` : `Done in ${where}, unverified`;
      toast(how ? `${done} · ${how}` : done);
    } else {
      toast(receipt.verified ? `Saved to ${where}` : `Written to ${where}, unverified`);
    }
    ($("intent") as HTMLInputElement).value = "";
    show("ask");
  } catch (e) {
    if (acting) {
      actingIn = null;
      invoke("clear_pointer").catch(console.error);
      await invoke("summon").catch(console.error);
    }
    renderFailure(e);
    show("failure");
  } finally {
    button.disabled = false;
    button.textContent = "Confirm & write";
    currentPlan = null;
  }
}

// What a run through a task on screen cost, in words.
//
// Shown because the claim that repeating a task is free is only worth making if
// the number is in front of the person being told it. A run that reused nothing
// still reports its calls: knowing the first run was expensive is what makes the
// second one meaning something.
function howItWent(work?: StepWork): string {
  if (!work || work.steps === 0) return "";

  const asked = work.calls === 1 ? "1 model call" : `${work.calls} model calls`;

  if (work.replayed === 0) return `${plural(work.steps, "step")}, ${asked}`;
  if (work.replayed === work.steps && work.calls === 0) {
    return `all ${plural(work.steps, "step")} from last time, nothing asked`;
  }
  return `${work.replayed} of ${plural(work.steps, "step")} from last time, ${asked}`;
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

// --- failures ---

const PLAIN: Record<string, { title: string; message: string }> = {
  unresolved: {
    title: "Could not read that page",
    message: "The page loaded, but nothing on it matched what you asked for.",
  },
  cannot_do_there: {
    title: "Can't do that there",
    message: "VANS understood you, but what's in front can't do it. The reason is below.",
  },
  unroutable: {
    title: "Not sure what you meant",
    message: "Try naming a site, or asking for a search.",
  },
  no_browser: {
    title: "Chrome isn't ready",
    message:
      "VANS works through the Chrome you are already using, with at least one window open. It will not start one for you.",
  },
  browser_scripting_off: {
    title: "Chrome won't let VANS read the page",
    message:
      "One menu item, once. Chrome keeps this off by default so that scripts cannot read what you are signed in to.",
  },
  permission_denied: {
    title: "macOS blocked the write",
    message: "Automation access has not been granted yet.",
  },
  coach_stopped: {
    title: "The steps stopped",
    message: "",
  },
  app_not_open: {
    title: "That's done somewhere else",
    message:
      "The application this belongs in is not open. VANS does not open applications for you \u2014 open it and ask again.",
  },
  no_coach_here: {
    title: "Can't point at things in a browser",
    message:
      "A browser window publishes no controls to the accessibility API, so there is nothing for VANS to draw on. It can work on the page directly instead.",
  },
  model_silent: {
    title: "The model didn't answer",
    message:
      "The request reached the model and it never replied. Nothing is wrong on this end — ask again.",
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

  if (e.code === "unroutable") {
    const data = e.data as Unroutable;
    $("notunderstood-why").textContent = data?.why ?? "";
    $("notunderstood-why").hidden = !data?.why;
    renderSuggestions($("can-do"), currentSuggestions());
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
    case "no_browser": {
      const toSetup = el("button", "btn secondary", "Show me how") as HTMLButtonElement;
      toSetup.addEventListener("click", () => {
        show("preflight");
        runPreflight(false);
      });
      detail.appendChild(toSetup);
      break;
    }
    case "browser_scripting_off":
      detail.appendChild(
        el("pre", "fix", "Chrome › View › Developer › Allow JavaScript from Apple Events"),
      );
      break;
    case "permission_denied":
      detail.appendChild(
        el("pre", "fix", "System Settings › Privacy & Security › Automation, then try again."),
      );
      break;
    case "coach_stopped": {
      // The engine's sentence is the explanation, not a stack trace to hide
      // behind a disclosure triangle. It says what the window was missing.
      const where = (e.data as { app?: string })?.app;
      $("failure-title").textContent = where ? `Stopped in ${where}` : "The steps stopped";
      $("failure-message").textContent = withoutThePrefix(e.message);
      return;
    }
    case "app_not_open": {
      const data = e.data as { app?: string; why?: string };
      if (data?.app) {
        $("failure-title").textContent = `That's done in ${data.app}`;
        $("failure-message").textContent = data.why
          ? `It ${data.why.replace(/^it /i, "")}. Open ${data.app} and ask again.`
          : `Open ${data.app} and ask again. VANS does not open applications for you.`;
      }
      break;
    }
    case "no_coach_here":
    case "model_silent": {
      const fix = (e.data as { fix?: string })?.fix;
      if (fix) detail.appendChild(el("pre", "fix", fix));
      break;
    }
  }

  detail.appendChild(raw(e.message));
}

/// The engine names itself at the front of every message, which is right in a
/// log and noise in a sentence being read by the person it happened to.
function withoutThePrefix(message: string): string {
  const said = message.replace(/^engine:\s*/, "").trim();
  return said.charAt(0).toUpperCase() + said.slice(1);
}

interface Deed {
  id: number;
  at: number;
  kind: string;
  where: string;
  asked: string;
  did: string;
  changed?: string;
  verified: boolean;
  undoable: boolean;
  undone: boolean;
}

/// What VANS has done to this machine.
///
/// Every other list in the panel is something it learned or offers to do. This
/// one is what it actually changed, which is the thing a person is entitled to
/// see without having to ask nicely for it.
async function showHistory() {
  show("history");
  const list = $("deeds");
  clear(list);
  list.appendChild(el("p", "lede", "Reading…"));

  let deeds: Deed[] = [];
  try {
    const result = await invoke<{ deeds?: Deed[] }>("history");
    deeds = result?.deeds ?? [];
  } catch (e) {
    clear(list);
    list.appendChild(el("p", "lede", `Could not read it: ${String(e)}`));
    return;
  }

  clear(list);
  if (deeds.length === 0) {
    list.appendChild(el("p", "lede", "Nothing yet. VANS has not changed anything."));
    return;
  }

  for (const d of deeds) {
    const row = el("div", "setup-row");

    const head = el("div", "deed-head");
    head.appendChild(el("span", "deed-what", d.did || d.asked || d.kind));
    head.appendChild(el("span", "deed-when", whenItWas(d.at)));
    row.appendChild(head);

    const marks: string[] = [d.kind];
    if (d.where) marks.push(d.where);
    // Said plainly, both ways round. "Verified" alone leaves someone guessing
    // what the absence of it means.
    marks.push(d.verified ? "seen to happen" : "not confirmed");
    if (d.undone) marks.push("put back");
    else if (!d.undoable) marks.push("cannot be undone");
    row.appendChild(el("div", "deed-marks", marks.join(" · ")));

    if (d.changed) row.appendChild(el("div", "deed-changed", d.changed));

    // The offer replaces the words: a button that puts it back says "can be put
    // back" better than the phrase does, and saying both is saying it twice.
    if (d.undoable && !d.undone) {
      const put = el("button", "btn quiet deed-undo", "Put it back") as HTMLButtonElement;
      put.addEventListener("click", () => undoDeed(d.id, put));
      row.appendChild(put);
    }

    list.appendChild(row);
  }
}

async function undoDeed(id: number, button: HTMLButtonElement) {
  button.disabled = true;
  button.textContent = "Putting it back…";
  try {
    await invoke("undo", { id });
    // Read the list again rather than editing the row in place: whether
    // something is still undoable is the engine's answer, not the panel's.
    await showHistory();
  } catch (e) {
    button.disabled = false;
    button.textContent = "Put it back";
    const said = isRpcError(e) ? e.message : String(e);
    button.insertAdjacentElement("afterend", el("div", "deed-changed", said));
  }
}

function whenItWas(at: number): string {
  if (!at) return "";
  const then = new Date(at * 1000);
  const ago = Math.round((Date.now() - then.getTime()) / 1000);
  if (ago < 60) return "just now";
  if (ago < 3600) return `${Math.round(ago / 60)} min ago`;
  if (ago < 86400) return `${Math.round(ago / 3600)} h ago`;
  return then.toLocaleDateString();
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
    const row = checkRow({
      name: "screen",
      label: "Screen Recording",
      ok: granted,
      detail: granted
        ? "VANS can see what you are looking at."
        : "Not granted yet. Ask for it below.",
      required: false,
    });
    out.appendChild(row);

    if (!granted) {
      const promptBtn = el("button", "btn secondary", "Request access") as HTMLButtonElement;
      promptBtn.addEventListener("click", () => {
        invoke("request_screen_recording").catch(console.error);
      });
      out.appendChild(promptBtn);
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
let activeContext: ScreenContext | null = null;

function updateActiveContext(ctx?: ScreenContext) {
  activeContext = ctx ?? null;
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
    const button = el("button", undefined, label(d.name)) as HTMLButtonElement;
    button.type = "button";
    button.setAttribute("role", "radio");
    button.dataset.name = d.name;
    if (d.touches_world) button.appendChild(el("span", "write-dot"));
    button.addEventListener("click", () => pickDestination(d.name));
    group.appendChild(button);
  }

  // The panel is first and safest, and starting anywhere else would mean a
  // hurried Return lands on something that changes the world.
  pickDestination(destinations[0]?.name ?? "");
}

const LABELS: Record<string, string> = { panel: "Here", clipboard: "Clipboard", notes: "Note" };

function label(name: string): string {
  return LABELS[name] ?? prettyField(name);
}

function destinationWrites(name: string): boolean {
  return !!destinations.find((d) => d.name === name)?.touches_world;
}

function pickDestination(name: string) {
  chosen = name;

  for (const node of $("destination").children) {
    const button = node as HTMLElement;
    const on = button.dataset.name === name;
    button.classList.toggle("on", on);
    button.setAttribute("aria-checked", String(on));
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
let frosted: boolean | null = null;
let frostedDark: boolean | null = null;

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

  const dark = isDark();
  if (next.blur !== frosted || dark !== frostedDark) {
    frosted = next.blur;
    frostedDark = dark;
    invoke("set_blur", { blurred: next.blur, dark }).catch(console.error);
  }

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

  ($("dark") as HTMLInputElement).checked = dark;

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
  const text = humanise(event.payload.message);
  $("progress").textContent = text;
  if (!$("guiding").hidden) {
    $("guiding-step").textContent = text;
  }
  const commitBtn = $("commit") as HTMLButtonElement | null;
  if (commitBtn && commitBtn.disabled) {
    commitBtn.textContent = text;
  }
  advanceProgress();
});

function humanise(message: string): string {
  const url = message.match(/page (\S+) signature/);
  if (url) return `Reading ${hostOf(url[1]) || url[1]}`;

  if (message.startsWith("cache hit")) {
    const n = message.match(/(\d+) rows/);
    return n ? `Found ${n[1]} — no thinking needed` : "Found it — no thinking needed";
  }
  if (message.startsWith("route cache hit")) return "Understood that already";
  if (message.startsWith("instruction shape")) return "Working out what you meant";
  if (message.startsWith("routed to")) return "Got it — going to work";
  if (message.includes("synthesized a working program")) return "Worked out how to read it";
  if (message.startsWith("step ") || message.startsWith("finished after") || message.startsWith("working out step")) return message;
  if (message.includes("rejected")) return "That did not work — trying again";
  return message;
}

listen<ScreenContext | undefined>("overlay://shown", (event) => {
  if (event.payload) {
    updateActiveContext(event.payload);
  } else {
    refreshActiveContext();
  }

  void loadSuggestions(event.payload?.app_name);

  panel.scrollTop = 0;

  const ask = $("ask");
  if (!ask.hidden) {
    const intent = $("intent") as HTMLInputElement;
    intent.focus();
    intent.select();
  }
});

window.addEventListener("keydown", (e) => {
  if (e.key !== "Escape") return;
  if (!$("question").hidden) {
    invoke("stop_guide").catch(console.error);
    return;
  }
  if (!$("guiding").hidden) {
    invoke("compact_panel", { compact: false }).catch(console.error);
    invoke("stop_guide").catch(console.error);
    invoke("clear_pointer").catch(console.error);
    show("ask");
    return;
  }
  if (!$("confirm").hidden) {
    currentPlan = null;
    backToAsk();
    return;
  }
  invoke("dismiss").catch(console.error);
});


const VIEW_KEY = "vans.view";

function applyView(table: boolean, persist: boolean) {
  $("cards").hidden = table;
  $("table-wrap").hidden = !table;
  $("view-cards").classList.toggle("on", !table);
  $("view-table").classList.toggle("on", table);
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

function backToAsk() {
  const intent = $("intent") as HTMLInputElement;
  show("ask");
  intent.focus();
  intent.select();
}

for (const button of Array.from($("mode").querySelectorAll("button"))) {
  button.addEventListener("click", () => {
    const clicked: Mode = button.getAttribute("data-mode") === "do" ? "do" : "show";
    pin(pinned === clicked ? null : clicked);
    ($("intent") as HTMLInputElement).focus();
  });
}
$("intent").addEventListener("input", renderMode);
renderMode();

$("confirm-instead").addEventListener("click", () => {
  currentPlan = null;
  void extract("show!", lastInstruction);
});

$("guiding-instead").addEventListener("click", async () => {
  switchTo = "do!";
  await invoke("stop_guide").catch(console.error);
});

$("view-cards").addEventListener("click", () => applyView(false, true));
$("view-table").addEventListener("click", () => applyView(true, true));

$("ask-again").addEventListener("click", backToAsk);
$("clear-style").addEventListener("click", clearStyle);
$("guiding-next").addEventListener("click", () => {
  $("guiding-step").textContent = "Working out the next step…";
  invoke("advance_guide").catch(console.error);
});
$("guiding-stop").addEventListener("click", async () => {
  invoke("compact_panel", { compact: false }).catch(console.error);
  await invoke("stop_guide").catch(console.error);
  await invoke("clear_pointer").catch(console.error);
  show("ask");
});

$("to-task").addEventListener("click", backToAsk);
$("back").addEventListener("click", backToAsk);
$("back-from-unknown").addEventListener("click", backToAsk);

$("intent").addEventListener("keydown", (e) => {
  const key = (e as KeyboardEvent).key;

  if (key === "Enter") {
    extract((e as KeyboardEvent).metaKey ? "flip" : undefined);
    return;
  }
  if (key === "ArrowDown") {
    e.preventDefault();
    focusSuggestion(0);
    return;
  }
  if (key === "Tab") {
    e.preventDefault();
    cycleDestination((e as KeyboardEvent).shiftKey ? -1 : 1);
  }
});

function suggestionButtons(): HTMLElement[] {
  const list = $("ask").hidden ? $("can-do") : $("suggestions");
  return Array.from(list.querySelectorAll<HTMLElement>(".suggestion"));
}

function focusSuggestion(index: number) {
  const items = suggestionButtons();
  if (!items.length) return;
  items[Math.max(0, Math.min(index, items.length - 1))].focus();
}

function cycleDestination(by: number) {
  if (!destinations.length) return;
  const at = destinations.findIndex((d) => d.name === chosen);
  const next = (at + by + destinations.length) % destinations.length;
  pickDestination(destinations[next].name);
}

document.addEventListener("keydown", (e) => {
  const items = suggestionButtons();
  const at = items.indexOf(document.activeElement as HTMLElement);
  if (at < 0) return;

  if (e.key === "ArrowDown") {
    e.preventDefault();
    focusSuggestion(at + 1);
  } else if (e.key === "ArrowUp") {
    e.preventDefault();
    if (at === 0) ($("intent") as HTMLInputElement).focus();
    else focusSuggestion(at - 1);
  }
});

window.addEventListener("keydown", (e) => {
  if ($("confirm").hidden) return;
  if (e.key === "Enter") {
    e.preventDefault();
    confirmWrite();
  }
});

let dragging = false;
let dragFrom = { x: 0, y: 0 };
let dragTo: { dx: number; dy: number } | null = null;
let dragFrame = 0;

$("grip").addEventListener("mousedown", (e) => {
  const target = e.target as HTMLElement;
  if (target.closest("button, input, a, select, textarea")) return;
  e.preventDefault();

  dragging = true;
  dragFrom = { x: e.screenX, y: e.screenY };
  invoke("begin_drag").catch(console.error);
});

window.addEventListener("mousemove", (e) => {
  if (!dragging) return;
  dragTo = { dx: e.screenX - dragFrom.x, dy: e.screenY - dragFrom.y };
  if (!dragFrame) {
    dragFrame = requestAnimationFrame(() => {
      dragFrame = 0;
      if (dragTo) invoke("drag_by", dragTo).catch(console.error);
    });
  }
});

window.addEventListener("mouseup", () => {
  if (!dragging) return;
  dragging = false;
  dragTo = null;
  invoke("end_drag").catch(console.error);
});

$("opacity").addEventListener("input", (e) => {
  applyLook({ ...look, tint: Number((e.target as HTMLInputElement).value) }, true);
});

$("blur").addEventListener("change", (e) => {
  applyLook({ ...look, blur: (e.target as HTMLInputElement).checked }, true);
});

$("dark").addEventListener("change", (e) => {
  applyTheme((e.target as HTMLInputElement).checked, true);
});

$("show-history").addEventListener("click", showHistory);
$("history-back").addEventListener("click", () => show("preflight"));

$("recheck").addEventListener("click", () => runPreflight(false));
$("check-screen").addEventListener("click", checkScreenRecording);
$("check-notes").addEventListener("click", checkNotes);
$("commit").addEventListener("click", confirmWrite);
$("cancel").addEventListener("click", () => {
  currentPlan = null;
  backToAsk();
});
$("setup").addEventListener("click", () => {
  show("preflight");
  runPreflight(false);
});

applyTheme(storedTheme() === "dark", false);
applyLook(storedLook(), false);
applyView(tableView(), false);
void loadSuggestions();

show("preflight");
runPreflight();
refreshActiveContext();
loadDestinations();
