import { listen } from "@tauri-apps/api/event";

interface Pointer {
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  step: string;
}

const PAD = 6;
const GAP = 14;

function $(id: string): HTMLElement {
  const el = document.getElementById(id);
  if (!el) throw new Error(`missing #${id}`);
  return el;
}

/// The screen is dimmed everywhere except the thing being pointed at.
///
/// A ring on its own competes with whatever the application already draws;
/// taking the light off everything else does not, and it reads instantly even
/// on a busy window. The hole is punched with a mask rather than by drawing
/// four rectangles around the target, so the edge stays exact at any radius.
function draw(p: Pointer) {
  const x = p.x - PAD;
  const y = p.y - PAD;
  const w = p.w + PAD * 2;
  const h = p.h + PAD * 2;
  const r = Math.min(12, Math.max(6, Math.min(w, h) / 4));

  $("art").innerHTML = `
    <defs>
      <mask id="cut">
        <rect width="100%" height="100%" fill="white"/>
        <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="${r}" fill="black"/>
      </mask>
    </defs>
    <rect width="100%" height="100%" fill="rgba(0,0,0,0.42)" mask="url(#cut)"/>
    <rect x="${x}" y="${y}" width="${w}" height="${h}" rx="${r}"
          fill="none" stroke="rgba(255,255,255,0.9)" stroke-width="2"/>
    <rect x="${x - 3}" y="${y - 3}" width="${w + 6}" height="${h + 6}" rx="${r + 3}"
          fill="none" stroke="rgba(10,132,255,0.55)" stroke-width="3">
      <animate attributeName="opacity" values="0.8;0.25;0.8" dur="1.8s" repeatCount="indefinite"/>
    </rect>`;

  const tip = $("tip");
  $("step").textContent = p.step;
  $("text").textContent = p.label;

  tip.style.visibility = "hidden";
  tip.classList.add("on");
  requestAnimationFrame(() => {
    const box = tip.getBoundingClientRect();
    let top = y + h + GAP;
    if (top + box.height > window.innerHeight - 8) top = Math.max(8, y - box.height - GAP);
    let left = x + w / 2 - box.width / 2;
    left = Math.max(8, Math.min(left, window.innerWidth - box.width - 8));
    tip.style.top = `${top}px`;
    tip.style.left = `${left}px`;
    tip.style.visibility = "visible";
  });

  $("stage").classList.add("on");
}

listen<Pointer>("coach:point", (e) => draw(e.payload));

listen("coach:clear", () => {
  $("stage").classList.remove("on");
  $("tip").classList.remove("on");
});
