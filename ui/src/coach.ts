import { emit, listen } from "@tauri-apps/api/event";

interface Pointer {
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  step: string;
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
let animationFrameId: number | null = null;

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

/// Four L-shaped brackets, one per corner, so the edges of the control are
/// marked as well as ringed. The arm is a quarter of the shorter side, capped,
/// so a small control gets small brackets rather than a box drawn twice.
function cornerBrackets(b: Bounds): string {
  const arm = Math.max(8, Math.min(22, Math.min(b.w, b.h) / 4));
  const { x, y, w, h } = b;
  return [
    `M ${x} ${y + arm} L ${x} ${y} L ${x + arm} ${y}`,
    `M ${x + w - arm} ${y} L ${x + w} ${y} L ${x + w} ${y + arm}`,
    `M ${x + w} ${y + h - arm} L ${x + w} ${y + h} L ${x + w - arm} ${y + h}`,
    `M ${x + arm} ${y + h} L ${x} ${y + h} L ${x} ${y + h - arm}`,
  ].join(" ");
}

function applySpotlightBounds(bounds: Bounds) {
  const hole = elementById<SVGRectElement>("spotlight-hole");
  const inner = elementById<SVGRectElement>("spotlight-inner");
  const glow = elementById<SVGRectElement>("spotlight-glow");
  elementById<SVGPathElement>("corners").setAttribute("d", cornerBrackets(bounds));

  hole.setAttribute("x", `${bounds.x}`);
  hole.setAttribute("y", `${bounds.y}`);
  hole.setAttribute("width", `${bounds.w}`);
  hole.setAttribute("height", `${bounds.h}`);
  hole.setAttribute("rx", `${bounds.r}`);

  inner.setAttribute("x", `${bounds.x}`);
  inner.setAttribute("y", `${bounds.y}`);
  inner.setAttribute("width", `${bounds.w}`);
  inner.setAttribute("height", `${bounds.h}`);
  inner.setAttribute("rx", `${bounds.r}`);

  glow.setAttribute("x", `${bounds.x - 3}`);
  glow.setAttribute("y", `${bounds.y - 3}`);
  glow.setAttribute("width", `${bounds.w + 6}`);
  glow.setAttribute("height", `${bounds.h + 6}`);
  glow.setAttribute("rx", `${bounds.r + 3}`);
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

function calculateTooltipPosition(target: Bounds, box: DOMRect): { top: number; left: number } {
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

function renderCoachStep(pointer: Pointer) {
  const targetBounds = calculateTargetBounds(pointer);
  const tip = elementById<HTMLElement>("tip");
  const step = elementById<HTMLElement>("step");
  const text = elementById<HTMLElement>("text");

  step.textContent = pointer.step;
  text.textContent = pointer.label;

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
      drawArrow(new DOMRect(pos.left, pos.top, box.width, box.height), targetBounds);
      elementById<HTMLElement>("stage").classList.add("on");
    });
  } else {
    const box = tip.getBoundingClientRect();
    const pos = calculateTooltipPosition(targetBounds, box);
    tip.style.top = `${pos.top}px`;
    tip.style.left = `${pos.left}px`;
    drawArrow(new DOMRect(pos.left, pos.top, box.width, box.height), targetBounds);

    animateSpotlightTransition(currentBounds, targetBounds);
  }
}

function dismissCoach() {
  if (animationFrameId !== null) {
    cancelAnimationFrame(animationFrameId);
    animationFrameId = null;
  }
  elementById<HTMLElement>("stage").classList.remove("on");
  elementById<HTMLElement>("tip").classList.remove("on");
  elementById<SVGPathElement>("arrow").setAttribute("opacity", "0");
  currentBounds = null;
}

// The window is built the moment there is something to point at, and its first
// event is emitted before this file has finished running. Saying when the
// listeners are attached is what gets that event sent again — otherwise the
// first step of every run is drawn to nobody.
void Promise.all([
  listen<Pointer>("coach:point", (event) => renderCoachStep(event.payload)),
  listen("coach:clear", () => dismissCoach()),
]).then(() => emit("coach://ready"));
