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

// --- helpers ---

const $ = <T extends HTMLElement>(id: string) => document.getElementById(id) as T;

function show(id: string) {
  for (const s of document.querySelectorAll("section")) {
    (s as HTMLElement).hidden = s.id !== id;
  }
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

// --- wiring ---

listen<{ for_request: number; message: string }>("engine://progress", (event) => {
  const progress = $("progress");
  progress.hidden = false;
  progress.appendChild(document.createTextNode(event.payload.message + "\n"));
});

$("run").addEventListener("click", extract);
$("recheck").addEventListener("click", runPreflight);
$("check-notes").addEventListener("click", checkNotes);
$("confirm").addEventListener("click", confirmWrite);
$("cancel").addEventListener("click", () => {
  currentPlan = null;
  show("task");
});
$("back").addEventListener("click", () => show("task"));
$("setup").addEventListener("click", () => {
  show("preflight");
  runPreflight();
});
$("again").addEventListener("click", () => show("task"));

show("preflight");
runPreflight();
