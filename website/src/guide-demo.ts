import { KEYS } from "./facts.ts";

export interface WallpaperItem {
  id: string;
  name: string;
  gradient: string;
  desktopGradient: string;
  category: string;
}

export const WALLPAPERS: WallpaperItem[] = [
  {
    id: "horizon",
    name: "Horizon Wave",
    gradient: "linear-gradient(135deg, #1d4ed8 0%, #4c8df6 40%, #22b8a7 80%, #3dbe6b 100%)",
    desktopGradient: "radial-gradient(ellipse 120% 80% at 50% 20%, #1d4ed8 0%, #0d2875 50%, #081438 100%), linear-gradient(135deg, #4c8df6 0%, #22b8a7 100%)",
    category: "Dynamic",
  },
  {
    id: "solitude",
    name: "Teal Solitude",
    gradient: "linear-gradient(135deg, #0f766e 0%, #22b8a7 50%, #3dbe6b 100%)",
    desktopGradient: "radial-gradient(ellipse 120% 80% at 50% 20%, #0f766e 0%, #083c38 50%, #041c1a 100%), linear-gradient(135deg, #22b8a7 0%, #3dbe6b 100%)",
    category: "Nature",
  },
  {
    id: "violet",
    name: "Violet Dawn",
    gradient: "linear-gradient(135deg, #4c1d95 0%, #8b6cf6 50%, #4c8df6 100%)",
    desktopGradient: "radial-gradient(ellipse 120% 80% at 50% 20%, #4c1d95 0%, #260d52 50%, #12042b 100%), linear-gradient(135deg, #8b6cf6 0%, #4c8df6 100%)",
    category: "Graphic",
  },
  {
    id: "ridge",
    name: "Emerald Ridge",
    gradient: "linear-gradient(135deg, #064e3b 0%, #3dbe6b 60%, #22b8a7 100%)",
    desktopGradient: "radial-gradient(ellipse 120% 80% at 50% 20%, #064e3b 0%, #032b20 50%, #01140e 100%), linear-gradient(135deg, #3dbe6b 0%, #22b8a7 100%)",
    category: "Landscape",
  },
  {
    id: "dusk",
    name: "Coastal Dusk",
    gradient: "linear-gradient(135deg, #1e1b4b 0%, #4c8df6 50%, #8b6cf6 100%)",
    desktopGradient: "radial-gradient(ellipse 120% 80% at 50% 20%, #1e1b4b 0%, #101030 50%, #08081a 100%), linear-gradient(135deg, #4c8df6 0%, #8b6cf6 100%)",
    category: "Coast",
  },
  {
    id: "geometry",
    name: "Geometric Aura",
    gradient: "linear-gradient(135deg, #0e2a47 0%, #4c8df6 35%, #22b8a7 70%, #8b6cf6 100%)",
    desktopGradient: "radial-gradient(ellipse 120% 80% at 50% 20%, #0e2a47 0%, #061524 50%, #020a12 100%), linear-gradient(135deg, #4c8df6 0%, #22b8a7 100%)",
    category: "Abstract",
  },
];

export interface StepDef {
  number: number;
  title: string;
  targetId: string;
  hint: string;
  actionText?: string;
}

export function initGuideDemo(container: HTMLElement) {
  if (!container) return;

  const steps: StepDef[] = [
    {
      number: 1,
      title: "Select Wallpaper in the sidebar",
      targetId: "guide-sidebar-wallpaper",
      hint: `${KEYS.SUMMON} Next · ${KEYS.ESC} Stop`,
      actionText: "Click Wallpaper",
    },
    {
      number: 2,
      title: "Click a wallpaper to set your desktop",
      targetId: "guide-tile-horizon",
      hint: `${KEYS.SUMMON} Next · ${KEYS.ESC} Stop`,
      actionText: "Click Horizon Wave",
    },
    {
      number: 3,
      title: "Choose how the picture fits your display",
      targetId: "guide-select-fit",
      hint: `${KEYS.SUMMON} Next · ${KEYS.ESC} Stop`,
      actionText: "Click Fill Screen",
    },
  ];

  let currentStep = 0;
  let mode: "show" | "do" = "show";
  let isAutoRunning = false;
  let resetTimer: number | null = null;
  let activeWallpaper = WALLPAPERS[0];

  container.innerHTML = `
    <div class="guide-window-frame">
      <div class="guide-mode-bar">
        <div class="seg" role="radiogroup" aria-label="Demo mode">
          <button type="button" class="seg-btn on" id="guide-mode-show" role="radio" aria-checked="true">Show me</button>
          <button type="button" class="seg-btn" id="guide-mode-do" role="radio" aria-checked="false">Do it for me</button>
        </div>
        <div class="guide-mode-caption mono-note" id="guide-caption"></div>
      </div>

      <!-- Desktop Stage behind the window -->
      <div class="desktop-stage" id="guide-desktop-stage">
        <div class="desktop-wallpaper" id="guide-desktop-wallpaper" style="background: ${activeWallpaper.desktopGradient};"></div>

        <div class="mac-window" id="guide-mac-window">
          <!-- Lit Mac Window Titlebar -->
          <div class="mac-titlebar">
            <div class="traffic-lights" aria-hidden="true">
              <span class="light red" title="Close"></span>
              <span class="light yellow" title="Minimize"></span>
              <span class="light green" title="Zoom"></span>
            </div>
            <div class="mac-titlebar-center">
              <span class="mac-title">Wallpaper</span>
            </div>
            <div class="mac-titlebar-right">
              <div class="mac-search-chip">
                <svg width="11" height="11" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
                  <path d="M11.742 10.344a6.5 6.5 0 1 0-1.397 1.398h-.001c.03.04.062.078.098.115l3.85 3.85a1 1 0 0 0 1.415-1.414l-3.85-3.85a1.007 1.007 0 0 0-.115-.1zM12 6.5a5.5 5.5 0 1 1-11 0 5.5 5.5 0 0 1 11 0z"/>
                </svg>
                <span>Search</span>
              </div>
            </div>
          </div>

          <div class="mac-body">
            <!-- Sidebar with real macOS style colored squircle icons -->
            <div class="mac-sidebar" id="guide-mac-sidebar">
              <div class="mac-side-item" id="guide-sidebar-general">
                <span class="mac-icon-squircle icon-general" aria-hidden="true">⚙</span>
                <span class="mac-side-label">General</span>
              </div>
              <div class="mac-side-item" id="guide-sidebar-appearance">
                <span class="mac-icon-squircle icon-appearance" aria-hidden="true">◑</span>
                <span class="mac-side-label">Appearance</span>
              </div>
              <div class="mac-side-item" id="guide-sidebar-control">
                <span class="mac-icon-squircle icon-control" aria-hidden="true">⊞</span>
                <span class="mac-side-label">Control Centre</span>
              </div>
              <div class="mac-side-item active" id="guide-sidebar-wallpaper">
                <span class="mac-icon-squircle icon-wallpaper" aria-hidden="true">🖼</span>
                <span class="mac-side-label">Wallpaper</span>
              </div>
              <div class="mac-side-item" id="guide-sidebar-displays">
                <span class="mac-icon-squircle icon-displays" aria-hidden="true">🖥</span>
                <span class="mac-side-label">Displays</span>
              </div>
              <div class="mac-side-item" id="guide-sidebar-lockscreen">
                <span class="mac-icon-squircle icon-lock" aria-hidden="true">🔒</span>
                <span class="mac-side-label">Lock Screen</span>
              </div>
            </div>

            <!-- Content Area: Wallpaper Settings -->
            <div class="mac-content" id="guide-mac-content">
              <!-- Current Preview Card -->
              <div class="wallpaper-current-card" id="guide-current-card">
                <div class="wallpaper-preview-thumb" id="guide-current-thumb" style="background: ${activeWallpaper.gradient};"></div>
                <div class="wallpaper-current-meta">
                  <div class="wallpaper-current-title" id="guide-current-name">${activeWallpaper.name}</div>
                  <div class="wallpaper-current-sub" id="guide-current-category">${activeWallpaper.category} · Dynamic</div>
                </div>
                <div class="mac-fake-select clickable" id="guide-select-fit">Fill Screen</div>
              </div>

              <!-- Gallery Section -->
              <div class="wallpaper-section-head">
                <span class="section-label">Mac Wallpapers</span>
                <span class="mono-note">Click any tile to preview</span>
              </div>

              <!-- Grid of Vivid Wallpaper Tiles -->
              <div class="wallpaper-grid" id="guide-wallpaper-grid">
                ${WALLPAPERS.map(
                  (wp) => `
                  <button type="button" class="wallpaper-tile ${wp.id === activeWallpaper.id ? "active" : ""}" id="guide-tile-${wp.id}" data-id="${wp.id}" aria-label="Wallpaper: ${wp.name}">
                    <div class="wallpaper-thumb" style="background: ${wp.gradient};">
                      <span class="wallpaper-check" aria-hidden="true">✓</span>
                    </div>
                    <div class="wallpaper-tile-name">${wp.name}</div>
                  </button>
                `,
                ).join("")}
              </div>

              <!-- Extra row for fit options -->
              <div class="mac-group">
                <div class="mac-row" id="guide-row-rotate">
                  <div class="mac-row-text">
                    <div class="mac-row-label">Auto-rotate pictures</div>
                    <div class="mac-row-sub">Change wallpaper on wake or daily</div>
                  </div>
                  <button type="button" class="mac-switch" id="guide-toggle-rotate" aria-label="Toggle auto-rotate" aria-pressed="false">
                    <span class="mac-switch-handle"></span>
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Cutout Overlay for dimmed backdrop around control (D-091) -->
          <svg class="guide-cutout-svg" aria-hidden="true">
            <defs>
              <mask id="guide-mask">
                <rect width="100%" height="100%" fill="white" />
                <rect id="guide-spotlight" x="0" y="0" width="0" height="0" rx="10" fill="black" />
              </mask>
            </defs>
            <rect width="100%" height="100%" fill="rgba(16, 17, 14, 0.4)" mask="url(#guide-mask)" />
          </svg>

          <!-- Volt Ring with Step Badge -->
          <div class="guide-ring" id="guide-ring" aria-hidden="true">
            <div class="guide-badge" id="guide-badge">1</div>
          </div>

          <!-- Gliding Ghost Pointer with VANS tag -->
          <div class="guide-ghost" id="guide-ghost" aria-hidden="true">
            <svg viewBox="0 0 60 60" width="60" height="60" fill="none">
              <!-- Cursor Arrow -->
              <path class="guide-ghost-shape" d="M 4 2 L 20 44 L 27 31 L 43 31 Z" />
              <!-- Small VANS badge attached to cursor -->
              <g class="guide-ghost-tag" transform="translate(32, 28)">
                <rect x="0" y="0" width="34" height="15" rx="3" />
                <text x="5" y="11">VANS</text>
              </g>
              <!-- Ripple ring for simulated click -->
              <circle class="guide-ghost-press" cx="4" cy="2" r="5" opacity="0" />
            </svg>
          </div>

          <!-- Guide Step Card / Tooltip -->
          <div class="guide-tip" id="guide-tip" role="region" aria-live="polite">
            <div class="guide-tip-head">
              <span class="guide-tip-step" id="guide-tip-step">STEP 1 OF 3</span>
              <div class="guide-tip-dots" aria-hidden="true">
                <span class="dot on"></span>
                <span class="dot"></span>
                <span class="dot"></span>
              </div>
            </div>
            <div class="guide-tip-title" id="guide-tip-title">Select Wallpaper in the sidebar</div>
            <div class="guide-tip-foot">
              <span class="guide-tip-hint mono-note" id="guide-tip-hint">⌥Space Next · esc Stop</span>
              <button type="button" class="btn btn-sm guide-tip-next" id="guide-tip-next">Next</button>
            </div>
          </div>
        </div>
      </div>
    </div>
  `;

  // Element handles
  const windowEl = container.querySelector<HTMLElement>("#guide-mac-window")!;
  const ringEl = container.querySelector<HTMLElement>("#guide-ring")!;
  const badgeEl = container.querySelector<HTMLElement>("#guide-badge")!;
  const ghostEl = container.querySelector<HTMLElement>("#guide-ghost")!;
  const tipEl = container.querySelector<HTMLElement>("#guide-tip")!;
  const tipStepEl = container.querySelector<HTMLElement>("#guide-tip-step")!;
  const tipTitleEl = container.querySelector<HTMLElement>("#guide-tip-title")!;
  const tipHintEl = container.querySelector<HTMLElement>("#guide-tip-hint")!;
  const tipNextBtn = container.querySelector<HTMLElement>("#guide-tip-next")!;
  const tipDots = container.querySelectorAll<HTMLElement>(".guide-tip-dots .dot");
  const spotlightEl = container.querySelector<SVGRectElement>("#guide-spotlight")!;
  const modeShowBtn = container.querySelector<HTMLButtonElement>("#guide-mode-show")!;
  const modeDoBtn = container.querySelector<HTMLButtonElement>("#guide-mode-do")!;
  const captionEl = container.querySelector<HTMLElement>("#guide-caption")!;
  const desktopWallpaperEl = container.querySelector<HTMLElement>("#guide-desktop-wallpaper")!;
  const currentThumbEl = container.querySelector<HTMLElement>("#guide-current-thumb")!;
  const currentNameEl = container.querySelector<HTMLElement>("#guide-current-name")!;
  const currentCategoryEl = container.querySelector<HTMLElement>("#guide-current-category")!;
  const sidebarWallpaper = container.querySelector<HTMLElement>("#guide-sidebar-wallpaper")!;
  const selectFit = container.querySelector<HTMLElement>("#guide-select-fit")!;
  const toggleRotate = container.querySelector<HTMLElement>("#guide-toggle-rotate")!;

  function applyWallpaper(wp: WallpaperItem) {
    activeWallpaper = wp;
    if (desktopWallpaperEl) {
      desktopWallpaperEl.style.background = wp.desktopGradient;
    }
    if (currentThumbEl) {
      currentThumbEl.style.background = wp.gradient;
    }
    if (currentNameEl) {
      currentNameEl.textContent = wp.name;
    }
    if (currentCategoryEl) {
      currentCategoryEl.textContent = `${wp.category} · Dynamic`;
    }

    // Update active class on tiles
    container.querySelectorAll<HTMLElement>(".wallpaper-tile").forEach((tile) => {
      if (tile.getAttribute("data-id") === wp.id) {
        tile.classList.add("active");
      } else {
        tile.classList.remove("active");
      }
    });
  }

  // Interactive clicks on any wallpaper tile
  container.querySelectorAll<HTMLElement>(".wallpaper-tile").forEach((tile) => {
    tile.addEventListener("click", () => {
      const id = tile.getAttribute("data-id");
      const found = WALLPAPERS.find((w) => w.id === id);
      if (found) {
        applyWallpaper(found);
        if (currentStep === 1) {
          advanceStep();
        }
      }
    });
  });

  // Position ring and pointer relative to target
  function updatePosition(animate = true) {
    if (currentStep >= steps.length) return;
    const step = steps[currentStep];
    const target = container.querySelector<HTMLElement>(`#${step.targetId}`);
    if (!target || !windowEl) return;

    const strip = container.querySelector<HTMLElement>("#guide-mac-sidebar");
    if (strip && strip.contains(target) && strip.scrollWidth > strip.clientWidth) {
      const hidden = target.offsetLeft + target.offsetWidth > strip.scrollLeft + strip.clientWidth || target.offsetLeft < strip.scrollLeft;
      if (hidden) strip.scrollLeft = Math.max(0, target.offsetLeft - 12);
    }

    const winRect = windowEl.getBoundingClientRect();
    const targetRect = target.getBoundingClientRect();

    const relX = targetRect.left - winRect.left;
    const relY = targetRect.top - winRect.top;
    const pad = 4;
    const ringW = targetRect.width + pad * 2;
    const ringH = targetRect.height + pad * 2;
    const ringX = relX - pad;
    const ringY = relY - pad;

    // Apply positioning
    ringEl.style.transition = animate ? "all 0.35s cubic-bezier(0.2, 0.8, 0.2, 1)" : "none";
    ringEl.style.transform = `translate3d(${ringX}px, ${ringY}px, 0)`;
    ringEl.style.width = `${ringW}px`;
    ringEl.style.height = `${ringH}px`;
    ringEl.classList.remove("done");
    badgeEl.textContent = String(step.number);

    // Spotlight cutout in SVG overlay
    spotlightEl.setAttribute("x", String(ringX));
    spotlightEl.setAttribute("y", String(ringY));
    spotlightEl.setAttribute("width", String(ringW));
    spotlightEl.setAttribute("height", String(ringH));

    // Pointer glides to top-left corner of the ringed control
    ghostEl.style.transition = animate ? "transform 0.4s cubic-bezier(0.2, 0.8, 0.2, 1), opacity 0.2s" : "none";
    ghostEl.style.transform = `translate3d(${ringX + 8}px, ${ringY + 8}px, 0)`;
    ghostEl.style.opacity = "1";

    // Position tip card strictly clear of the ringed control and content beside it
    tipEl.style.transition = animate ? "transform 0.38s cubic-bezier(0.2, 0.8, 0.2, 1), opacity 0.2s" : "none";

    const isMobile = window.innerWidth <= 768;
    const isStep1 = step.targetId === "guide-sidebar-wallpaper";
    tipEl.classList.toggle("narrow", !isMobile && isStep1);

    const galleryGrid = container.querySelector<HTMLElement>("#guide-wallpaper-grid");
    const gridRect = galleryGrid ? galleryGrid.getBoundingClientRect() : null;

    let tipX = 24;
    let tipY = 24;

    if (isMobile) {
      // On mobile, position card comfortably below the active row or tiles
      if (gridRect) {
        tipY = gridRect.bottom - winRect.top + 14;
      } else {
        tipY = ringY + ringH + 16;
      }
      tipX = Math.max(12, Math.min(relX, winRect.width - 290));
    } else {
      // On desktop:
      if (isStep1) {
        const last = target.parentElement?.lastElementChild;
        const listBottom = last ? last.getBoundingClientRect().bottom - winRect.top : relY + ringH;
        tipX = 10;
        tipY = listBottom + 16;
      } else if (step.targetId === "guide-tile-horizon") {
        // Step 2: Below the wallpaper grid so all 6 tiles remain visible
        if (gridRect) {
          tipY = gridRect.bottom - winRect.top + 14;
          tipX = Math.max(220, Math.min(relX, winRect.width - 290));
        } else {
          tipY = ringY + ringH + 18;
          tipX = relX;
        }
      } else {
        // Step 3: Beside or below the control
        tipX = Math.max(220, Math.min(relX - 220, winRect.width - 290));
        tipY = relY + ringH + 14;
      }
    }

    // Keep tip inside window bounds
    const maxTipY = Math.max(380, winRect.height - 130);
    tipY = Math.min(tipY, maxTipY);

    tipEl.style.transform = `translate3d(${tipX}px, ${tipY}px, 0)`;
    tipEl.style.opacity = "1";
  }

  function renderStep() {
    if (currentStep < steps.length) {
      const step = steps[currentStep];
      tipStepEl.textContent = `STEP ${step.number} OF ${steps.length}`;
      tipTitleEl.textContent = step.title;
      tipHintEl.textContent = mode === "do" ? "esc to stop" : step.hint;
      tipNextBtn.textContent = step.number === steps.length ? "Done" : "Next";

      tipDots.forEach((dot, idx) => {
        dot.classList.toggle("on", idx === currentStep);
      });

      captionEl.textContent = mode === "do" ? "esc to stop" : "";
      updatePosition(true);
    } else {
      // Completed state: checkmark on ring
      ringEl.classList.add("done");
      badgeEl.textContent = "✓";
      tipStepEl.textContent = "COMPLETED";
      tipTitleEl.textContent = "That is all of it";
      tipHintEl.textContent = "Resetting in a moment…";
      tipNextBtn.textContent = "Restart";
      ghostEl.style.opacity = "0";

      tipDots.forEach((dot) => dot.classList.add("on"));

      if (resetTimer) window.clearTimeout(resetTimer);
      resetTimer = window.setTimeout(() => {
        reset();
      }, 4000);
    }
  }

  function advanceStep() {
    if (currentStep < steps.length) {
      currentStep++;
      renderStep();
    } else {
      reset();
    }
  }

  function reset() {
    if (resetTimer) window.clearTimeout(resetTimer);
    resetTimer = null;
    currentStep = 0;
    isAutoRunning = false;
    mode = "show";
    modeShowBtn.classList.add("on");
    modeShowBtn.setAttribute("aria-checked", "true");
    modeDoBtn.classList.remove("on");
    modeDoBtn.setAttribute("aria-checked", "false");
    renderStep();
  }

  // Next button click
  tipNextBtn.addEventListener("click", () => {
    if (currentStep === 1) {
      applyWallpaper(WALLPAPERS[0]);
    }
    advanceStep();
  });

  // Step 1: Click Wallpaper in sidebar
  sidebarWallpaper.addEventListener("click", () => {
    if (currentStep === 0) {
      sidebarWallpaper.classList.add("active");
      advanceStep();
    }
  });

  // Step 3: Select dropdown or toggle
  selectFit.addEventListener("click", () => {
    selectFit.textContent = selectFit.textContent === "Fill Screen" ? "Fit to Screen" : "Fill Screen";
    if (currentStep === 2) {
      advanceStep();
    }
  });

  toggleRotate.addEventListener("click", () => {
    const isPressed = toggleRotate.getAttribute("aria-pressed") === "true";
    toggleRotate.setAttribute("aria-pressed", isPressed ? "false" : "true");
    toggleRotate.classList.toggle("active", !isPressed);
    if (currentStep === 2) {
      advanceStep();
    }
  });

  // Do it for me simulation
  function runAutoGuide() {
    isAutoRunning = true;
    mode = "do";
    currentStep = 0;
    renderStep();

    function autoStep() {
      if (!isAutoRunning || mode !== "do") return;

      if (currentStep === 0) {
        // Glide and click Wallpaper in sidebar
        ghostEl.classList.add("pressing");
        window.setTimeout(() => {
          ghostEl.classList.remove("pressing");
          sidebarWallpaper.classList.add("active");
          currentStep = 1;
          renderStep();
          window.setTimeout(autoStep, 1000);
        }, 600);
      } else if (currentStep === 1) {
        // Glide and click Horizon Wave wallpaper tile
        ghostEl.classList.add("pressing");
        window.setTimeout(() => {
          ghostEl.classList.remove("pressing");
          applyWallpaper(WALLPAPERS[0]);
          currentStep = 2;
          renderStep();
          window.setTimeout(autoStep, 1000);
        }, 700);
      } else if (currentStep === 2) {
        // Glide and click Fill Screen
        ghostEl.classList.add("pressing");
        window.setTimeout(() => {
          ghostEl.classList.remove("pressing");
          selectFit.textContent = "Fill Screen";
          currentStep = 3;
          renderStep();
        }, 600);
      }
    }

    window.setTimeout(autoStep, 600);
  }

  modeShowBtn.addEventListener("click", () => {
    reset();
  });

  modeDoBtn.addEventListener("click", () => {
    modeShowBtn.classList.remove("on");
    modeShowBtn.setAttribute("aria-checked", "false");
    modeDoBtn.classList.add("on");
    modeDoBtn.setAttribute("aria-checked", "true");
    runAutoGuide();
  });

  // Keyboard navigation: ⌥Space advances step, esc stops/resets
  window.addEventListener("keydown", (e) => {
    if (e.altKey && e.code === "Space") {
      e.preventDefault();
      advanceStep();
    } else if (e.key === "Escape") {
      if (mode === "do" || currentStep > 0) {
        reset();
      }
    }
  });

  // Resize and scroll listeners to keep ring & tooltip aligned
  window.addEventListener("resize", () => {
    updatePosition(false);
  });

  const sidebarEl = container.querySelector<HTMLElement>("#guide-mac-sidebar");
  if (sidebarEl) {
    sidebarEl.addEventListener("scroll", () => {
      updatePosition(false);
    });
  }

  // Initial render
  renderStep();
}
