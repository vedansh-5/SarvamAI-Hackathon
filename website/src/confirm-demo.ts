import { KEYS } from "./facts.ts";

export function initConfirmDemo(container: HTMLElement) {
  if (!container) return;

  let isHolding = false;
  let holdStart = 0;
  let animFrame = 0;
  let state: "ready" | "done" | "undone" = "ready";
  const HOLD_DURATION_MS = 750;

  function render() {
    if (state === "ready") {
      container.innerHTML = `
        <div class="panel confirming" style="max-width: 600px; margin: 0 auto; min-height: auto;">
          <div class="confirm">
            <div class="confirm-head">
              <span class="write-square" aria-hidden="true"></span>
              <span class="write-kicker">CONFIRM</span>
            </div>
            <h3 class="confirm-title">Move 3 files into Screenshots</h3>
            <div class="preview-box">Create folder ~/Desktop/Screenshots
Move Screenshot 2026-10-12 at 09.14.22.png
Move Screenshot 2026-10-12 at 14.30.15.png
Move Screenshot 2026-10-13 at 10.02.48.png</div>
            <div class="confirm-foot">
              <span class="confirm-note mono-note">Hold ${KEYS.RETURN} or press &amp; hold button</span>
              <button type="button" class="btn hold-btn" id="confirm-hold-btn" aria-label="Hold to confirm">
                <span class="hold-progress" id="confirm-progress"></span>
                <span class="hold-label" id="confirm-label">Hold to confirm</span>
              </button>
            </div>
          </div>
        </div>
      `;
      setupHoldHandlers();
    } else if (state === "done") {
      container.innerHTML = `
        <div class="panel" style="max-width: 600px; margin: 0 auto; min-height: auto;">
          <div class="confirm">
            <div class="confirm-head">
              <span class="kicker volt" style="font-size: 11px;">DONE</span>
            </div>
            <h3 class="confirm-title" style="font-size: 24px;">Done · 3 files moved</h3>
            <div class="preview-box" style="color: var(--mute);">Created ~/Desktop/Screenshots
Moved 3 screenshot files from Desktop to Screenshots</div>
            <div class="confirm-foot">
              <span class="confirm-note mono-note">Recorded to activity ledger</span>
              <button type="button" class="btn" id="confirm-undo-btn">Undo</button>
            </div>
          </div>
        </div>
      `;
      container.querySelector("#confirm-undo-btn")?.addEventListener("click", () => {
        state = "undone";
        render();
      });
    } else if (state === "undone") {
      container.innerHTML = `
        <div class="panel" style="max-width: 600px; margin: 0 auto; min-height: auto;">
          <div class="confirm">
            <div class="confirm-head">
              <span class="kicker" style="font-size: 11px;">RESTORED</span>
            </div>
            <h3 class="confirm-title" style="font-size: 24px;">Put back · 3 files restored</h3>
            <div class="preview-box" style="color: var(--mute);">Files moved back to ~/Desktop</div>
            <div class="confirm-foot">
              <span class="confirm-note mono-note">Action reversed</span>
              <button type="button" class="btn" id="confirm-reset-btn">Reset demo</button>
            </div>
          </div>
        </div>
      `;
      container.querySelector("#confirm-reset-btn")?.addEventListener("click", () => {
        state = "ready";
        render();
      });
    }
  }

  function setupHoldHandlers() {
    const btn = container.querySelector<HTMLButtonElement>("#confirm-hold-btn");
    const progress = container.querySelector<HTMLElement>("#confirm-progress");
    const label = container.querySelector<HTMLElement>("#confirm-label");
    if (!btn || !progress || !label) return;

    const startHold = () => {
      if (state !== "ready" || isHolding) return;
      isHolding = true;
      holdStart = performance.now();
      label.textContent = "Keep holding…";

      const step = (now: number) => {
        if (!isHolding) return;
        const elapsed = now - holdStart;
        const pct = Math.min(100, (elapsed / HOLD_DURATION_MS) * 100);
        progress.style.width = `${pct}%`;

        if (elapsed >= HOLD_DURATION_MS) {
          isHolding = false;
          cancelAnimationFrame(animFrame);
          state = "done";
          render();
        } else {
          animFrame = requestAnimationFrame(step);
        }
      };
      animFrame = requestAnimationFrame(step);
    };

    const cancelHold = () => {
      if (!isHolding) return;
      isHolding = false;
      cancelAnimationFrame(animFrame);
      progress.style.width = "0%";
      label.textContent = "Hold to confirm";
    };

    btn.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      startHold();
    });

    window.addEventListener("pointerup", cancelHold);
    window.addEventListener("pointercancel", cancelHold);

    btn.addEventListener("keydown", (e) => {
      if (e.key === "Enter" && !e.repeat) {
        startHold();
      }
    });

    btn.addEventListener("keyup", (e) => {
      if (e.key === "Enter") {
        cancelHold();
      }
    });
  }

  render();
}
