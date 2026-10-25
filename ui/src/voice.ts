import { listen } from "@tauri-apps/api/event";
import { followTheme } from "./theme";

followTheme();

type State = "arming" | "listening" | "thinking" | "heard" | "speaking" | "note";

interface Says {
  state: State;
  words: string;
}

interface Shape {
  notch: number;
  bar: number;
  width: number;
}

const island = document.getElementById("island") as HTMLElement;
const words = document.getElementById("words") as HTMLElement;
const measure = document.getElementById("measure") as HTMLElement;
const wave = Array.from(document.querySelectorAll<HTMLElement>(".wave i"));
const ring = document.querySelector<SVGCircleElement>("#tail .mic circle");
const root = document.documentElement.style;

const PROFILE = [0.35, 0.6, 0.85, 1, 0.85, 0.6, 0.35];
const RING = 47.1;
const LONGEST = 110;
const WINGS = 70;
const FLOAT_LEAD = 42;
const PAD = 48;
const LINE = 18;
const HINTS: [number, string][] = [
  [2600, "Say what you want…"],
  [7000, "Let go when you're done"],
];

let shape: "notch" | "float" = "float";
let notch = 0;
let bar = 0;
let room = 460;
let hint = "";
let target = 0;
let shown = 0;
let moving = false;
let shownWords: string[] = [];
let hints: number[] = [];
let leaving = 0;

function setVar(name: string, value: string) {
  root.setProperty(name, value);
}

function clearHints() {
  hints.forEach((h) => clearTimeout(h));
  hints = [];
}

function textWidth(text: string): number {
  measure.textContent = text;
  return measure.getBoundingClientRect().width;
}

function size() {
  const open = island.classList.contains("open");
  const state = island.dataset.state as State;
  const most = room - (shape === "notch" ? 2 * 10 : 24);
  if (!open) {
    setVar("--w", shape === "notch" ? `${notch}px` : `${Math.min(most, 120)}px`);
    setVar("--h", shape === "notch" ? `${bar}px` : "36px");
    return;
  }
  if (state === "arming") {
    setVar("--w", shape === "notch" ? `${notch + 28}px` : "72px");
    setVar("--h", shape === "notch" ? `${bar + 7}px` : "36px");
    return;
  }
  const text = words.textContent ?? "";
  const least = shape === "notch" ? notch + 2 * WINGS : 150;
  const lead = shape === "notch" ? 0 : FLOAT_LEAD;
  const want = Math.ceil(textWidth(text) + PAD + lead);
  const width = Math.max(least, Math.min(most, want));
  setVar("--w", `${width}px`);
  const lines = Math.min(2, Math.max(1, Math.ceil(want / Math.max(1, width - 2))));
  const textHeight = text ? lines * LINE : LINE;
  setVar("--h", shape === "notch" ? `${bar + textHeight + 14}px` : `${Math.max(36, textHeight + 16)}px`);
}

function wordsOf(text: string): string[] {
  return text.trim().split(/\s+/).filter(Boolean);
}

function clipped(list: string[]): string[] {
  let total = 0;
  const kept: string[] = [];
  for (let i = list.length - 1; i >= 0; i--) {
    total += list[i].length + 1;
    if (total > LONGEST && kept.length) {
      kept.unshift("…");
      break;
    }
    kept.unshift(list[i]);
  }
  return kept;
}

function writeLive(text: string) {
  const full = wordsOf(text);
  let same = 0;
  while (same < full.length && same < shownWords.length && full[same] === shownWords[same]) same++;
  const fresh = full.length - same;
  const kept = clipped(full);
  words.classList.remove("hint");
  words.replaceChildren();
  kept.forEach((w, i) => {
    if (i > 0) words.append(" ");
    const span = document.createElement("span");
    span.textContent = w;
    if (i >= kept.length - fresh) span.className = "in";
    words.append(span);
  });
  shownWords = full;
  size();
}

function writePlain(text: string, asHint = false) {
  words.classList.toggle("hint", asHint);
  words.replaceChildren(document.createTextNode(text));
  shownWords = [];
  size();
}

function writeSpoken(text: string) {
  words.classList.remove("hint");
  words.replaceChildren();
  let at = 0;
  for (const part of text.split(/(\s+)/)) {
    if (!part) continue;
    if (/^\s+$/.test(part)) {
      words.append(part);
    } else {
      const span = document.createElement("span");
      span.textContent = part;
      span.dataset.from = String(at);
      span.dataset.to = String(at + part.length);
      words.append(span);
    }
    at += part.length;
  }
  words.scrollTop = 0;
  shownWords = [];
  size();
}

function spoken(from: number, to: number) {
  let now: HTMLElement | null = null;
  for (const span of Array.from(words.querySelectorAll<HTMLElement>("span[data-from]"))) {
    const start = Number(span.dataset.from);
    const end = Number(span.dataset.to);
    span.classList.toggle("said", end <= from);
    const current = start < to && end > from;
    span.classList.toggle("now", current);
    if (current) now = span;
  }
  if (now) {
    const top = Math.max(0, now.offsetTop - words.offsetTop - LINE);
    words.scrollTo({ top, behavior: "smooth" });
  }
}

function frame(now: number) {
  shown = Math.max(target, shown * 0.86);
  target *= 0.9;
  wave.forEach((bar, i) => {
    const drift = ((Math.sin(now / 150 + i * 0.75) + 1) / 2) * 0.09;
    const reach = Math.pow(shown, 0.8) * PROFILE[i];
    bar.style.transform = `scaleY(${Math.min(1, 0.16 + reach * 0.9 + drift)})`;
  });
  setVar("--lvl", shown.toFixed(3));
  if (island.dataset.state === "listening" && island.classList.contains("open")) {
    requestAnimationFrame(frame);
  } else {
    moving = false;
    setVar("--lvl", "0");
  }
}

function animate() {
  if (moving) return;
  moving = true;
  requestAnimationFrame(frame);
}

function quiet(fraction: number) {
  if (!ring) return;
  ring.style.strokeDashoffset = String(RING * (1 - fraction));
}

function open(state: State) {
  window.clearTimeout(leaving);
  island.classList.remove("leaving", "quick", "shake");
  island.dataset.state = state;
  island.classList.add("open");
}

listen<Shape>("voice://shape", (e) => {
  notch = e.payload.notch;
  bar = e.payload.bar;
  room = e.payload.width;
  shape = notch > 0 ? "notch" : "float";
  island.dataset.shape = shape;
  setVar("--notch", `${notch}px`);
  setVar("--bar", `${bar}px`);
  size();
});

listen<Says>("voice://says", (e) => {
  const { state, words: said } = e.payload;
  clearHints();
  open(state);
  quiet(0);
  if (state === "arming") {
    writePlain("");
    return;
  }
  if (state === "listening") {
    hint = said || "Listening…";
    target = 0;
    shown = 0;
    writePlain(hint, true);
    animate();
    if (!said) {
      hints = HINTS.map(([after, text]) =>
        window.setTimeout(() => {
          if (island.dataset.state === "listening" && words.classList.contains("hint")) writePlain(text, true);
        }, after),
      );
    }
    return;
  }
  hint = "";
  if (state === "speaking") {
    writeSpoken(said);
    return;
  }
  if (state === "note") {
    writePlain(said);
    void island.offsetWidth;
    island.classList.add("shake");
    return;
  }
  if (state === "thinking" || state === "heard") {
    if (said.trim()) writeLive(said);
    else writePlain("…");
    return;
  }
  writePlain(said);
});

listen<string>("voice://words", (e) => {
  const state = island.dataset.state;
  if (state !== "listening" && state !== "thinking") return;
  if (e.payload.trim()) {
    clearHints();
    writeLive(e.payload);
  } else {
    writePlain(hint, true);
  }
});

listen<number>("voice://level", (e) => {
  target = Math.max(target, e.payload);
});

listen<number>("voice://quiet", (e) => quiet(e.payload));

listen<{ from: number; to: number }>("voice://spoken", (e) => {
  if (island.dataset.state === "speaking") spoken(e.payload.from, e.payload.to);
});

listen<{ how?: string }>("voice://close", (e) => {
  const how = e.payload?.how ?? "done";
  clearHints();
  island.classList.remove("shake");
  island.classList.add(how === "cancel" || how === "tap" ? "quick" : "leaving");
  island.classList.remove("open");
  setVar("--lvl", "0");
  size();
  leaving = window.setTimeout(() => {
    island.classList.remove("leaving", "quick");
    shownWords = [];
  }, 400);
});
