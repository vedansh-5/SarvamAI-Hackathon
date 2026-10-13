export function initHeroDotBackground(canvasId: string = "bg-canvas") {
  const canvas = document.getElementById(canvasId) as HTMLCanvasElement | null;
  if (!canvas) return;

  const ctx = canvas.getContext("2d");
  if (!ctx) return;

  const still = window.matchMedia("(prefers-reduced-motion: reduce)");
  let isRunning = false;
  let animId = 0;
  let mouse = { x: -1000, y: -1000 };
  let width = 0;
  let height = 0;

  const GAP = 28;
  const BASE_R = 1.2;

  function resize() {
    const dpr = window.devicePixelRatio || 1;
    width = window.innerWidth;
    height = window.innerHeight;
    canvas!.width = Math.round(width * dpr);
    canvas!.height = Math.round(height * dpr);
    ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
  }

  function render(t: number) {
    if (!ctx) return;
    ctx.clearRect(0, 0, width, height);

    const isLight = document.documentElement.getAttribute("data-theme") === "light";
    const baseRgb = isLight ? "16, 17, 14" : "243, 244, 238";
    const voltRgb = "255, 210, 63";

    const cols = Math.ceil(width / GAP) + 1;
    const rows = Math.ceil(height / GAP) + 1;

    for (let c = 0; c < cols; c++) {
      const x = c * GAP;
      for (let r = 0; r < rows; r++) {
        const y = r * GAP;

        // Subtle slow drift
        const wave = Math.sin(t * 0.0006 + (c * 0.2 + r * 0.15)) * 0.5 + 0.5;
        const dist = Math.hypot(x - mouse.x, y - mouse.y);
        const near = Math.max(0, 1 - dist / 220);

        let alpha = 0.035 + wave * 0.035 + near * 0.25;
        let radius = BASE_R + near * 1.5;

        ctx.fillStyle = near > 0.15 ? `rgba(${voltRgb}, ${alpha})` : `rgba(${baseRgb}, ${alpha})`;
        ctx.beginPath();
        ctx.arc(x, y, radius, 0, Math.PI * 2);
        ctx.fill();
      }
    }
  }

  function loop(timestamp: number) {
    if (!isRunning) return;
    render(timestamp);
    animId = requestAnimationFrame(loop);
  }

  function start() {
    if (isRunning) return;
    isRunning = true;
    if (still.matches) {
      render(0);
      return;
    }
    animId = requestAnimationFrame(loop);
  }

  function stop() {
    isRunning = false;
    cancelAnimationFrame(animId);
  }

  window.addEventListener("resize", () => {
    resize();
    if (still.matches) render(0);
  });

  window.addEventListener("pointermove", (e) => {
    mouse.x = e.clientX;
    mouse.y = e.clientY;
    if (still.matches) render(0);
  });

  window.addEventListener("pointerleave", () => {
    mouse.x = -1000;
    mouse.y = -1000;
    if (still.matches) render(0);
  });

  // Pause when tab hidden
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) stop();
    else start();
  });

  // Pause when hero off-screen
  const hero = document.getElementById("hero");
  if (hero) {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) start();
        else stop();
      });
    });
    observer.observe(hero);
  }

  resize();
  start();
}
