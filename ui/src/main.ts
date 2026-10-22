import { invoke } from "@tauri-apps/api/core";
import { emit, listen } from "@tauri-apps/api/event";
import { DotField, type DotMode } from "./dots";
import { livingPill } from "./pill";
import { THEMES, chooseTheme, followTheme, type Theme } from "./theme";

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

interface Permission {
  name: string;
  label: string;
  why: string;
  state: "granted" | "not_asked" | "needed" | "unknown";
  settings: string;
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
  tokens_in?: number;
  tokens_out?: number;
  cost?: number;
  destination: string;
  needs_commit: boolean;
  kind: string;
  mode?: string;
  mode_why?: string;
  say?: string;
  say_title?: string;
  panel?: "dark" | "light";
  marked?: boolean;
  copy?: string;
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
  tokens_in?: number;
  tokens_out?: number;
  cost?: number;
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
  remembered?: boolean;
  instant?: boolean;
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
  renderAskChips();
  showExample();
}

const MOST_CHIPS = 6;
const MOST_BESIDE_REMEMBERED = 4;

function renderAskChips() {
  const all = currentSuggestions();
  const remembered = all.filter((s) => s.remembered);
  const examples = all.filter((s) => !s.remembered);
  $("remembered-row").hidden = remembered.length === 0;
  renderChips($("remembered"), remembered);
  renderChips($("suggestions"), examples.slice(0, remembered.length ? MOST_BESIDE_REMEMBERED : MOST_CHIPS));
}

function renderChips(into: HTMLElement, items: Suggestion[]) {
  clear(into);
  items.forEach((s, i) => into.appendChild(chipFor(s, i)));
}

function chipFor(s: Suggestion, i: number): HTMLElement {
  const writes = !!s.destination && destinationWrites(s.destination);
  const chip = el("button", "chip") as HTMLButtonElement;
  chip.classList.toggle("write", writes);
  chip.classList.toggle("instant", !!s.instant);
  chip.type = "button";
  chip.style.animationDelay = `${i * 0.05}s`;
  chip.title = s.instant ? `${s.text} · ran without asking the model last time` : s.text;
  chip.appendChild(el("span", "dot"));
  chip.appendChild(el("span", undefined, s.destination ? `${s.text} → ${label(s.destination)}` : s.text));
  chip.addEventListener("click", () => {
    ($("intent") as HTMLInputElement).value = s.text;
    if (s.destination) pickDestination(s.destination);
    show("ask");
    extract();
  });
  if (!s.remembered) return chip;

  const forget = el("button", "chip-x", "×") as HTMLButtonElement;
  forget.type = "button";
  forget.title = "Forget this";
  forget.setAttribute("aria-label", `Forget “${s.text}”`);
  forget.addEventListener("click", async () => {
    try {
      await invoke("forget_instruction", { text: s.text });
      toast("Forgotten");
    } catch (e) {
      console.error("could not forget that", e);
    }
    await loadSuggestions(activeContext?.app_name);
    fitWindow();
  });

  const wrap = el("span", "chip-wrap");
  wrap.appendChild(chip);
  wrap.appendChild(forget);
  return wrap;
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
  renderFoot();
  clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => {
    node.hidden = true;
    renderFoot();
    fitWindow();
  }, 2600);
}

const FINISHED = new Set(["results", "failure", "notunderstood", "history"]);
const WITH_FIELD = new Set(["ask", "working", "results", "notunderstood", "failure"]);
const WITHOUT_FOOT = new Set(["guiding", "question"]);
let showing = "ask";
let pillStep = "";

function tellPill() {
  const state =
    showing === "working"
      ? "working"
      : showing === "guiding"
        ? "guiding"
        : showing === "question"
          ? "asking"
          : "idle";
  const stepping = state === "guiding" || actingIn !== null;
  if (state === "idle" && actingIn === null) pillStep = "";
  emit("pill://state", {
    state,
    text: stepping && pillStep ? pillStep : state === "asking" ? "Question" : undefined,
  }).catch(() => {});
}
let openFresh = false;

const HINTS: Record<string, [string, string][]> = {
  ask: [
    ["↵", "Run"],
    ["⇥", "Example"],
  ],
  working: [["esc", "Hide"]],
  results: [
    ["↵", "Ask again"],
    ["esc", "Back"],
  ],
  confirm: [
    ["hold ↵", "Do it"],
    ["esc", "Cancel"],
  ],
  notunderstood: [
    ["↑↓", "Pick"],
    ["esc", "Back"],
  ],
  failure: [["esc", "Back"]],
  preflight: [["esc", "Done"]],
  history: [["esc", "Back"]],
};

function renderFoot() {
  const toasting = !$("toast").hidden;
  $("dests").hidden = showing !== "ask" || toasting;

  const hints = $("hints");
  clear(hints);
  for (const [key, what] of HINTS[showing] ?? []) {
    const hint = el("span", "hint");
    hint.appendChild(el("kbd", undefined, key));
    hint.appendChild(document.createTextNode(what));
    hints.appendChild(hint);
  }
}

let workingSince = 0;
let workingTick = 0;

function setStatus(text: string, pill = "") {
  $("status-text").textContent = text;
  $("status-pill").textContent = pill;
  $("status-pill").hidden = pill === "";
  $("status").hidden = text === "" && pill === "";
  renderRun();
}

function renderRun() {
  const ready = showing === "ask" && ($("intent") as HTMLInputElement).value.trim() !== "";
  $("run").hidden = !ready;
  $("active-app-label").hidden = ready || !$("status").hidden || showing !== "ask";
}

function seconds(since: number): string {
  return `${((performance.now() - since) / 1000).toFixed(1)}s`;
}

function show(id: string) {
  const was = showing;
  showing = id;
  openFresh = false;
  for (const s of document.querySelectorAll("section")) {
    (s as HTMLElement).hidden = s.id !== id;
  }
  invoke("set_pinned", { pinned: PINNED.has(id) }).catch(console.error);
  adjusting = id === "preflight";
  if ((id === "preflight") !== (was === "preflight")) watchPermissions(id === "preflight");
  setReading(id !== "ask");
  paintTint();

  const field = WITH_FIELD.has(id);
  $("grip").hidden = !field;
  $("field-line").hidden = !field;
  $("foot").hidden = WITHOUT_FOOT.has(id);
  panel.classList.toggle("confirming", id === "confirm");
  panel.classList.toggle("guiding", id === "guiding");
  ($("intent") as HTMLInputElement).readOnly = id === "working";

  clearInterval(workingTick);
  if (id === "working") {
    workingSince = performance.now();
    setStatus("0.0s");
    workingTick = window.setInterval(() => setStatus(seconds(workingSince)), 100);
  } else if (was === "working" && WITH_FIELD.has(id) && id !== "ask") {
    setStatus(seconds(workingSince));
  } else {
    setStatus("");
  }

  scroller.scrollTop = 0;
  showExample();
  if (id === "ask") renderMode();
  if (id === "ask" && was !== "ask") void loadSuggestions(activeContext?.app_name);
  renderFoot();
  paintPresence();
  if (id !== "confirm") releaseHold();
  if (id === "confirm") invoke("take_keyboard").catch(console.error);
  tellPill();
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
  loadWatching().catch(console.error);
  loadInvite().catch(console.error);

  let result: Preflight;
  let perms: Permission[];
  try {
    [result, perms] = await Promise.all([
      invoke<Preflight>("preflight"),
      loadPermissions(),
    ]);
  } catch (e) {
    clear(checks);
    checks.appendChild(
      el("p", "bad", isRpcError(e) ? e.message : String(e)),
    );
    return;
  }

  clear(checks);
  const shown = result.checks.filter((c) => c.name !== "accessibility" && !(c.name === "llm" && lastInvite.tester));
  for (const c of shown) {
    checks.appendChild(checkRow(c));
  }
  setupReady = result.ready && !mustAsk(perms) && perms.every((p) => p.state === "granted");
  renderSetupGroups();

  if (result.ready && !mustAsk(perms) && advance) {
    if (welcomed()) show("ask");
    else void showWelcome();
  }
}

interface LoginItem {
  on: boolean;
  works: boolean;
  approve: boolean;
}

const LOGIN_KEY = "vans.login.decided";

function showLogin(item: LoginItem) {
  const box = $("login-item") as HTMLInputElement;
  box.checked = item.on;
  box.disabled = !item.works;
  const note = $("login-note");
  note.hidden = item.works && !item.approve;
  note.textContent = !item.works
    ? "Works in the installed app."
    : "macOS wants you to allow it in System Settings › General › Login Items.";
}

async function loadLogin() {
  let item = await invoke<LoginItem>("login_item");
  let decided = true;
  try {
    decided = localStorage.getItem(LOGIN_KEY) !== null;
  } catch {}
  if (item.works && !decided) {
    try {
      item = await invoke<LoginItem>("set_login_item", { on: true });
    } catch (e) {
      console.error(e);
    }
    try {
      localStorage.setItem(LOGIN_KEY, "1");
    } catch {}
  }
  showLogin(item);
}

interface Watching {
  on: boolean;
  clicks: number;
  shared?: boolean;
}

interface NotedClick {
  at: string;
  app: string;
  role: string;
  title: string;
}

const MAC_MOMENTS: Record<string, string> = {
  wake: "Mac woke",
  sleep: "Mac went to sleep",
  lock: "Mac locked",
  unlock: "Mac unlocked",
};

function notedLine(c: NotedClick): string {
  if (c.role === "front") return `Switched to ${c.app}`;
  if (c.role === "moment") return MAC_MOMENTS[c.title] ?? c.title;
  return `${c.app} · ${c.title}`;
}

function showWatching(w: Watching) {
  ($("watching") as HTMLInputElement).checked = w.on;
  const sent = w.shared
    ? " The habits it finds, and how you answered them, are sent to Vedansh to improve VANS; the list of clicks itself is not."
    : "";
  $("watching-note").textContent = w.on
    ? `VANS notes the names of the buttons and menu items you click, which app is in front, and when your Mac wakes, unlocks and locks, so it can offer what you usually do at the moment you usually do it. Never what you type, window titles, or anything on a web page. The notes stay on this Mac for seven days.${sent}`
    : "Off. Nothing new is noted. What was noted goes after seven days, or now with Clear.";
  const see = $("watching-see") as HTMLButtonElement;
  see.textContent = w.clicks ? `See what VANS noted (${w.clicks})` : "Nothing noted yet";
  see.disabled = w.clicks === 0;
  ($("watching-clear") as HTMLButtonElement).disabled = w.clicks === 0;
  if (w.clicks === 0) $("watching-list").hidden = true;
}

async function loadWatching() {
  showWatching(await invoke<Watching>("watching"));
}

async function seeNoted() {
  const list = $("watching-list");
  if (!list.hidden) {
    list.hidden = true;
    return;
  }
  const clicks = await invoke<NotedClick[]>("noted_clicks");
  list.replaceChildren(
    ...clicks
      .slice(-200)
      .reverse()
      .map((c) => {
        const li = document.createElement("li");
        const when = document.createElement("time");
        when.textContent = new Date(c.at).toLocaleString([], {
          weekday: "short",
          hour: "2-digit",
          minute: "2-digit",
        });
        li.append(when, notedLine(c));
        return li;
      }),
  );
  list.hidden = false;
}

const PERMS_KEY = "vans.permissions";

interface PermsMemory {
  asked: boolean;
  seen: Record<string, Permission["state"]>;
}

function permsMemory(): PermsMemory {
  try {
    const raw = localStorage.getItem(PERMS_KEY);
    if (raw) return { asked: false, seen: {}, ...JSON.parse(raw) };
  } catch {}
  return { asked: false, seen: {} };
}

function rememberPerms(perms: Permission[], asked: boolean) {
  const memory = permsMemory();
  memory.asked = memory.asked || asked;
  for (const p of perms) {
    if (p.state !== "unknown") memory.seen[p.name] = p.state;
  }
  try {
    localStorage.setItem(PERMS_KEY, JSON.stringify(memory));
  } catch {}
}

function withMemory(perms: Permission[]): Permission[] {
  const { seen } = permsMemory();
  return perms.map((p) =>
    p.state === "unknown" && seen[p.name] ? { ...p, state: seen[p.name] } : p,
  );
}

function mustAsk(perms: Permission[]): boolean {
  const { asked } = permsMemory();
  return perms.some(
    (p) =>
      p.state === "not_asked" ||
      (p.name === "accessibility" && p.state !== "granted") ||
      (!asked && p.state !== "granted"),
  );
}

let permsAsking = false;

async function loadPermissions(): Promise<Permission[]> {
  const perms = withMemory(await invoke<Permission[]>("permissions"));
  rememberPerms(perms, false);
  renderPermissions(perms);
  if (waitingFor && perms.find((p) => p.name === waitingFor)?.state === "granted") {
    waitingFor = null;
    invoke("summon").catch(console.error);
  }
  return perms;
}

let permsOpen = false;

function renderPermissions(perms: Permission[]) {
  lastPerms = perms;
  const list = $("perms");
  clear(list);
  const { asked } = permsMemory();
  for (const p of perms) {
    const ok = p.state === "granted";
    const box = el("div", ok ? "check ok" : "check");
    box.appendChild(el("span", "check-dot"));
    const text = el("div", "check-text");
    text.appendChild(el("div", "check-label", p.label));
    text.appendChild(el("div", "check-detail", p.why));
    box.appendChild(text);
    const control = p.name !== "accessibility" && p.name !== "screen";
    const denied = p.state === "needed" && control;
    if (!ok && (denied || (asked && p.state !== "not_asked"))) {
      const fix = el("button", "btn quiet", control ? "Ask again" : "Turn on") as HTMLButtonElement;
      fix.addEventListener("click", () => {
        if (control) void askAgain(p);
        else openSettingsFor(p);
      });
      box.appendChild(fix);
    } else {
      box.appendChild(el("span", "check-tag", ok ? "Granted" : "Needed"));
    }
    list.appendChild(box);
  }
  const screen = perms.find((p) => p.name === "screen");
  $("perms-reopen").hidden = !asked || !screen || screen.state === "granted";
  ($("perms-ask") as HTMLButtonElement).hidden = perms.every((p) => p.state === "granted");
  const allGranted = perms.every((p) => p.state === "granted");
  const folded = allGranted && !permsOpen;
  list.hidden = folded;
  $("perms-note").hidden = allGranted;
  $("perms-fold").hidden = !allGranted;
  $("perms-fold").textContent = permsOpen ? "Hide" : "Show";
  if (!permsAsking) {
    $("perms-state").textContent = allGranted
      ? `All ${perms.length} permissions granted.`
      : asked
        ? "Accessibility and Screen Recording can only be switched on by you, in Settings. Turn on takes you there and VANS comes back when it is on."
        : "";
  }
  fitWindow();
}

document.addEventListener("pointermove", (e) => {
  const option = (e.target as HTMLElement | null)?.closest?.(".seg > *") as HTMLElement | null;
  if (!option) return;
  const box = option.getBoundingClientRect();
  option.style.setProperty("--mx", `${e.clientX - box.left}px`);
  option.style.setProperty("--my", `${e.clientY - box.top}px`);
});

let lastPerms: Permission[] = [];

$("perms-fold").addEventListener("click", () => {
  permsOpen = !permsOpen;
  renderPermissions(lastPerms);
});

let waitingFor: string | null = null;

function openSettingsFor(p: Permission) {
  waitingFor = p.name;
  invoke("open_permission_settings", { name: p.name }).catch(console.error);
  invoke("dismiss").catch(console.error);
}

async function askAgain(p: Permission) {
  permsAsking = true;
  $("perms-state").textContent = "Answer the macOS prompts as they come up.";
  try {
    const perms = withMemory(await invoke<Permission[]>("ask_permissions_again"));
    rememberPerms(perms, true);
    permsAsking = false;
    renderPermissions(perms);
  } catch {
    permsAsking = false;
    openSettingsFor(p);
  }
}

async function askPermissions() {
  const button = $("perms-ask") as HTMLButtonElement;
  button.disabled = true;
  permsAsking = true;
  $("perms-state").textContent = "Answer each macOS prompt as it comes up.";
  try {
    const perms = withMemory(await invoke<Permission[]>("ask_permissions"));
    rememberPerms(perms, true);
    permsAsking = false;
    renderPermissions(perms);
  } catch (e) {
    permsAsking = false;
    $("perms-state").textContent = String(e);
  } finally {
    button.disabled = false;
  }
}

let permsTick = 0;

function watchPermissions(on: boolean) {
  clearInterval(permsTick);
  if (!on) return;
  permsTick = window.setInterval(() => {
    if (!permsAsking) loadPermissions().catch(console.error);
  }, 2000);
}

function checkRow(c: Check): HTMLElement {
  const box = el("div", c.ok ? "check ok" : "check");
  box.appendChild(el("span", "check-dot"));

  const text = el("div", "check-text");
  text.appendChild(el("div", "check-label", c.label));
  text.appendChild(el("div", "check-detail", c.detail));
  if (!c.ok && c.fix) text.appendChild(el("pre", "fix", c.fix));
  box.appendChild(text);

  box.appendChild(el("span", "check-tag", checkTag(c)));
  return box;
}

function checkTag(c: Check): string {
  if (!c.ok) return "Needed";
  if (c.name === "browser") return "Connected";
  if (c.name === "llm" && lastInvite.tester) return "Saved";
  return "Ready";
}

// --- Setup, in groups ---

let setupReady = false;
let startOpen: boolean | null = null;
const setupGroups: { id: string; title: string; blocks: string[] }[] = [
  { id: "start", title: "Getting started", blocks: ["perms", "ready", "invite", "model"] },
  { id: "behave", title: "How VANS behaves", blocks: ["rules", "learning"] },
  { id: "about", title: "About this copy", blocks: ["tell", "record", "updates", "start", "appearance"] },
];
const groupOpen: Record<string, boolean> = { behave: true, about: true };

function block(name: string): HTMLElement | null {
  return document.querySelector<HTMLElement>(`[data-block="${name}"]`);
}

function arrangeSetup() {
  const body = $("setup-body");
  const helper = $("setup-helper");
  for (const g of setupGroups) {
    const group = el("div", "setup-group");
    group.dataset.group = g.id;
    const head = el("button", "group-head") as HTMLButtonElement;
    head.type = "button";
    head.appendChild(el("span", "group-title", g.title));
    head.appendChild(el("span", "group-state"));
    head.appendChild(el("span", "group-chev"));
    head.addEventListener("click", () => {
      const open = !groupIsOpen(g.id);
      if (g.id === "start") startOpen = open;
      else groupOpen[g.id] = open;
      renderSetupGroups();
    });
    const inside = el("div", "group-body");
    for (const name of g.blocks) {
      const b = block(name);
      if (b) inside.appendChild(b);
    }
    group.appendChild(head);
    group.appendChild(inside);
    body.insertBefore(group, helper);
  }
  arrangeRules();
  renderSetupGroups();
}

function groupIsOpen(id: string): boolean {
  if (id === "start") return startOpen ?? !setupReady;
  return groupOpen[id] ?? true;
}

function renderSetupGroups() {
  $("setup-lede").hidden = setupReady;
  for (const g of setupGroups) {
    const group = document.querySelector<HTMLElement>(`.setup-group[data-group="${g.id}"]`);
    if (!group) continue;
    const open = groupIsOpen(g.id);
    group.classList.toggle("open", open);
    (group.querySelector(".group-head") as HTMLElement).setAttribute("aria-expanded", String(open));
    (group.querySelector(".group-body") as HTMLElement).hidden = !open;
    const state = group.querySelector(".group-state") as HTMLElement;
    if (g.id === "start") state.textContent = setupReady ? "VANS is ready" : "Something still needs you";
    if (g.id === "about") state.textContent = $("version-line").textContent ?? "";
  }
  fitWindow();
}

function ruleValue(rule: HTMLElement): string {
  const value = (rule.querySelector("input") as HTMLInputElement).value.trim();
  return value || rule.dataset.empty || "";
}

function arrangeRules() {
  for (const rule of Array.from(document.querySelectorAll<HTMLElement>(".rule[data-rule]"))) {
    let view = rule.querySelector<HTMLElement>(".rule-view");
    let save = rule.querySelector<HTMLElement>(".rule-save");
    if (!view) {
      view = el("div", "setting-row rule-view");
      view.appendChild(el("span", "setting-label", rule.dataset.name));
      view.appendChild(el("span", "rule-value"));
      const edit = el("button", "btn link", "Edit") as HTMLButtonElement;
      edit.type = "button";
      edit.addEventListener("click", () => editRule(rule, true));
      view.appendChild(edit);
      rule.prepend(view);
      save = el("div", "row rule-save");
      const button = el("button", "btn", "Save") as HTMLButtonElement;
      button.type = "button";
      const said = el("span", "note inline");
      button.addEventListener("click", async () => {
        said.textContent = "";
        if (await saveRules()) editRule(rule, false);
        else said.textContent = "Could not save";
      });
      save.appendChild(button);
      save.appendChild(said);
      rule.appendChild(save);
    }
    editRule(rule, false);
  }
}

function editRule(rule: HTMLElement, editing: boolean) {
  const view = rule.querySelector<HTMLElement>(".rule-view")!;
  view.hidden = editing;
  (view.querySelector(".rule-value") as HTMLElement).textContent = ruleValue(rule);
  (rule.querySelector(".note") as HTMLElement).hidden = !editing;
  (rule.querySelector("input") as HTMLElement).hidden = !editing;
  (rule.querySelector(".rule-save") as HTMLElement).hidden = !editing;
  if (editing) (rule.querySelector("input") as HTMLInputElement).focus();
  fitWindow();
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
  renderRun();
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

  try {
    if (mustAsk(withMemory(await invoke<Permission[]>("permissions")))) {
      show("preflight");
      void runPreflight(false);
      return;
    }
  } catch (e) {
    console.error(e);
  }

  stoppedAt = null;
  renderWhyStopped();
  ($("intent") as HTMLInputElement).value = instruction;
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
    if (plan.panel && !plan.needs_commit) applyTheme(plan.panel === "dark", true);
    invoke("note", {
      from: "timing",
      what: `${plan.elapsed_ms} ms, ${plan.synthesis_calls + plan.decision_calls} calls, ${plan.app || hostOf(plan.page_url) || "?"}: ${instruction}`,
    }).catch(() => {});
    if (plan.say) {
      if (plan.marked) showClearStyle(true);
      renderSay(plan);
      show("results");
      if (plan.decision_calls + plan.synthesis_calls === 0) setStatus("", "Instant");
    } else if (plan.kind === "app.guide") {
      await showTheWay(plan);
    } else if (plan.needs_commit) {
      renderConfirm(plan);
      show("confirm");
    } else {
      renderResults(plan);
      show("results");
      if (plan.decision_calls + plan.synthesis_calls === 0) setStatus("instant", "Remembered");
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
      if (!rerunAs) {
        stoppedAt = { asked: lastInstruction, app: coachedApp, step: $("guiding-step").textContent ?? "" };
        renderWhyStopped();
      }
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
    avoid_x?: number;
    avoid_y?: number;
    avoid_w?: number;
    avoid_h?: number;
  };
}>("engine://drive_step", (event) => {
  if (actingIn === null) return;
  const s = event.payload.params;
  pillStep = `Step ${s.number}`;
  tellPill();
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
    avoid: [s.avoid_x ?? 0, s.avoid_y ?? 0, s.avoid_w ?? 0, s.avoid_h ?? 0],
  }).catch(console.error);
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

listen("coach://yes-to-all", () => {
  if ($("guiding").hidden) return;
  invoke("answer_guide", { yes: true, all: true }).catch(console.error);
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
    avoid_x?: number;
    avoid_y?: number;
    avoid_w?: number;
    avoid_h?: number;
    offer?: boolean;
    offer_all?: boolean;
    doing?: boolean;
  };
}>(
  "engine://guide",
  (event) => {
    const s = event.payload.params;
    $("guiding-step").textContent = `${s.number}. ${s.label}`;
    pillStep = `Step ${s.number}`;
    tellPill();

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
      mode: s.doing ? "drive" : "coach",
      avoid: [s.avoid_x ?? 0, s.avoid_y ?? 0, s.avoid_w ?? 0, s.avoid_h ?? 0],
      offer: s.offer ?? false,
      offerAll: s.offer_all ?? false,
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

const SHORT_FACT = 32;

/// A card per record: a picture if there is one, a headline that goes
/// somewhere if there is a link, the rest as prose with the short facts on one
/// line, and the site it came from.
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

    const facts: string[] = [];
    for (const f of bodyFields) {
      const v = (row[f] ?? "").trim();
      if (!v) continue;
      if (v.length <= SHORT_FACT) facts.push(v);
      else body.appendChild(el("p", "card-text", v));
    }
    if (facts.length) body.appendChild(el("p", "card-text", facts.join(" · ")));

    if (host) body.appendChild(el("div", "card-host", host));

    card.appendChild(body);
    into.appendChild(card);
  }
}

function renderSay(plan: Plan) {
  $("answer").hidden = true;
  clear($("answer-cites"));
  $("results-kicker").textContent = plan.say_title ?? "";
  $("results-kicker").title = "";
  clear($("cards"));
  const text = el("p", "card-text", plan.say ?? "");
  text.style.whiteSpace = "pre-wrap";
  $("cards").appendChild(text);
  clear($("rows"));
  $("cards").hidden = false;
  $("table-wrap").hidden = true;
  $("view-switch").hidden = true;
  $("copy-result").hidden = !plan.copy;
  $("copy-result").textContent = "Copy";
}

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const area = document.createElement("textarea");
    area.value = text;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    const copied = document.execCommand("copy");
    area.remove();
    return copied;
  }
}

function renderResults(plan: Plan) {
  $("copy-result").hidden = true;
  $("view-switch").hidden = !!plan.action || plan.rows.length === 0;
  if (!$("view-switch").hidden) applyView(tableView(), false);
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
  const remembered = plan.decision_calls + plan.synthesis_calls === 0 ? " · remembered" : "";
  const answered = !!plan.answer;
  $("results-kicker").textContent = answered
    ? `${plan.app || source}${remembered}`
    : `${n} result${n === 1 ? "" : "s"} from ${source}${remembered}`;
  $("results-kicker").title = timing(plan);
  $("show-read").textContent = `Show what VANS read (${n})`;
  $("show-read").hidden = !answered || n === 0;
  if (answered) {
    $("view-switch").hidden = true;
    $("cards").hidden = true;
    $("table-wrap").hidden = true;
  }

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

    const label = hostOf(firstLink(row, plan.fields));
    if (!label) continue;
    const chip = el("button", "cite", label) as HTMLButtonElement;
    chip.type = "button";
    chip.title = row[plan.fields[0]] ?? "";
    chip.addEventListener("click", () => {
      showRead();
      const card = $("cards").children[n - 1] as HTMLElement | undefined;
      card?.scrollIntoView({ behavior: "smooth", block: "center" });
      card?.classList.add("lit");
      setTimeout(() => card?.classList.remove("lit"), 1200);
    });
    cites.appendChild(chip);
  }
}

function showRead() {
  if ($("show-read").hidden) return;
  $("show-read").hidden = true;
  $("view-switch").hidden = false;
  applyView(tableView(), false);
  fitWindow();
}

function firstLink(row: Record<string, string>, fields: string[]): string {
  for (const f of fields) {
    const v = (row[f] ?? "").trim();
    if (/^https?:/i.test(v)) return v;
  }
  return "";
}

function holdLabel(plan: Plan | null): string {
  return plan?.action ? "Hold ↵ to do it" : "Hold ↵ to write";
}

function renderConfirm(plan: Plan) {
  showHeadsup("confirm", plan);
  $("confirm-tag").textContent = plan.action ? "Changes something" : "Writes something";
  $("confirm-target").textContent = plan.action ? (plan.app ?? "") : label(plan.destination);
  if (plan.action) {
    $("confirm-title").textContent = plan.app ? `Do this in ${plan.app}?` : "Do this?";
    $("confirm-preview").textContent = plan.action;
  } else {
    const n = plan.rows.length;
    $("confirm-title").textContent =
      `Write ${n} result${n === 1 ? "" : "s"} to ${label(plan.destination)}?`;
    $("confirm-preview").textContent = documentText(plan);
  }
  ($("commit") as HTMLButtonElement).textContent = holdLabel(plan);
}

const HOLD_FOR = 750;
let holdTimer = 0;

function releaseHold() {
  if (!holdTimer) return;
  clearTimeout(holdTimer);
  holdTimer = 0;
  const button = $("commit") as HTMLButtonElement;
  button.classList.remove("holding");
  if (!button.disabled) button.textContent = holdLabel(currentPlan);
}

function startHold() {
  const button = $("commit") as HTMLButtonElement;
  if (holdTimer || button.disabled || $("confirm").hidden || !currentPlan) return;
  button.style.setProperty("--hold-for", `${HOLD_FOR}ms`);
  button.textContent = "Keep holding…";
  button.classList.add("holding");
  holdTimer = window.setTimeout(() => {
    holdTimer = 0;
    button.classList.remove("holding");
    void confirmWrite();
  }, HOLD_FOR);
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
  const cost = (calls === 0 ? "no model calls" : `${calls} model call${calls === 1 ? "" : "s"}`) + spent(plan);
  return `${cost}, ${plan.elapsed_ms} ms — page signature ${plan.signature}`;
}

async function carryOn(planId: string, app: string) {
  actingIn = app;
  await invoke("dismiss").catch(console.error);
  await new Promise((resolve) => setTimeout(resolve, 150));
  try {
    const receipt = await invoke<Receipt>("commit", { planId });
    actingIn = null;
    invoke("finish_pointer").catch(console.error);
    await invoke("summon").catch(console.error);
    const how = howItWent(receipt.work);
    const done = receipt.verified ? `Done in ${app}` : `Done in ${app}, unverified`;
    toast(how ? `${done} \u00B7 ${how}` : done);
    ($("intent") as HTMLInputElement).value = "";
    show("ask");
  } catch (e) {
    actingIn = null;
    invoke("clear_pointer").catch(console.error);
    await invoke("summon").catch(console.error);
    renderFailure(e);
    show("failure");
  }
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
    if (currentPlan.panel) applyTheme(currentPlan.panel === "dark", true);
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
    button.textContent = holdLabel(null);
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

  const asked = (work.calls === 1 ? "1 model call" : `${work.calls} model calls`) + spent(work);

  if (work.replayed === 0) return `${plural(work.steps, "step")}, ${asked}`;
  if (work.replayed === work.steps && work.calls === 0) {
    return `all ${plural(work.steps, "step")} from last time, nothing asked`;
  }
  return `${work.replayed} of ${plural(work.steps, "step")} from last time, ${asked}`;
}

function tokens(n: number): string {
  return n >= 1000 ? `${(n / 1000).toFixed(1)}k tokens` : `${n} tokens`;
}

function money(cost: number): string {
  if (cost >= 0.01) return `$${cost.toFixed(2)}`;
  if (cost >= 0.0001) return `$${cost.toFixed(4)}`;
  return "under $0.0001";
}

function spent(work: { tokens_in?: number; tokens_out?: number; cost?: number }): string {
  const used = (work.tokens_in ?? 0) + (work.tokens_out ?? 0);
  if (used === 0) return "";
  return work.cost ? `, ${tokens(used)}, ${money(work.cost)}` : `, ${tokens(used)}`;
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
  broke_off: {
    title: "Stopped partway",
    message: "",
  },
  not_allowed: {
    title: "Your rules say no",
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
  resetReport();

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
    renderSuggestions($("can-do"), currentSuggestions().slice(0, 4));
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
    case "not_allowed": {
      const data = e.data as { where?: string; rule?: string };
      $("failure-title").textContent = data?.where ? `Not in ${data.where}` : "Your rules say no";
      const rule = data?.rule ?? withoutThePrefix(e.message);
      $("failure-message").textContent = rule.charAt(0).toUpperCase() + rule.slice(1) + ".";
      return;
    }
    case "broke_off": {
      const data = e.data as { app?: string; plan_id?: string; did?: string[]; why?: string };
      const did = data?.did ?? [];
      $("failure-title").textContent = data?.app ? `Stopped partway in ${data.app}` : "Stopped partway";
      $("failure-message").textContent =
        `${did.length} step${did.length === 1 ? " was" : "s were"} done, then: ${withoutThePrefix(data?.why ?? e.message)}`;
      const list = el("ol", "done-steps", "");
      for (const line of did) {
        list.appendChild(el("li", "", line.split(" \u2014 ")[0]));
      }
      detail.appendChild(list);
      if (data?.plan_id) {
        const again = el("button", "btn secondary", "Carry on from here") as HTMLButtonElement;
        const planId = data.plan_id;
        const app = data.app ?? "";
        again.addEventListener("click", () => void carryOn(planId, app));
        detail.appendChild(again);
      }
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

  if (!ctx || !ctx.app_name || ctx.app_name === "Unknown") {
    label.textContent = "";
    label.title = "";
    return;
  }
  label.textContent = `in ${ctx.app_name}`;
  label.title = ctx.window_title ? `${ctx.app_name} — ${ctx.window_title}` : ctx.app_name;
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
    const button = el("button", d.touches_world ? "writes" : undefined, label(d.name)) as HTMLButtonElement;
    button.type = "button";
    button.setAttribute("role", "radio");
    button.dataset.name = d.name;
    button.title = `⌘${destinations.indexOf(d) + 1} · ${d.describe}`;
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
  $("destination-note").hidden = !d.touches_world;
  paintPresence();
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

const DEFAULT_LOOK: Look = { tint: 86, blur: true };

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

/// Dark unless the user has said otherwise.
///
/// The stylesheet's bare :root is the light palette and dark is an attribute on
/// top of it, so the choice is made here on every start: anything but a stored
/// "light" comes up dark.
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
  paintPresence();

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
const scroller = document.querySelector(".scroll") as HTMLElement;

const presence = new DotField($("presence") as HTMLCanvasElement, { cols: 3, rows: 3, gap: 6, dot: 0.3, baseAlpha: 0.22 });
const guidePresence = new DotField($("guide-presence") as HTMLCanvasElement, {
  cols: 7,
  rows: 3,
  gap: 7,
  dot: 0.26,
  baseAlpha: 0.16,
});
let guideMode: DotMode = "think";

function token(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

function paintPresence() {
  const writing = showing === "confirm" || destinationWrites(chosen);
  presence.colours(token(writing ? "--write" : "--volt"), token("--ink"));
  presence.mode = showing === "working" ? "think" : "idle";
  guidePresence.colours(token("--volt"), token("--dark-paper"));
  guidePresence.mode = guideMode;
}

presence.start();
guidePresence.start();

function showAccents(current: Theme) {
  const box = $("accents");
  box.replaceChildren(
    ...THEMES.map((t) => {
      const b = document.createElement("button");
      b.type = "button";
      b.className = "accent-swatch";
      b.title = t.name;
      b.setAttribute("role", "radio");
      b.setAttribute("aria-label", t.name);
      b.setAttribute("aria-checked", String(t.id === current.id));
      b.style.setProperty("--swatch", t.accent);
      b.addEventListener("click", async () => showAccents(await chooseTheme(t.id)));
      return b;
    }),
  );
  paintPresence();
}

const startingTheme = followTheme(showAccents);
showAccents(startingTheme);
invoke("set_accent", { accent: startingTheme.id }).catch(() => {});

/// Tells the shell how tall the panel wants to be.
///
/// A summoned panel is not a document viewer: it should be the size of what it
/// has to say. At a fixed height, preflight — the one screen whose entire job
/// is to be read — came up scrolled into its own middle with the heading cut
/// off the top, and nothing on screen suggested there was anything above it.
///
/// The field, the content and the footer are measured, never .panel or the
/// scrolling middle. Those stretch to fill the window, so measuring them would
/// report the window's own height back to the window, and the panel could grow
/// but never shrink again.
let requested = 0;

function tall(id: string): number {
  const node = $(id);
  return node.hidden ? 0 : node.getBoundingClientRect().height;
}

function fitWindow() {
  const style = getComputedStyle(panel);
  const chrome =
    parseFloat(style.paddingTop) +
    parseFloat(style.paddingBottom) +
    parseFloat(style.borderTopWidth) +
    parseFloat(style.borderBottomWidth);

  const height = Math.ceil(tall("grip") + tall("field-line") + tall("content") + tall("foot") + chrome);

  // Sub-pixel churn from font loading and scrollbar reflow would otherwise
  // resize the window continuously.
  if (Math.abs(height - requested) < 2) return;
  requested = height;

  invoke("resize_panel", { height }).catch(console.error);
}

const sized = new ResizeObserver(fitWindow);
for (const id of ["content", "grip", "foot"]) sized.observe($(id));

// --- wiring ---

listen<{ for_request: number; message: string }>("engine://progress", (event) => {
  const text = humanise(event.payload.message);
  $("progress").textContent = text;
  if (!$("guiding").hidden) {
    $("guiding-step").textContent = text;
    guideMode = "think";
    paintPresence();
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

let kept = false;

function keepPanel() {
  if (kept) return;
  kept = true;
  invoke("keep_panel").catch(() => {});
}

document.addEventListener("mousedown", keepPanel, true);
document.addEventListener("keydown", keepPanel, true);

listen<ScreenContext | undefined>("overlay://shown", (event) => {
  kept = false;
  if (event.payload) {
    updateActiveContext(event.payload);
  } else {
    refreshActiveContext();
  }

  void loadSuggestions(event.payload?.app_name);
  loadHabit().catch(console.error);

  scroller.scrollTop = 0;

  if (openFresh) {
    const intent = $("intent") as HTMLInputElement;
    if (showing === "results" || showing === "history") {
      intent.value = "";
    }
    currentPlan = null;
    show("ask");
    renderMode();
    fitWindow();
  }

  const ask = $("ask");
  if (!ask.hidden) {
    const intent = $("intent") as HTMLInputElement;
    intent.focus();
    intent.select();
  }
});

window.addEventListener("keydown", (e) => {
  if (e.metaKey && /^[1-9]$/.test(e.key) && showing === "ask") {
    const d = destinations[Number(e.key) - 1];
    if (d) {
      e.preventDefault();
      pickDestination(d.name);
    }
    return;
  }
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
  if (showing === "history") {
    show("preflight");
    return;
  }
  if (FINISHED.has(showing) || showing === "preflight") {
    backToAsk();
    return;
  }
  const intent = $("intent") as HTMLInputElement;
  if (showing === "ask" && intent.value !== "") {
    intent.value = "";
    renderMode();
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

interface Ability {
  id: string;
  title: string;
  says: string;
  examples?: string[];
}

let abilitiesKnown: Ability[] = [];

async function abilities(): Promise<Ability[]> {
  try {
    abilitiesKnown = (await invoke<Ability[]>("abilities")) ?? [];
  } catch {}
  return abilitiesKnown;
}

interface WelcomeKey {
  before?: string;
  key: string;
  after?: string;
}

interface Welcome {
  title: string;
  lede: string;
  keys?: WelcomeKey[];
}

async function fillWelcome() {
  let w: Welcome;
  try {
    w = await invoke<Welcome>("welcome");
  } catch {
    return;
  }
  $("welcome-title").textContent = w.title;
  $("welcome-lede").textContent = w.lede;
  const keys = $("welcome-keys");
  clear(keys);
  for (const k of w.keys ?? []) {
    const line = el("li");
    if (k.before) line.appendChild(document.createTextNode(k.before + " "));
    line.appendChild(el("kbd", undefined, k.key));
    if (k.after) line.appendChild(document.createTextNode(" " + k.after));
    keys.appendChild(line);
  }
}

function tryExample(text: string) {
  ($("intent") as HTMLInputElement).value = text;
  backToAsk();
  renderMode();
}

function exampleChip(text: string): HTMLElement {
  const chip = el("button", "chip") as HTMLButtonElement;
  chip.type = "button";
  chip.appendChild(el("span", "dot"));
  chip.appendChild(el("span", undefined, text));
  chip.addEventListener("click", () => {
    welcomeDone();
    tryExample(text);
  });
  return chip;
}

function abilityRow(a: Ability, examples: string[]): HTMLElement {
  const row = el("div", "ability");
  row.appendChild(el("div", "ability-title", a.title));
  row.appendChild(el("p", "ability-says", a.says));
  if (examples.length) {
    const box = el("div", "ability-examples");
    for (const text of examples) box.appendChild(exampleChip(text));
    row.appendChild(box);
  }
  return row;
}

let abilitiesFrom = "ask";

async function showAbilities() {
  abilitiesFrom = showing === "abilities" ? abilitiesFrom : showing;
  show("abilities");
  const list = $("ability-list");
  clear(list);
  for (const a of await abilities()) list.appendChild(abilityRow(a, a.examples ?? []));
  fitWindow();
}

const WELCOME_KEY = "vans.welcomed";

function welcomed(): boolean {
  try {
    return localStorage.getItem(WELCOME_KEY) === "yes";
  } catch {
    return true;
  }
}

function welcomeDone() {
  try {
    localStorage.setItem(WELCOME_KEY, "yes");
  } catch {}
}

async function showWelcome() {
  const [, all] = await Promise.all([fillWelcome(), abilities()]);
  show("welcome");
  const tries = $("welcome-tries");
  clear(tries);
  const withExamples = all.filter((a) => a.examples?.length).slice(0, 3);
  for (const a of withExamples) tries.appendChild(abilityRow(a, [a.examples![0]]));
  fitWindow();
}

$("welcome-start").addEventListener("click", () => {
  welcomeDone();
  backToAsk();
});
$("welcome-more").addEventListener("click", () => {
  welcomeDone();
  void showAbilities();
});
$("abilities-back").addEventListener("click", () => {
  if (abilitiesFrom === "ask" || !abilitiesFrom) backToAsk();
  else show(abilitiesFrom);
  fitWindow();
});
$("show-abilities").addEventListener("click", () => void showAbilities());
$("setup-abilities").addEventListener("click", () => void showAbilities());
$("setup-welcome").addEventListener("click", () => void showWelcome());

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

livingPill($("run"));
$("run").addEventListener("click", () => void extract());

$("ask-again").addEventListener("click", backToAsk);
$("copy-result").addEventListener("click", async () => {
  const text = currentPlan?.copy;
  if (!text) return;
  $("copy-result").textContent = (await copyText(text)) ? "Copied" : "Could not copy";
});

interface DueTask {
  what: string;
  when: string;
}

let dueTasks: DueTask[] = [];

interface HabitOffer {
  key: string;
  name: string;
  times: number;
  routine: string;
  instruction: string;
}

let habitOffer: HabitOffer | null = null;

async function loadHabit() {
  const offers = await invoke<HabitOffer[]>("habits");
  habitOffer = offers[0] ?? null;
  renderHabit();
  fitWindow();
}

async function settleHabit(how: "save" | "show" | "dismiss", name = ""): Promise<string> {
  const offer = habitOffer;
  if (!offer) return "";
  const done = await invoke<{ instruction: string }>("settle_habit", { key: offer.key, how, name });
  habitOffer = null;
  renderHabit();
  void loadHabit();
  return done.instruction;
}

function renderHabit() {
  const box = $("habit");
  clear(box);
  box.hidden = habitOffer === null;
  const offer = habitOffer;
  if (!offer) return;

  const row = el("div", "row");
  row.appendChild(el("span", "field-note", `You often do this. ${offer.name}.`));

  const save = el("button", "btn secondary", "Save as a routine") as HTMLButtonElement;
  save.addEventListener("click", () => {
    clear(row);
    row.appendChild(el("span", "field-note", "Name it. Say the name to run it."));
    const name = document.createElement("input");
    name.className = "field";
    name.value = offer.routine;
    const keep = el("button", "btn secondary", "Save") as HTMLButtonElement;
    const back = el("button", "btn quiet", "Cancel") as HTMLButtonElement;
    keep.addEventListener("click", async () => {
      const called = name.value.trim() || offer.routine;
      try {
        await settleHabit("save", called);
        toast(`Saved. Say “${called}” to run it.`);
      } catch (e) {
        toast(isRpcError(e) ? e.message : String(e));
      }
    });
    back.addEventListener("click", renderHabit);
    row.append(name, keep, back);
    name.focus();
    name.select();
    fitWindow();
  });

  const teach = el("button", "btn secondary", "Show me") as HTMLButtonElement;
  teach.addEventListener("click", async () => {
    const instruction = await settleHabit("show");
    if (instruction) void extract("show", instruction);
  });

  const later = el("button", "btn quiet", "Not now") as HTMLButtonElement;
  later.addEventListener("click", () => {
    settleHabit("dismiss").catch(console.error);
  });

  row.append(save, teach, later);
  box.appendChild(row);
}

listen<{ instruction?: string }>("nudge://run", async (event) => {
  const instruction = event.payload?.instruction?.trim();
  if (!instruction) return;
  openFresh = false;
  await invoke("summon").catch(console.error);
  void extract("do", instruction);
});

listen("engine://habits", () => {
  loadHabit().catch(console.error);
});

interface UpdateState {
  current: string;
  updates: boolean;
  ready?: string | null;
  looked: boolean;
  error?: string | null;
}

const SEEN_VERSION_KEY = "vans.version";

function renderUpdate(u: UpdateState) {
  const box = $("update-ready");
  clear(box);
  box.hidden = !u.ready;
  if (u.ready) {
    const row = el("div", "row");
    row.appendChild(el("span", "field-note", `VANS ${u.ready} is ready. It restarts by itself when you are away.`));
    const now = el("button", "btn secondary", "Restart now") as HTMLButtonElement;
    now.addEventListener("click", () => invoke("restart_app").catch(console.error));
    row.appendChild(now);
    box.appendChild(row);
  }
  $("version-line").textContent = u.ready ? `VANS ${u.current}, ${u.ready} ready` : `VANS ${u.current}`;
  $("restart-now").hidden = !u.ready;
  ($("check-updates") as HTMLButtonElement).hidden = !!u.ready;
  renderSetupGroups();
  const note = $("update-note");
  note.hidden = true;
  if (!u.updates) {
    note.textContent = "This copy was built from the code, so it does not update itself.";
    note.hidden = false;
  } else if (u.error) {
    note.textContent = `Could not look for updates: ${u.error}.`;
    note.hidden = false;
  } else if (u.looked && !u.ready) {
    note.textContent = "Up to date.";
    note.hidden = false;
  }
  fitWindow();
}

async function loadUpdate() {
  const u = await invoke<UpdateState>("update_state");
  renderUpdate(u);
  try {
    const seen = localStorage.getItem(SEEN_VERSION_KEY);
    if (seen && seen !== u.current) toast(`Updated to VANS ${u.current}`);
    localStorage.setItem(SEEN_VERSION_KEY, u.current);
  } catch {}
}

$("check-updates").addEventListener("click", async () => {
  const button = $("check-updates") as HTMLButtonElement;
  button.disabled = true;
  button.textContent = "Looking…";
  try {
    renderUpdate(await invoke<UpdateState>("check_updates"));
  } finally {
    button.disabled = false;
    button.textContent = "Check for updates";
  }
});

$("restart-now").addEventListener("click", () => invoke("restart_app").catch(console.error));

listen<string>("vans://updated", () => {
  invoke<UpdateState>("update_state").then(renderUpdate).catch(console.error);
});

loadUpdate().catch(console.error);

function renderDue() {
  const box = $("due");
  clear(box);
  box.hidden = dueTasks.length === 0;
  for (const task of dueTasks) {
    const row = el("div", "row");
    row.appendChild(el("span", "field-note", `It is time: ${task.what}`));
    const run = el("button", "btn secondary", "Run") as HTMLButtonElement;
    run.addEventListener("click", () => {
      dueTasks = dueTasks.filter((t) => t !== task);
      renderDue();
      void extract(undefined, task.what);
    });
    const skip = el("button", "btn quiet", "Skip") as HTMLButtonElement;
    skip.addEventListener("click", () => {
      dueTasks = dueTasks.filter((t) => t !== task);
      renderDue();
      fitWindow();
    });
    row.appendChild(run);
    row.appendChild(skip);
    box.appendChild(row);
  }
}

listen<{ params: DueTask }>("engine://schedule_due", (event) => {
  const task = event.payload?.params;
  if (!task?.what) return;
  dueTasks.push(task);
  renderDue();
  if (showing === "ask" || FINISHED.has(showing)) {
    openFresh = false;
    show("ask");
    fitWindow();
    invoke("summon").catch(console.error);
  } else {
    toast(`Scheduled: ${task.what}. It is waiting in the ask box.`);
  }
});

const STAYS_THROUGH_A_DESKTOP_CHANGE = new Set(["working", "confirm", "question"]);

listen("overlay://desktop-changed", () => {
  if (STAYS_THROUGH_A_DESKTOP_CHANGE.has(showing)) return;
  invoke("dismiss").catch(console.error);
});

listen("overlay://hidden", () => {
  const offersAWayOn = showing === "failure" && $("failure-detail").querySelector("button") !== null;
  openFresh = FINISHED.has(showing) && !offersAWayOn;
});
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
    if (showing === "working") return;
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
    const intent = $("intent") as HTMLInputElement;
    if (intent.value === "" && showing === "ask" && examples().length) {
      intent.value = examples()[exampleAt % examples().length];
      renderMode();
      return;
    }
    cycleDestination((e as KeyboardEvent).shiftKey ? -1 : 1);
  }
});

let exampleAt = 0;

function examples(): string[] {
  return currentSuggestions()
    .filter((s) => !s.destination)
    .map((s) => s.text);
}

function showExample() {
  const all = examples();
  ($("intent") as HTMLInputElement).placeholder = all.length ? all[exampleAt % all.length] : "Ask VANS…";
}

function nextExample() {
  const all = examples();
  if (!all.length || ($("intent") as HTMLInputElement).value !== "" || showing !== "ask") return;
  exampleAt = (exampleAt + 1) % all.length;
  showExample();
}

window.setInterval(nextExample, 2800);

function suggestionButtons(): HTMLElement[] {
  const list = $("ask").hidden ? $("can-do") : $("ask");
  return Array.from(list.querySelectorAll<HTMLElement>(".suggestion, .chip"));
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
  if ($("confirm").hidden || e.key !== "Enter") return;
  e.preventDefault();
  if (!e.repeat) startHold();
});

window.addEventListener("keyup", (e) => {
  if (e.key === "Enter") releaseHold();
});

window.addEventListener("blur", releaseHold);

let dragging = false;
let dragFrom = { x: 0, y: 0 };
let dragTo: { dx: number; dy: number } | null = null;
let dragFrame = 0;

function beginDrag(e: MouseEvent) {
  const target = e.target as HTMLElement;
  if (target.closest("button, input, a, select, textarea")) return;
  e.preventDefault();

  dragging = true;
  dragFrom = { x: e.screenX, y: e.screenY };
  invoke("begin_drag").catch(console.error);
}

for (const handle of Array.from(document.querySelectorAll<HTMLElement>(".grip"))) {
  handle.addEventListener("mousedown", beginDrag);
}

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

interface Rules {
  never: string[];
  spend_limit: number;
  ask_first?: string[];
}

function namesIn(id: string): string[] {
  return ($(id) as HTMLInputElement).value
    .split(",")
    .map((s) => s.trim())
    .filter((s) => s !== "");
}

async function loadRules() {
  try {
    const r = await invoke<Rules>("rules");
    ($("rules-never") as HTMLInputElement).value = (r?.never ?? []).join(", ");
    ($("rules-ask") as HTMLInputElement).value = (r?.ask_first ?? []).join(", ");
    ($("rules-limit") as HTMLInputElement).value = r?.spend_limit ? String(r.spend_limit) : "";
  } catch (e) {
    console.error(e);
  }
  arrangeRules();
}

async function saveRules(): Promise<boolean> {
  const never = namesIn("rules-never");
  const askFirst = namesIn("rules-ask");
  const limit = Number(($("rules-limit") as HTMLInputElement).value.replace(/[^0-9.]/g, "")) || 0;
  try {
    await invoke<Rules>("set_rules", { never, spendLimit: limit, askFirst });
    return true;
  } catch (e) {
    console.error(e);
    return false;
  }
}

void loadRules();

let canSendFeedback = false;

async function checkFeedback() {
  try {
    canSendFeedback = Boolean((await invoke<{ ready?: boolean }>("can_send_feedback"))?.ready);
  } catch {
    canSendFeedback = false;
  }
  $("tell").hidden = !canSendFeedback;
}

void checkFeedback();

function resetReport() {
  $("report").hidden = !canSendFeedback;
  $("report-open").hidden = false;
  $("report-form").hidden = true;
  ($("report-note") as HTMLTextAreaElement).value = "";
  $("report-state").textContent = "";
}

function failedGuideReport(): string {
  const lines = [`Asked: ${lastInstruction.trim() || "(nothing typed)"}`, `VANS said: ${$("failure-title").textContent ?? ""}`];
  const message = $("failure-message").textContent?.trim();
  if (message) lines.push(message);
  const detail = ($("failure-detail") as HTMLElement).innerText.trim();
  if (detail) lines.push("", detail.slice(0, 4000));
  return lines.join("\n");
}

const STOP_REASONS = ["Wrong step", "Too slow", "Just looking"];

let stoppedAt: { asked: string; app: string; step: string } | null = null;

function renderWhyStopped() {
  const box = $("why-stopped");
  clear(box);
  const at = stoppedAt;
  box.hidden = at === null || !canSendFeedback;
  if (!at || !canSendFeedback) return;

  const row = el("div", "row");
  row.appendChild(el("span", "field-note", "Why did you stop? One tap tells Vedansh."));
  for (const reason of STOP_REASONS) {
    const pick = el("button", "btn secondary", reason) as HTMLButtonElement;
    pick.addEventListener("click", async () => {
      stoppedAt = null;
      renderWhyStopped();
      fitWindow();
      const text = [reason, `Asked: ${at.asked.trim() || "(nothing typed)"}`, `In: ${at.app || "(unknown)"}`, `At: ${at.step.trim() || "(before the first step)"}`].join("\n");
      try {
        await invoke("send_feedback", { kind: "stopped_guide", text });
        toast("Thanks. Sent to Vedansh");
      } catch (e) {
        toast(isRpcError(e) ? `Not sent: ${withoutThePrefix(e.message)}` : "Not sent. Try again in a moment.");
      }
    });
    row.appendChild(pick);
  }
  const skip = el("button", "btn quiet", "Skip") as HTMLButtonElement;
  skip.addEventListener("click", () => {
    stoppedAt = null;
    renderWhyStopped();
    fitWindow();
  });
  row.appendChild(skip);
  box.appendChild(row);
}

async function sendFeedback(kind: "failed_guide" | "note", text: string, state: HTMLElement): Promise<boolean> {
  state.textContent = "Sending\u2026";
  try {
    await invoke("send_feedback", { kind, text });
    state.textContent = "Sent. Thank you.";
    return true;
  } catch (e) {
    state.textContent = isRpcError(e) ? `Not sent: ${withoutThePrefix(e.message)}` : "Not sent. Try again in a moment.";
    console.error(e);
    return false;
  }
}

$("report-open").addEventListener("click", () => {
  $("report-text").textContent = failedGuideReport();
  $("report-open").hidden = true;
  $("report-form").hidden = false;
  ($("report-note") as HTMLTextAreaElement).focus();
});

$("report-cancel").addEventListener("click", resetReport);

$("report-send").addEventListener("click", async () => {
  const note = ($("report-note") as HTMLTextAreaElement).value.trim();
  const text = $("report-text").textContent + (note ? `\n\nThey added: ${note}` : "");
  const button = $("report-send") as HTMLButtonElement;
  button.disabled = true;
  if (await sendFeedback("failed_guide", text, $("report-state"))) {
    $("report-form").hidden = true;
    toast("Sent to Vedansh");
  }
  button.disabled = false;
});

$("tell-send").addEventListener("click", async () => {
  const box = $("tell-text") as HTMLTextAreaElement;
  const text = box.value.trim();
  if (!text) return;
  const button = $("tell-send") as HTMLButtonElement;
  button.disabled = true;
  if (await sendFeedback("note", text, $("tell-state"))) box.value = "";
  button.disabled = false;
});
void loadModel();

$("show-history").addEventListener("click", showHistory);
interface ModelStatus {
  configured: boolean;
  source: string;
  base_url: string;
  model: string;
  fast_model: string;
  fallback_model: string;
  has_key: boolean;
  key_hint: string;
  price_in: number;
  price_out: number;
}

function field(id: string): HTMLInputElement {
  return $(id) as HTMLInputElement;
}

function showModel(m: ModelStatus) {
  field("model-key").value = "";
  field("model-key").placeholder = m.has_key ? `Key saved (${m.key_hint || "hidden"}). Paste a new one to replace it` : "Paste your model key";
  field("model-name").value = m.model ?? "";
  field("model-base").value = m.base_url ?? "";
  field("model-fast").value = m.fast_model ?? "";
  field("model-fallback").value = m.fallback_model ?? "";
  field("model-price-in").value = m.price_in ? String(m.price_in) : "";
  field("model-price-out").value = m.price_out ? String(m.price_out) : "";
  $("model-forget").hidden = !(m.has_key && m.source === "saved");
  $("model-note").textContent =
    m.source === "environment"
      ? "The model is set by the environment this copy was started with. Saving here applies until it is restarted."
      : "VANS needs a key for the model it thinks with. It is kept in your Mac\u2019s Keychain and sent only to that model.";
}

interface InviteState {
  tester: boolean;
  has: boolean;
  email?: string;
  waiting?: boolean;
}

type InviteStep = "email" | "code" | "paste";

const WAITING = "You\u2019re on the waiting list. VANS starts working as soon as your place opens; there\u2019s nothing to do.";

let lastInvite: InviteState = { tester: false, has: false };
let changingInvite = false;
let inviteStep: InviteStep = "email";
let joiningAs = "";

function showInvite(i: InviteState) {
  lastInvite = i;
  $("invite-section").hidden = !i.tester;
  $("model-section").hidden = i.tester;
  field("invite-code").value = "";
  field("invite-code").placeholder = i.has ? "Saved. Paste a new code to replace it" : "vans-\u2026";
  const folded = i.has && !changingInvite;
  $("invite-saved").hidden = !folded;
  $("invite-who").textContent = i.email ? `Signed in as ${i.email}.` : "Signed in with an invite code.";
  $("join-email").hidden = folded || inviteStep !== "email";
  $("join-code").hidden = folded || inviteStep !== "code";
  $("invite-entry").hidden = folded || inviteStep !== "paste";
  $("invite-paste").hidden = folded;
  $("invite-paste").textContent = inviteStep === "paste" ? "Sign in with your email instead" : "Have an invite code instead?";
}

function inviteGoTo(step: InviteStep) {
  inviteStep = step;
  showInvite(lastInvite);
  field(step === "email" ? "join-address" : step === "code" ? "join-digits" : "invite-code").focus();
  fitWindow();
}

function rpcMessage(e: unknown): string {
  return isRpcError(e) ? withoutThePrefix(e.message) : String(e);
}

$("invite-change").addEventListener("click", () => {
  changingInvite = true;
  inviteGoTo("email");
});

$("invite-paste").addEventListener("click", () => {
  inviteGoTo(inviteStep === "paste" ? "email" : "paste");
});

$("join-back").addEventListener("click", () => {
  $("join-code-state").textContent = "";
  inviteGoTo("email");
});

async function loadInvite() {
  try {
    showInvite(await invoke<InviteState>("invite"));
  } catch {}
}

async function sendJoinCode() {
  const button = $("join-send") as HTMLButtonElement;
  const email = field("join-address").value.trim();
  if (!email) {
    $("join-state").textContent = "Type your email first.";
    return;
  }
  button.disabled = true;
  $("join-state").textContent = "Sending\u2026";
  try {
    await invoke<InviteState>("ask_to_join", { email });
    joiningAs = email;
    $("join-state").textContent = "";
    $("join-sent").textContent = `We sent a 6-digit code to ${email}. It works for 15 minutes. Check spam if it isn\u2019t there in a minute.`;
    field("join-digits").value = "";
    inviteGoTo("code");
  } catch (e) {
    $("join-state").textContent = rpcMessage(e);
  } finally {
    button.disabled = false;
    fitWindow();
  }
}

$("join-send").addEventListener("click", sendJoinCode);
field("join-address").addEventListener("keydown", (e) => {
  if ((e as KeyboardEvent).key === "Enter") sendJoinCode();
});

async function checkJoinCode() {
  const button = $("join-check") as HTMLButtonElement;
  const code = field("join-digits").value.replace(/\s+/g, "");
  if (!code) {
    $("join-code-state").textContent = "Type the code from the email first.";
    return;
  }
  button.disabled = true;
  $("join-code-state").textContent = "Checking\u2026";
  try {
    const state = await invoke<InviteState>("join", { email: joiningAs, code });
    changingInvite = false;
    inviteStep = "email";
    $("join-code-state").textContent = "";
    showInvite(state);
    if (state.waiting) $("invite-who").textContent = `Signed in as ${state.email}. ${WAITING}`;
    runPreflight(false);
  } catch (e) {
    $("join-code-state").textContent = rpcMessage(e);
  } finally {
    button.disabled = false;
    fitWindow();
  }
}

$("join-check").addEventListener("click", checkJoinCode);
field("join-digits").addEventListener("keydown", (e) => {
  if ((e as KeyboardEvent).key === "Enter") checkJoinCode();
});

$("invite-save").addEventListener("click", async () => {
  const button = $("invite-save") as HTMLButtonElement;
  const code = field("invite-code").value.trim();
  if (!code) {
    $("invite-state").textContent = "Paste the code first.";
    return;
  }
  button.disabled = true;
  $("invite-state").textContent = "Checking\u2026";
  try {
    changingInvite = false;
    inviteStep = "email";
    showInvite(await invoke<InviteState>("set_invite", { code }));
    $("invite-state").textContent = "Saved. The VANS server knows this code.";
    runPreflight(false);
  } catch (e) {
    $("invite-state").textContent = rpcMessage(e);
  } finally {
    button.disabled = false;
    fitWindow();
  }
});

async function loadModel() {
  try {
    showModel(await invoke<ModelStatus>("model_settings"));
  } catch {
    // A build that takes its model from the environment has nothing to show.
  }
}

function price(id: string): number {
  return Number(field(id).value.replace(/[^0-9.]/g, "")) || 0;
}

$("model-save").addEventListener("click", async () => {
  const button = $("model-save") as HTMLButtonElement;
  button.disabled = true;
  $("model-saved").textContent = "Testing\u2026";
  try {
    const status = await invoke<ModelStatus>("set_model_settings", {
      apiKey: field("model-key").value.trim(),
      baseUrl: field("model-base").value.trim(),
      model: field("model-name").value.trim(),
      fastModel: field("model-fast").value.trim(),
      fallbackModel: field("model-fallback").value.trim(),
      priceIn: price("model-price-in"),
      priceOut: price("model-price-out"),
    });
    showModel(status);
    $("model-saved").textContent = "Saved. The model answered.";
    runPreflight(false);
  } catch (e) {
    $("model-saved").textContent = isRpcError(e) ? withoutThePrefix(e.message) : String(e);
  } finally {
    button.disabled = false;
    fitWindow();
  }
});

$("model-forget").addEventListener("click", async () => {
  try {
    showModel(await invoke<ModelStatus>("forget_model_key"));
    $("model-saved").textContent = "Key removed";
    runPreflight(false);
  } catch (e) {
    $("model-saved").textContent = isRpcError(e) ? withoutThePrefix(e.message) : String(e);
  }
});

$("model-more").addEventListener("toggle", () => fitWindow());

$("history-back").addEventListener("click", () => show("preflight"));

$("recheck").addEventListener("click", () => runPreflight(false));
$("quit").addEventListener("click", () => {
  invoke("quit_app").catch(console.error);
});

$("login-item").addEventListener("change", async (e) => {
  const box = e.target as HTMLInputElement;
  try {
    showLogin(await invoke<LoginItem>("set_login_item", { on: box.checked }));
  } catch (err) {
    box.checked = !box.checked;
    $("login-note").hidden = false;
    $("login-note").textContent = String(err);
  }
});
$("watching").addEventListener("change", async (e) => {
  const box = e.target as HTMLInputElement;
  try {
    showWatching(await invoke<Watching>("set_watching", { on: box.checked }));
  } catch (err) {
    box.checked = !box.checked;
    $("watching-note").textContent = String(err);
  }
});
$("watching-see").addEventListener("click", () => {
  seeNoted().catch(console.error);
});
$("watching-clear").addEventListener("click", async () => {
  showWatching(await invoke<Watching>("clear_clicks"));
});
$("show-read").addEventListener("click", showRead);
$("perms-ask").addEventListener("click", askPermissions);
$("perms-reopen").addEventListener("click", () => {
  invoke("reopen").catch(console.error);
});
$("commit").addEventListener("mousedown", startHold);
$("commit").addEventListener("mouseup", releaseHold);
$("commit").addEventListener("mouseleave", releaseHold);
$("theme-toggle").addEventListener("click", () => applyTheme(!isDark(), true));
$("cancel").addEventListener("click", () => {
  currentPlan = null;
  backToAsk();
});
$("setup").addEventListener("click", () => {
  if (showing === "preflight") {
    backToAsk();
    return;
  }
  show("preflight");
  runPreflight(false);
});

applyTheme(storedTheme() !== "light", false);
applyLook(storedLook(), false);
applyView(tableView(), false);
void loadSuggestions();

show("preflight");
arrangeSetup();
runPreflight();
loadLogin().catch(console.error);
loadWatching().catch(console.error);
refreshActiveContext();
loadDestinations();
