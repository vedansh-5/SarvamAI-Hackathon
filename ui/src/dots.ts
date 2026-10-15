export type DotMode = "idle" | "think" | "speak" | "listen" | "done";

export interface DotShape {
  cols: number;
  rows: number;
  gap: number;
  dot: number;
  baseAlpha: number;
}

function rgbOf(color: string): [number, number, number] {
  const hex = color.trim().replace("#", "");
  if (/^[0-9a-f]{3}$/i.test(hex)) {
    return [0, 1, 2].map((i) => parseInt(hex[i] + hex[i], 16)) as [number, number, number];
  }
  if (/^[0-9a-f]{6}$/i.test(hex)) {
    const n = parseInt(hex, 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }
  const parts = color.match(/\d+(\.\d+)?/g);
  if (parts && parts.length >= 3) {
    return [Number(parts[0]), Number(parts[1]), Number(parts[2])];
  }
  return [128, 128, 128];
}

export class DotField {
  mode: DotMode = "idle";
  private accent: [number, number, number] = [128, 128, 128];
  private base: [number, number, number] = [128, 128, 128];
  private level = 0;
  private columns: number[] = [];
  private frame = 0;

  constructor(
    private canvas: HTMLCanvasElement,
    private shape: DotShape,
  ) {
    canvas.style.width = `${shape.cols * shape.gap}px`;
    canvas.style.height = `${shape.rows * shape.gap}px`;
  }

  colours(accent: string, base: string) {
    this.accent = rgbOf(accent);
    this.base = rgbOf(base);
  }

  start() {
    if (this.frame) return;
    const loop = (now: number) => {
      this.draw(now / 1000);
      this.frame = requestAnimationFrame(loop);
    };
    this.frame = requestAnimationFrame(loop);
  }

  stop() {
    cancelAnimationFrame(this.frame);
    this.frame = 0;
  }

  draw(t: number) {
    const ctx = this.canvas.getContext("2d");
    if (!ctx) return;

    const { cols, rows, gap, dot, baseAlpha } = this.shape;
    const dpr = window.devicePixelRatio || 1;
    const width = cols * gap;
    const height = rows * gap;
    if (this.canvas.width !== Math.round(width * dpr) || this.canvas.height !== Math.round(height * dpr)) {
      this.canvas.width = Math.round(width * dpr);
      this.canvas.height = Math.round(height * dpr);
    }
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, width, height);

    const [ar, ag, ab] = this.accent;
    const [br, bg, bb] = this.base;
    const mode = this.mode;

    let target = 0;
    if (mode === "listen") target = 0.35 + 0.65 * Math.abs(Math.sin(t * 6.3)) * Math.abs(Math.sin(t * 1.9 + 1));
    else if (mode === "speak") target = 0.3 + 0.7 * Math.abs(Math.sin(t * 5.1) * Math.sin(t * 2.3 + 0.5));
    this.level += (target - this.level) * 0.18;

    const cx = (cols - 1) / 2;
    const cy = (rows - 1) / 2;
    const furthest = Math.hypot(cx, cy) || 1;
    const radius = gap * dot;

    for (let x = 0; x < cols; x++) {
      let column = 0;
      if (mode === "listen") {
        const noise = Math.abs(Math.sin(t * 3.3 + x * 0.9) * 0.6 + Math.sin(t * 5.7 + x * 0.37) * 0.4);
        const envelope = 1 - Math.pow(Math.abs(x - cx) / (cx + 1), 2) * 0.7;
        const want = this.level * noise * envelope * 1.15;
        this.columns[x] = (this.columns[x] || 0) + (want - (this.columns[x] || 0)) * 0.25;
        column = this.columns[x];
      }

      for (let y = 0; y < rows; y++) {
        const d = Math.hypot(x - cx, y - cy) / furthest;
        let v: number;
        if (mode === "listen") {
          v = Math.abs(y - cy) / (cy + 0.5) <= column ? 1 - (Math.abs(y - cy) / (cy + 1)) * 0.45 : 0.06;
        } else if (mode === "think") {
          const a = t * 2.4;
          const rx = cx * 0.78;
          const ry = cy * 0.8;
          const p1x = cx + Math.cos(a) * rx;
          const p1y = cy + Math.sin(a) * ry;
          const p2x = cx + Math.cos(-a * 0.7 + 2) * rx * 0.55;
          const p2y = cy + Math.sin(-a * 0.7 + 2) * ry * 0.55;
          const spread = Math.max(1.4, cols * 0.05);
          const d1 = Math.hypot(x - p1x, (y - p1y) * 1.6);
          const d2 = Math.hypot(x - p2x, (y - p2y) * 1.6);
          v = Math.max(Math.exp(-(d1 * d1) / (spread * 2)), 0.85 * Math.exp(-(d2 * d2) / spread)) + 0.06;
        } else if (mode === "speak") {
          v = 0.06 + this.level * (1 - d) * (0.55 + 0.45 * (Math.sin(d * 10 - t * 7) * 0.5 + 0.5)) * 1.2;
        } else if (mode === "done") {
          v = 0.95 - d * 0.4;
        } else {
          v = 0.15 + 0.6 * Math.max(0, Math.sin(t * 1.8 - (x + y) * 0.9));
        }
        v = Math.max(0, Math.min(1, v));

        ctx.fillStyle =
          v > 0.4 ? `rgba(${ar},${ag},${ab},${0.4 + 0.6 * v})` : `rgba(${br},${bg},${bb},${baseAlpha})`;
        ctx.beginPath();
        ctx.arc(x * gap + gap / 2, y * gap + gap / 2, radius * (0.55 + 0.95 * v), 0, Math.PI * 2);
        ctx.fill();
      }
    }
  }
}
