import { listen } from "@tauri-apps/api/event";

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

function applySpotlightBounds(bounds: Bounds) {
  const hole = elementById<SVGRectElement>("spotlight-hole");
  const inner = elementById<SVGRectElement>("spotlight-inner");
  const glow = elementById<SVGRectElement>("spotlight-glow");

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
      elementById<HTMLElement>("stage").classList.add("on");
    });
  } else {
    const box = tip.getBoundingClientRect();
    const pos = calculateTooltipPosition(targetBounds, box);
    tip.style.top = `${pos.top}px`;
    tip.style.left = `${pos.left}px`;

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
  currentBounds = null;
}

listen<Pointer>("coach:point", (event) => renderCoachStep(event.payload));
listen("coach:clear", () => dismissCoach());
