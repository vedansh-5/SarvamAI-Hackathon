import { DotField } from "./dots.ts";
import { REQUEST_FACTS, KEYS } from "./facts.ts";

export interface DemoQuery {
  text: string;
  type: "teach" | "answer" | "write";
  answer?: string;
  isWrite?: boolean;
}

export function initLivePanel(container: HTMLElement) {
  if (!container) return;

  const queries: Record<string, DemoQuery> = {};
  for (const rf of REQUEST_FACTS) {
    queries[rf.query] = {
      text: rf.query,
      type: rf.kind === "teaching" ? "teach" : rf.kind === "files" ? "write" : "answer",
      answer: rf.sampleAnswer || "Pointing at the controls on your screen…",
      isWrite: rf.kind === "files",
    };
  }

  const remembered = new Set<string>();
  let state: "asking" | "working" | "answered" | "confirm" = "asking";
  let isSummoned = true;
  let typeTimer: number | null = null;
  let workTimer: number | null = null;

  container.innerHTML = `
    <div class="panel-wrapper" id="panel-wrapper" style="max-width: 720px; margin: 0 auto;">
      <div class="panel" id="vans-live-panel">
        <div class="panel-body">
          <!-- Persistent top field -->
          <div class="field">
            <canvas class="presence wide" id="panel-dot-canvas" width="49" height="21"></canvas>
            <input
              type="text"
              class="ask"
              id="panel-ask-input"
              placeholder="how do I…"
              aria-label="Ask VANS"
              autocomplete="off"
              spellcheck="false"
            />
            <div class="status" id="panel-readout" aria-live="polite"></div>
          </div>

          <div class="line"></div>

          <!-- Body content: suggestions, results, or confirm -->
          <div class="panel-content-area" id="panel-content-area">
            <!-- Chips area -->
            <div class="chip-row" id="panel-chips-row" style="padding: 16px 22px;">
              <span class="kicker volt">ASK</span>
              <div class="chips" id="panel-chips">
                <button type="button" class="chip" data-q="how do I change my wallpaper">
                  <span class="dot"></span><span>change wallpaper</span>
                </button>
                <button type="button" class="chip" data-q="what is this page about">
                  <span class="dot"></span><span>what is this page</span>
                </button>
                <button type="button" class="chip write" data-q="move the screenshots on my Desktop into a folder called Screenshots">
                  <span class="dot"></span><span>move screenshots</span>
                </button>
                <button type="button" class="chip" data-q="what does this error say">
                  <span class="dot"></span><span>what does this error say</span>
                </button>
              </div>
            </div>

            <!-- Answer display -->
            <div class="panel-answer-box" id="panel-answer-box" hidden>
              <div class="plain-text" id="panel-answer-text"></div>
            </div>

            <!-- In-panel Confirm state -->
            <div class="panel-confirm-box" id="panel-confirm-box" hidden>
              <div class="confirm" style="padding: 16px 20px;">
                <div class="confirm-head">
                  <span class="write-square"></span>
                  <span class="write-kicker">CONFIRM</span>
                </div>
                <h3 class="confirm-title" style="font-size: 22px;">Move 3 files into Screenshots</h3>
                <div class="preview-box" style="font-size: 13.5px; max-height: 140px;">Create folder ~/Desktop/Screenshots
Move Screenshot 2026-10-12 at 09.14.22.png
Move Screenshot 2026-10-12 at 14.30.15.png
Move Screenshot 2026-10-13 at 10.02.48.png</div>
                <div class="confirm-foot" style="margin-top: 12px;">
                  <span class="confirm-note mono-note">Hold Return or press &amp; hold</span>
                  <button type="button" class="btn hold-btn" id="panel-hold-btn">
                    <span class="hold-progress" id="panel-hold-progress"></span>
                    <span class="hold-label" id="panel-hold-label">Hold to confirm</span>
                  </button>
                </div>
              </div>
            </div>

            <div class="panel-confirm-done" id="panel-confirm-done" hidden style="padding: 18px 22px;">
              <div class="kicker volt" style="font-size: 11px; margin-bottom: 8px;">DONE</div>
              <div style="font-size: 17px; font-weight: 600; color: var(--ink);">Done · 3 files moved</div>
              <div class="mono-note" style="margin: 8px 0 14px;">Recorded to activity ledger</div>
              <button type="button" class="btn" id="panel-undo-btn">Undo</button>
            </div>
          </div>

          <div class="line"></div>

          <!-- Footer hints -->
          <div class="panel-foot" style="padding: 12px 22px; display: flex; align-items: center; justify-content: space-between;">
            <div class="hints">
              <span class="hint"><kbd>⌥</kbd><kbd>Space</kbd> summon</span>
              <span class="hint"><kbd>${KEYS.ESC}</kbd> step back</span>
              <span class="hint"><kbd>${KEYS.RETURN}</kbd> ask</span>
            </div>
            <div class="mono-note" id="panel-state-tag">ready</div>
          </div>
        </div>
      </div>
    </div>
  `;

  const panelWrapper = container.querySelector<HTMLElement>("#panel-wrapper")!;
  const panelEl = container.querySelector<HTMLElement>("#vans-live-panel")!;
  const input = container.querySelector<HTMLInputElement>("#panel-ask-input")!;
  const readout = container.querySelector<HTMLElement>("#panel-readout")!;
  const chipsRow = container.querySelector<HTMLElement>("#panel-chips-row")!;
  const answerBox = container.querySelector<HTMLElement>("#panel-answer-box")!;
  const answerText = container.querySelector<HTMLElement>("#panel-answer-text")!;
  const confirmBox = container.querySelector<HTMLElement>("#panel-confirm-box")!;
  const confirmDone = container.querySelector<HTMLElement>("#panel-confirm-done")!;
  const stateTag = container.querySelector<HTMLElement>("#panel-state-tag")!;
  const canvas = container.querySelector<HTMLCanvasElement>("#panel-dot-canvas")!;

  // Dot field in header presence
  const dotField = new DotField(canvas, {
    cols: 7,
    rows: 3,
    gap: 7,
    dot: 0.28,
    baseAlpha: 0.35,
  });
  dotField.start();

  function updateReadout(timeText: string) {
    readout.textContent = timeText;
  }

  function setAskingState() {
    state = "asking";
    panelEl.classList.remove("confirming");
    dotField.mode = "listen";
    chipsRow.hidden = false;
    answerBox.hidden = true;
    confirmBox.hidden = true;
    confirmDone.hidden = true;
    stateTag.textContent = "asking";
  }

  function runQuery(qText: string) {
    if (typeTimer) clearTimeout(typeTimer);
    if (workTimer) clearTimeout(workTimer);

    input.value = qText;
    state = "working";
    dotField.mode = "think";
    updateReadout("working…");
    stateTag.textContent = "working…";

    const isRepeat = remembered.has(qText);
    const workDuration = isRepeat ? 280 : 850;

    workTimer = window.setTimeout(() => {
      remembered.add(qText);
      const queryItem = queries[qText];

      if (queryItem?.type === "write") {
        // Confirm state
        state = "confirm";
        panelEl.classList.add("confirming");
        dotField.mode = "speak";
        chipsRow.hidden = true;
        answerBox.hidden = true;
        confirmBox.hidden = false;
        confirmDone.hidden = true;
        updateReadout(isRepeat ? "Remembered · instant" : "1.2 s");
        stateTag.textContent = "confirming";
        setupPanelHold();
        return;
      }

      state = "answered";
      dotField.mode = "done";
      chipsRow.hidden = true;
      confirmBox.hidden = true;
      confirmDone.hidden = true;
      answerBox.hidden = false;

      updateReadout(isRepeat ? "Remembered · instant" : "1.4 s");
      stateTag.textContent = "answered";

      if (queryItem) {
        answerText.textContent = queryItem.answer || "";
      } else {
        answerText.textContent = "This is a demo. In the real panel, VANS would work that out.";
      }
    }, workDuration);
  }

  function setupPanelHold() {
    const holdBtn = container.querySelector<HTMLButtonElement>("#panel-hold-btn");
    const progress = container.querySelector<HTMLElement>("#panel-hold-progress");
    const label = container.querySelector<HTMLElement>("#panel-hold-label");
    const undoBtn = container.querySelector<HTMLButtonElement>("#panel-undo-btn");
    if (!holdBtn || !progress || !label) return;

    let isHolding = false;
    let holdStart = 0;
    let animFrame = 0;
    const HOLD_MS = 750;

    const startHold = () => {
      isHolding = true;
      holdStart = performance.now();
      label.textContent = "Keep holding…";

      const step = (now: number) => {
        if (!isHolding) return;
        const elapsed = now - holdStart;
        const pct = Math.min(100, (elapsed / HOLD_MS) * 100);
        progress.style.width = `${pct}%`;

        if (elapsed >= HOLD_MS) {
          isHolding = false;
          cancelAnimationFrame(animFrame);
          confirmBox.hidden = true;
          confirmDone.hidden = false;
          panelEl.classList.remove("confirming");
          dotField.mode = "done";
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

    holdBtn.onpointerdown = (e) => {
      e.preventDefault();
      startHold();
    };

    window.addEventListener("pointerup", cancelHold);

    undoBtn?.addEventListener("click", () => {
      confirmDone.hidden = true;
      confirmBox.hidden = false;
      panelEl.classList.add("confirming");
      progress.style.width = "0%";
      label.textContent = "Hold to confirm";
    });
  }

  // Type out default "how do I" on first view
  function autoTypeDefault() {
    const target = "how do I change my wallpaper";
    let idx = 0;
    input.value = "";
    dotField.mode = "think";

    function step() {
      if (idx < target.length) {
        input.value += target[idx];
        idx++;
        typeTimer = window.setTimeout(step, 45);
      } else {
        runQuery(target);
      }
    }
    typeTimer = window.setTimeout(step, 800);
  }

  // Escape steps back: answer -> field with words -> empty field -> dismissed
  function handleEscape() {
    if (state === "answered" || state === "confirm") {
      setAskingState();
    } else if (input.value.length > 0) {
      input.value = "";
      updateReadout("");
    } else {
      // Toggle dismissed / summon
      isSummoned = false;
      panelWrapper.style.opacity = "0.2";
      panelWrapper.style.transform = "scale(0.97)";
      updateReadout("dismissed");
    }
  }

  function handleSummon() {
    isSummoned = true;
    panelWrapper.style.opacity = "1";
    panelWrapper.style.transform = "scale(1)";
    input.focus();
    updateReadout("ready");
  }

  // Click chips
  container.querySelectorAll<HTMLButtonElement>(".chip").forEach((chip) => {
    chip.addEventListener("click", () => {
      const q = chip.getAttribute("data-q");
      if (q) runQuery(q);
    });
  });

  // Wire Section 3 cards to live panel
  document.querySelectorAll<HTMLElement>(".request-card").forEach((card) => {
    card.addEventListener("click", () => {
      card.blur();
      const q = card.getAttribute("data-request");
      if (!q) return;

      const panelSec = document.getElementById("two-ways");
      panelSec?.scrollIntoView({ behavior: "smooth", block: "center" });
      runQuery(q);
    });
  });

  // Free typing
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter") {
      const val = input.value.trim();
      if (val) runQuery(val);
    } else if (e.key === "Escape") {
      handleEscape();
    }
  });

  window.addEventListener("keydown", (e) => {
    if (e.altKey && e.code === "Space") {
      e.preventDefault();
      if (!isSummoned) {
        handleSummon();
      } else {
        panelWrapper.scrollIntoView({ behavior: "smooth", block: "center" });
        input.focus();
      }
    } else if (e.key === "Escape" && document.activeElement === input) {
      handleEscape();
    }
  });

  setAskingState();
  autoTypeDefault();
}
