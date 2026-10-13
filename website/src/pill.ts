const EASE = 0.15;
const SETTLED = 0.4;

interface Point {
  x: number;
  y: number;
}

export function livingPill(pill: HTMLElement) {
  const still = window.matchMedia("(prefers-reduced-motion: reduce)");
  let box = pill.getBoundingClientRect();
  let at: Point = { x: 0, y: 0 };
  let to: Point = { x: 0, y: 0 };
  let frame = 0;

  const paint = () => {
    pill.style.setProperty("--mx", `${at.x.toFixed(1)}px`);
    pill.style.setProperty("--my", `${at.y.toFixed(1)}px`);
  };

  const snap = (p: Point) => {
    at = { ...p };
    to = { ...p };
    paint();
  };

  const step = () => {
    at = { x: at.x + (to.x - at.x) * EASE, y: at.y + (to.y - at.y) * EASE };
    const settled = Math.abs(to.x - at.x) < SETTLED && Math.abs(to.y - at.y) < SETTLED;
    if (settled) at = { ...to };
    paint();
    frame = settled ? 0 : requestAnimationFrame(step);
  };

  const within = (e: PointerEvent): Point => ({ x: e.clientX - box.left, y: e.clientY - box.top });

  pill.addEventListener("pointerenter", (e) => {
    box = pill.getBoundingClientRect();
    snap(within(e));
  });

  pill.addEventListener("pointermove", (e) => {
    to = within(e);
    if (still.matches) {
      snap(to);
      return;
    }
    if (!frame) frame = requestAnimationFrame(step);
  });

  pill.addEventListener("focus", () => {
    if (!pill.matches(":focus-visible")) return;
    box = pill.getBoundingClientRect();
    cancelAnimationFrame(frame);
    frame = 0;
    snap({ x: box.width / 2, y: box.height / 2 });
  });
}

export function setPillLabel(pill: HTMLElement, text: string) {
  for (const part of Array.from(pill.querySelectorAll(".vans-pill-label, .vans-pill-halo"))) {
    part.textContent = text;
  }
}
