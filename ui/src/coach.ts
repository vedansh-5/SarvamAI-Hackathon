import { emit, listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { followTheme } from "./theme";

followTheme();

/// Says what the overlay is doing, into the shell's log rather than a console
/// nobody is reading. A page that never loaded and an event that reached no
/// listener look the same from outside: nothing happens, quietly.
function note(what: string) {
  invoke("note", { from: "coach", what }).catch(() => {});
}

interface Pointer {
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  step: string;
  verb?: string;
  detail?: string;
  mode?: string;
  ox?: number;
  oy?: number;
  offer?: boolean;
  offer_all?: boolean;
  avoid?: number[];
}

interface Bounds {
  x: number;
  y: number;
  w: number;
  h: number;
  r: number;
}

const PAD = 6;
const GAP = 14;
const MORPH_DURATION_MS = 320;

let currentBounds: Bounds | null = null;
let currentVerb = "click";
let animationFrameId: number | null = null;

// Where the ghost cursor is resting, so the next step's glide starts from where
// the last one ended rather than from nowhere. A hand that teleports is not a
// hand.
let ghostAt: { x: number; y: number } | null = null;
let ghostFrame: number | null = null;

const reducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

let runMode = "coach";
let origin = { x: 0, y: 0 };

interface Mark {
  n: number;
  x: number;
  y: number;
}

const TRAIL_MOST = 8;
let trail: Mark[] = [];
let lastShown: Mark | null = null;

function stepNumber(p: Pointer): number {
  const m = /\d+/.exec(p.step ?? "");
  return m ? Number(m[0]) : 0;
}

function rememberStep(n: number, at: { x: number; y: number } | null) {
  if (lastShown && n > lastShown.n) {
    const done = lastShown;
    trail = [...trail.filter((m) => m.n !== done.n), done].slice(-TRAIL_MOST);
  } else if (lastShown && n < lastShown.n) {
    trail = [];
  }
  lastShown = at ? { n, x: at.x + origin.x, y: at.y + origin.y } : null;
}

const SVG_NS = "http://www.w3.org/2000/svg";

function drawTrail(current: { x: number; y: number } | null) {
  const dots = elementById<SVGGElement>("trail-dots");
  const line = elementById<SVGPathElement>("trail-line");
  dots.replaceChildren();

  const onThisScreen = trail
    .map((m) => ({ n: m.n, x: m.x - origin.x, y: m.y - origin.y }))
    .filter((m) => m.x >= 0 && m.y >= 0 && m.x <= window.innerWidth && m.y <= window.innerHeight);

  const points = current ? [...onThisScreen, current] : onThisScreen;
  line.setAttribute(
    "d",
    points.length > 1 ? points.map((p, i) => `${i ? "L" : "M"} ${p.x} ${p.y}`).join(" ") : "",
  );

  for (const m of onThisScreen) {
    const g = document.createElementNS(SVG_NS, "g");
    g.setAttribute("class", "trail-dot");
    g.setAttribute("transform", `translate(${m.x} ${m.y})`);
    const circle = document.createElementNS(SVG_NS, "circle");
    circle.setAttribute("r", "9");
    circle.setAttribute("stroke-width", "1.5");
    const label = document.createElementNS(SVG_NS, "text");
    label.textContent = `${m.n}`;
    g.append(circle, label);
    dots.append(g);
  }
}

function clearTrail() {
  trail = [];
  lastShown = null;
  elementById<SVGGElement>("trail-dots").replaceChildren();
  elementById<SVGPathElement>("trail-line").setAttribute("d", "");
}

const KEYS_SHOWN: Record<string, string> = { up: "↑", down: "↓", left: "←", right: "→" };

function captionFor(pointer: Pointer, verb: string): string {
  const detail = (pointer.detail ?? "").trim();
  if (runMode === "drive") return pointer.label;
  switch (verb) {
    case "type":
      return detail ? `Type “${detail}”` : "Type here";
    case "press":
      return detail ? `Press ${detail}` : "Press the keys";
    case "scroll":
      return `Scroll ${detail} ${KEYS_SHOWN[detail] ?? ""}`.trim();
    case "doubleclick":
      return "Double-click here";
    case "rightclick":
      return "Right-click here";
    case "hover":
      return "Point here, without clicking";
    case "drag":
      return detail ? `Drag this to ${detail}` : "Drag this";
    case "set":
      return detail ? `Set this to ${detail}` : "Set this";
    case "wait":
      return "Waiting for this";
    default:
      return "Click here";
  }
}

let captionWidth = 0;
let captionAbove = false;
let captionLift = 40;
let captionAt: { x: number; y: number } | null = null;

function setCaption(text: string, hint: string) {
  const caption = elementById<HTMLElement>("caption");
  caption.textContent = text;
  if (hint) {
    const small = document.createElement("small");
    small.textContent = hint;
    caption.append(small);
  }
  caption.classList.add("on");
  captionWidth = caption.offsetWidth;
  if (captionAt) moveCaption(captionAt.x, captionAt.y);
}

const CAPTION_UNDER_TAG = 38;

function moveCaption(x: number, y: number) {
  const caption = elementById<HTMLElement>("caption");
  captionAt = { x, y };
  if (keepClearOf) {
    // Beside the menu, level with the row. Anywhere on the menu covers a row.
    let beside = keepClearOf.x + keepClearOf.w + 10;
    if (beside + captionWidth > window.innerWidth - 8) beside = keepClearOf.x - 10 - captionWidth;
    const level = Math.max(8, Math.min(y - 13, window.innerHeight - 34));
    caption.style.transform = `translate(${Math.round(beside)}px, ${Math.round(level)}px)`;
    return;
  }
  let left = x + 14;
  if (left + captionWidth > window.innerWidth - 8) left = x - 18 - captionWidth;
  left = Math.max(8, Math.min(left, window.innerWidth - 8 - captionWidth));
  const top = captionAbove
    ? Math.max(8, y - captionLift)
    : Math.min(y + CAPTION_UNDER_TAG, window.innerHeight - 34);
  caption.style.transform = `translate(${Math.round(left)}px, ${Math.round(top)}px)`;
}

function hideCaption() {
  elementById<HTMLElement>("caption").classList.remove("on");
  captionAt = null;
}

function elementById<T extends HTMLElement | SVGElement>(id: string): T {
  const el = document.getElementById(id);
  if (!el) throw new Error(`missing #${id}`);
  return el as T;
}

function calculateTargetBounds(pointer: Pointer): Bounds {
  const x = pointer.x - PAD;
  const y = pointer.y - PAD;
  const w = pointer.w + PAD * 2;
  const h = pointer.h + PAD * 2;
  const r = Math.min(14, Math.max(6, Math.min(w, h) / 4));
  return { x, y, w, h, r };
}

/// Where on the control the step happens.
///
/// Inside the control rather than at its centre, so the label someone is
/// reading stays legible.
function landingPoint(b: Bounds, verb: string): { x: number; y: number } {
  if (verb === "scroll") return { x: b.x + b.w / 2, y: b.y + b.h / 2 };
  const x = verb === "type" ? b.x + 14 : b.x + Math.min(b.w * 0.5, 40);
  return { x, y: b.y + b.h / 2 };
}

const SCROLL_ARROWS: Record<string, string> = { up: "↑", down: "↓", left: "←", right: "→" };

function easeInOutCubic(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}

/// The ghost cursor glides to where the step happens, then presses.
///
/// Along a curve rather than a straight line: a linear tween reads as a glitch,
/// and a slight arc is how a hand on a trackpad actually travels. The glide takes
/// longer for a longer journey, within limits, so a neighbouring control is a
/// flick and the far side of the screen is a sweep.
///
/// The first step of a run has nowhere to come from, so the ghost arrives from
/// below and to the left of its target, the way a hand enters from the edge.
function flyGhostTo(to: { x: number; y: number }, press: boolean) {
  const ghost = elementById<SVGGElement>("ghost");
  const from = ghostAt ?? { x: to.x - 150, y: to.y + 120 };

  if (ghostFrame !== null) cancelAnimationFrame(ghostFrame);
  ghost.classList.remove("landed", "pressing");
  ghost.setAttribute("opacity", "1");

  const place = (x: number, y: number) => {
    ghost.setAttribute("transform", `translate(${x} ${y})`);
    moveCaption(x, y);
  };

  const land = () => {
    ghostAt = to;
    ghostFrame = null;
    place(to.x, to.y);
    if (!press) return;
    ghost.classList.remove("landed");
    // Read layout between removing the class and adding it back, or a press on the
    // same spot as the last one would not replay its ripple.
    void ghost.getBoundingClientRect();
    ghost.classList.add("pressing");
    ghost.classList.add("landed");
    window.setTimeout(() => ghost.classList.remove("pressing"), 140);
  };

  place(from.x, from.y);

  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const distance = Math.hypot(dx, dy);

  if (reducedMotion || distance < 2) {
    land();
    return;
  }

  // The control point sits off to one side of the straight line, a fifth of the
  // way out, which bends the path without making it loop.
  const bend = Math.min(90, distance * 0.2);
  const cx = (from.x + to.x) / 2 - (dy / distance) * bend;
  const cy = (from.y + to.y) / 2 + (dx / distance) * bend;
  const duration = Math.min(560, Math.max(240, distance * 0.6));
  const start = performance.now();

  const frame = (now: number) => {
    const t = Math.min(1, (now - start) / duration);
    const e = easeInOutCubic(t);
    const u = 1 - e;

    place(
      u * u * from.x + 2 * u * e * cx + e * e * to.x,
      u * u * from.y + 2 * u * e * cy + e * e * to.y,
    );
    if (t < 1) {
      ghostFrame = requestAnimationFrame(frame);
    } else {
      land();
    }
  };
  ghostFrame = requestAnimationFrame(frame);
}

function hideGhost() {
  if (ghostFrame !== null) cancelAnimationFrame(ghostFrame);
  ghostFrame = null;
  ghostAt = null;
  const ghost = elementById<SVGGElement>("ghost");
  ghost.setAttribute("opacity", "0");
  ghost.classList.remove("landed", "pressing");
  hideCaption();
}

/// What the step asks for, drawn on the control itself.
///
/// A ring says which control. It does not say what to do with it, and pointing
/// at a field is not the same instruction as typing in one — so typing gets a
/// caret sitting where the text will go, and everything else is left to the
/// ghost, which lands where the step happens.
function showWhatToDo(b: Bounds, verb: string) {
  const typing = verb === "type";
  const { x, y } = landingPoint(b, verb);

  elementById<SVGGElement>("caret").setAttribute("transform", `translate(${x} ${y})`);
  elementById<SVGGElement>("caret").setAttribute("opacity", typing ? "1" : "0");
  elementById<SVGGElement>("tap").setAttribute("opacity", typing ? "1" : "0");
}

/// How far the dimming fades out around the control.
///
/// A hard-edged hole in a dark sheet reads as a cutout; the point is to draw the
/// eye, not to black out the application someone is working in. The falloff
/// scales with the control so a sidebar row and a whole pane both sit in one.
function haloRadius(b: Bounds): number {
  return Math.max(150, Math.max(b.w, b.h) * 1.6);
}

function applySpotlightBounds(bounds: Bounds) {
  const halo = elementById<SVGCircleElement>("spotlight-halo");
  halo.setAttribute("cx", `${bounds.x + bounds.w / 2}`);
  halo.setAttribute("cy", `${bounds.y + bounds.h / 2}`);
  halo.setAttribute("r", `${haloRadius(bounds)}`);

  const hole = elementById<SVGRectElement>("spotlight-hole");
  const ring = elementById<HTMLElement>("ring");
  showWhatToDo(bounds, currentVerb);

  hole.setAttribute("x", `${bounds.x}`);
  hole.setAttribute("y", `${bounds.y}`);
  hole.setAttribute("width", `${bounds.w}`);
  hole.setAttribute("height", `${bounds.h}`);
  hole.setAttribute("rx", `${bounds.r}`);

  ring.style.left = `${bounds.x}px`;
  ring.style.top = `${bounds.y}px`;
  ring.style.width = `${bounds.w}px`;
  ring.style.height = `${bounds.h}px`;
  ring.style.borderRadius = `${bounds.r}px`;
}

function interpolateValue(start: number, end: number, progress: number): number {
  return start + (end - start) * progress;
}

function interpolateBounds(start: Bounds, end: Bounds, progress: number): Bounds {
  return {
    x: interpolateValue(start.x, end.x, progress),
    y: interpolateValue(start.y, end.y, progress),
    w: interpolateValue(start.w, end.w, progress),
    h: interpolateValue(start.h, end.h, progress),
    r: interpolateValue(start.r, end.r, progress),
  };
}

function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3);
}

function animateSpotlightTransition(from: Bounds, to: Bounds, onComplete?: () => void) {
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId);
  }

  const startTime = performance.now();

  function step(now: number) {
    const elapsed = now - startTime;
    const progress = Math.min(1, elapsed / MORPH_DURATION_MS);
    const easedProgress = easeOutCubic(progress);

    const interpolated = interpolateBounds(from, to, easedProgress);
    applySpotlightBounds(interpolated);
    currentBounds = interpolated;

    if (progress < 1) {
      animationFrameId = requestAnimationFrame(step);
    } else {
      animationFrameId = null;
      currentBounds = to;
      applySpotlightBounds(to);
      onComplete?.();
    }
  }

  animationFrameId = requestAnimationFrame(step);
}

let keepClearOf: Bounds | null = null;

function overlaps(left: number, top: number, box: DOMRect, r: Bounds): boolean {
  return !(left + box.width <= r.x || left >= r.x + r.w || top + box.height <= r.y || top >= r.y + r.h);
}

/// Beside an open menu rather than over it. A menu is the thing being pointed
/// into, and a card laid across its rows hides the row the step is about.
function besideTheMenu(target: Bounds, box: DOMRect, menu: Bounds): { top: number; left: number } | null {
  const edge = 8;
  const level = Math.max(
    edge,
    Math.min(target.y + target.h / 2 - box.height / 2, window.innerHeight - box.height - edge),
  );
  const across = Math.max(edge, Math.min(target.x, window.innerWidth - box.width - edge));
  const places = [
    { left: menu.x + menu.w + GAP, top: level },
    { left: menu.x - box.width - GAP, top: level },
    { left: across, top: menu.y + menu.h + GAP },
    { left: across, top: menu.y - box.height - GAP },
  ];
  for (const p of places) {
    const onScreen =
      p.left >= edge &&
      p.top >= edge &&
      p.left + box.width <= window.innerWidth - edge &&
      p.top + box.height <= window.innerHeight - edge;
    if (onScreen && !overlaps(p.left, p.top, box, menu) && !overlaps(p.left, p.top, box, target)) {
      return p;
    }
  }
  return null;
}

function calculateTooltipPosition(target: Bounds, box: DOMRect): { top: number; left: number } {
  if (keepClearOf) {
    const beside = besideTheMenu(target, box, keepClearOf);
    if (beside) return beside;
  }
  let top = target.y + target.h + GAP;
  if (top + box.height > window.innerHeight - 8) {
    top = Math.max(8, target.y - box.height - GAP);
  }
  let left = target.x + target.w / 2 - box.width / 2;
  left = Math.max(8, Math.min(left, window.innerWidth - box.width - 8));
  return { top, left };
}

/// A curved arrow from the caption to the edge of the control it describes.
///
/// It starts at the middle of whichever side of the card faces the target and
/// ends just off the target's nearest edge, so the head sits beside the control
/// rather than on top of the thing the person is meant to look at.
function drawArrow(tip: DOMRect, target: Bounds) {
  const arrow = elementById<SVGPathElement>("arrow");

  const targetCentre = { x: target.x + target.w / 2, y: target.y + target.h / 2 };
  const cardCentre = { x: tip.left + tip.width / 2, y: tip.top + tip.height / 2 };

  // A card set to one side of its control points across, not up or down.
  const toTheRight = tip.left >= target.x + target.w;
  const toTheLeft = tip.right <= target.x;
  if (toTheRight || toTheLeft) {
    const fromX = toTheRight ? tip.left - 2 : tip.right + 2;
    const toX = toTheRight ? target.x + target.w + 10 : target.x - 10;
    if (Math.abs(fromX - toX) < 24) {
      arrow.setAttribute("opacity", "0");
      return;
    }
    const pull = (toX - fromX) / 2;
    arrow.setAttribute(
      "d",
      `M ${fromX} ${cardCentre.y} C ${fromX + pull} ${cardCentre.y}, ${toX - pull} ${targetCentre.y}, ${toX} ${targetCentre.y}`,
    );
    arrow.setAttribute("opacity", "1");
    return;
  }

  const goingUp = targetCentre.y < cardCentre.y;
  const from = { x: cardCentre.x, y: goingUp ? tip.top - 2 : tip.bottom + 2 };
  const to = {
    x: targetCentre.x,
    y: goingUp ? target.y + target.h + 10 : target.y - 10,
  };

  // Too close to be worth an arrow: the card is already touching the control.
  if (Math.abs(to.y - from.y) < 24) {
    arrow.setAttribute("opacity", "0");
    return;
  }

  const bend = (to.y - from.y) / 2;
  arrow.setAttribute(
    "d",
    `M ${from.x} ${from.y} C ${from.x} ${from.y + bend}, ${to.x} ${to.y - bend}, ${to.x} ${to.y}`,
  );
  arrow.setAttribute("opacity", "1");
}

function pointFromTheTip(tip: DOMRect, target: Bounds) {
  if (runMode === "drive") {
    elementById<SVGPathElement>("arrow").setAttribute("opacity", "0");
    return;
  }
  drawArrow(tip, target);
}

/// Some steps have nothing on screen to point at.
///
/// "Open System Settings" is a step the person carries out before the
/// application it is about exists, so there is no control, no rectangle and
/// nothing to ring. Everything that marks a control is taken off and the
/// instruction is placed in the middle of the screen — a ring drawn around a
/// zero-sized rectangle is a dot in the corner, which is worse than no ring.
function hasSomethingToPointAt(pointer: Pointer): boolean {
  return pointer.w > 0 && pointer.h > 0;
}

function renderInstruction(pointer: Pointer) {
  note(`instruction with no control: ${pointer.step} ${pointer.label}`);

  elementById<SVGElement>("arrow").setAttribute("d", "");
  for (const id of ["tap", "caret"]) {
    elementById<SVGElement>(id).setAttribute("opacity", "0");
  }
  const hole = elementById<SVGRectElement>("spotlight-hole");
  hole.setAttribute("width", "0");
  hole.setAttribute("height", "0");
  elementById<SVGCircleElement>("spotlight-halo").setAttribute("r", "0");
  elementById<HTMLElement>("ring").hidden = true;

  // Nothing is cut out of the dimming, so the whole screen goes quiet behind
  // one sentence. That is the right weight for a step that is about the Mac
  // rather than about a control.
  currentBounds = null;

  const tip = elementById<HTMLElement>("tip");
  tip.style.visibility = "hidden";
  tip.classList.add("on");

  requestAnimationFrame(() => {
    const box = tip.getBoundingClientRect();
    tip.style.top = `${Math.round((window.innerHeight - box.height) / 2)}px`;
    tip.style.left = `${Math.round((window.innerWidth - box.width) / 2)}px`;
    tip.style.visibility = "visible";
    elementById<HTMLElement>("stage").classList.add("on");
  });
}

function setHint(keys: [string, string][]) {
  const hint = elementById<HTMLElement>("hint");
  hint.replaceChildren();
  for (const [key, what] of keys) {
    const item = document.createElement("span");
    const kbd = document.createElement("kbd");
    kbd.textContent = key;
    item.append(kbd, what);
    hint.append(item);
  }
}

const DOTS_MOST = 8;

function setDots(n: number) {
  const dots = elementById<HTMLElement>("dots");
  dots.replaceChildren();
  for (let i = 0; i < Math.min(n, DOTS_MOST); i++) {
    dots.append(document.createElement("span"));
  }
}

function showPaused(why: { ask?: string; label?: string } | null) {
  elementById<SVGGElement>("ghost").classList.add("waiting");
  if (why?.ask?.startsWith("checkpoint: ")) {
    elementById<HTMLElement>("caption").classList.add("asking");
    setCaption(
      `${why.ask.slice("checkpoint: ".length)} \u00B7 next: ${why.label || "the next step"}`,
      "⌥Space to carry on · Esc to stop",
    );
    return;
  }
  if (why?.ask) {
    elementById<HTMLElement>("caption").classList.add("asking");
    setCaption(`About to: ${why.label || "do this"}`, "\u2325Space to go ahead \u00B7 Esc to stop");
    return;
  }
  setCaption("Paused", "⌥Space to carry on · Esc to stop");
}

function renderCoachStep(pointer: Pointer) {
  window.clearTimeout(slowNote);
  elementById<SVGGElement>("ghost").classList.remove("waiting");
  elementById<HTMLElement>("caption").classList.remove("asking");
  runMode = pointer.mode === "drive" ? "drive" : "coach";
  origin = { x: pointer.ox ?? 0, y: pointer.oy ?? 0 };
  const avoid = pointer.avoid ?? [];
  keepClearOf =
    avoid.length === 4 && avoid[2] > 0 && avoid[3] > 0
      ? { x: avoid[0], y: avoid[1], w: avoid[2], h: avoid[3], r: 0 }
      : null;
  const targetBounds = calculateTargetBounds(pointer);
  note(
    `point ${pointer.w}x${pointer.h} at (${pointer.x}, ${pointer.y})` +
      ` in a page of ${window.innerWidth}x${window.innerHeight}`,
  );
  const tip = elementById<HTMLElement>("tip");
  const step = elementById<HTMLElement>("step");
  const text = elementById<HTMLElement>("text");

  const number = stepNumber(pointer);
  const ring = elementById<HTMLElement>("ring");
  ring.hidden = false;
  ring.classList.remove("done");
  elementById<HTMLElement>("badge").textContent = number ? `${number}` : "";
  setDots(number);

  step.textContent = pointer.step;
  text.textContent = pointer.label;
  currentVerb = (pointer.verb ?? "click").toLowerCase();

  // The text to type, or the keys to hold, spelled out rather than described.
  // "Type your name" leaves someone guessing; the words themselves do not.
  const detail = elementById<HTMLElement>("detail");
  const spelled = (pointer.detail ?? "").trim();
  detail.hidden = spelled === "";
  if (currentVerb === "type") {
    detail.textContent = `"${spelled}"`;
  } else if (currentVerb === "scroll") {
    detail.textContent = `${SCROLL_ARROWS[spelled] ?? "↕"} Scroll ${spelled}`;
  } else {
    detail.textContent = spelled;
  }

  tip.classList.toggle("quiet", runMode === "drive");
  setHint(
    pointer.offer
      ? [
          ["\u2318\u21B5", "Do it for me"],
          ...(pointer.offer_all ? [["\u21E7\u2318\u21B5", "And the rest like it"] as [string, string]] : []),
          ["esc", "I\u2019ll do them"],
        ]
      : [
          ["\u2325Space", "Next"],
          ["esc", "Stop"],
        ],
  );

  if (!hasSomethingToPointAt(pointer)) {
    rememberStep(number, null);
    drawTrail(null);
    hideGhost();
    tip.classList.remove("quiet");
    renderInstruction(pointer);
    return;
  }

  const landing = landingPoint(targetBounds, currentVerb);
  const tipBelow = targetBounds.y + targetBounds.h + GAP + 140 <= window.innerHeight - 8;
  captionLift = targetBounds.h / 2 + 30;
  captionAbove = runMode === "coach" && tipBelow && landing.y - captionLift >= 8;
  rememberStep(number, landing);
  drawTrail(landing);
  if (keepClearOf && runMode === "coach") {
    // The card is already beside the menu saying what to do; a second label
    // there would sit on top of it.
    hideCaption();
  } else {
    setCaption(
      captionFor(pointer, currentVerb),
      runMode === "drive" ? "Esc to stop" : pointer.offer ? "\u2318\u21A9 do it for me" : "",
    );
  }

  // Once per step, from here and not from the drawing code: the spotlight is
  // redrawn on every frame while it moves, and a glide restarted on every frame
  // never gets anywhere.
  flyGhostTo(landing, currentVerb === "click" || currentVerb === "");

  if (currentBounds === null) {
    applySpotlightBounds(targetBounds);
    currentBounds = targetBounds;

    tip.style.visibility = "hidden";
    tip.classList.add("on");

    requestAnimationFrame(() => {
      const box = tip.getBoundingClientRect();
      const pos = calculateTooltipPosition(targetBounds, box);
      tip.style.top = `${pos.top}px`;
      tip.style.left = `${pos.left}px`;
      tip.style.visibility = "visible";
      pointFromTheTip(new DOMRect(pos.left, pos.top, box.width, box.height), targetBounds);
      elementById<HTMLElement>("stage").classList.add("on");
    });
  } else {
    const box = tip.getBoundingClientRect();
    const pos = calculateTooltipPosition(targetBounds, box);
    tip.style.top = `${pos.top}px`;
    tip.style.left = `${pos.left}px`;
    pointFromTheTip(new DOMRect(pos.left, pos.top, box.width, box.height), targetBounds);

    animateSpotlightTransition(currentBounds, targetBounds);
  }
}

/// A tick on the last control, so a run that finished says so.
///
/// Until now the overlay simply vanished, which is the same thing it does when
/// a run is stopped or gives up. Those are different outcomes and should not
/// look alike.
function markItDone() {
  window.clearTimeout(slowNote);
  if (!currentBounds) return;

  const ring = elementById<HTMLElement>("ring");
  ring.classList.add("done");
  elementById<HTMLElement>("badge").textContent = "\u2713";

  elementById<SVGGElement>("tap").setAttribute("opacity", "0");
  elementById<SVGPathElement>("arrow").setAttribute("opacity", "0");
  elementById<HTMLElement>("text").textContent = "That is all of it";
  elementById<HTMLElement>("detail").hidden = true;
  setHint([]);
  setCaption("Done", "");
}

let slowNote = 0;

function markItNoticed() {
  if (!currentBounds || runMode !== "coach") return;

  window.clearTimeout(slowNote);
  slowNote = window.setTimeout(() => {
    elementById<HTMLElement>("detail").textContent = "Got it. The model is slow to answer right now\u2026";
  }, 4000);

  elementById<HTMLElement>("ring").classList.add("done");
  elementById<HTMLElement>("badge").textContent = "\u2713";
  elementById<SVGGElement>("tap").setAttribute("opacity", "0");

  const detail = elementById<HTMLElement>("detail");
  detail.hidden = false;
  detail.textContent = "Got it. Next step coming\u2026";
  setHint([["esc", "Stop"]]);
  if (elementById<HTMLElement>("caption").classList.contains("on")) setCaption("Got it", "");
}

function dismissCoach() {
  window.clearTimeout(slowNote);
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId);
    animationFrameId = null;
  }
  elementById<HTMLElement>("stage").classList.remove("on");
  elementById<HTMLElement>("tip").classList.remove("on");
  elementById<SVGPathElement>("arrow").setAttribute("opacity", "0");
  elementById<SVGGElement>("tap").setAttribute("opacity", "0");
  elementById<HTMLElement>("ring").hidden = true;
  hideGhost();
  clearTrail();
  currentBounds = null;
}

// The window is built the moment there is something to point at, and its first
// event is emitted before this file has finished running. Saying when the
// listeners are attached is what gets that event sent again — otherwise the
// first step of every run is drawn to nobody.
void Promise.all([
  listen<Pointer>("coach:point", (event) => renderCoachStep(event.payload)),
  listen("coach:clear", () => dismissCoach()),
  listen("coach:done", () => markItDone()),
  listen("engine://guide_noticed", () => markItNoticed()),
  listen<{ ask?: string; label?: string } | null>("coach:paused", (event) => showPaused(event.payload)),
]).then(() => {
  note("listening");
  return emit("coach://ready");
});
