import file from "./themes.json";

export interface Theme {
  id: string;
  name: string;
  accent: string;
  onAccent: string;
  soft: string;
  ink: string;
}

export const THEMES: Theme[] = file.themes;
export const DEFAULT_THEME_ID: string = file.default;

export const STORAGE_KEY_ACCENT = "vans-accent";
export const STORAGE_KEY_THEME = "vans-theme";

export function themeNamed(id: string | null | undefined): Theme {
  return THEMES.find((t) => t.id === id) ?? THEMES.find((t) => t.id === file.default) ?? THEMES[0];
}

export function savedTheme(): Theme {
  try {
    return themeNamed(localStorage.getItem(STORAGE_KEY_ACCENT));
  } catch {
    return themeNamed(null);
  }
}

export function channels(hex: string): string {
  const n = parseInt(hex.slice(1), 16);
  return `${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}`;
}

type ThemeListener = (theme: Theme, isDark: boolean) => void;
const listeners: ThemeListener[] = [];

export function onThemeChange(fn: ThemeListener) {
  listeners.push(fn);
}

export function paint(t: Theme) {
  const root = document.documentElement.style;
  root.setProperty("--volt", t.accent);
  root.setProperty("--on-volt", t.onAccent);
  root.setProperty("--volt-rgb", channels(t.accent));
  root.setProperty("--soft", t.soft);
  root.setProperty("--accent-ink", t.ink);
  document.documentElement.dataset.accent = t.id;

  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  for (const fn of listeners) {
    try {
      fn(t, isDark);
    } catch {}
  }
}

export function chooseTheme(id: string): Theme {
  const t = themeNamed(id);
  try {
    localStorage.setItem(STORAGE_KEY_ACCENT, t.id);
  } catch {}
  paint(t);
  updatePickerUi(t.id);
  return t;
}

function updatePickerUi(selectedId: string) {
  const current = themeNamed(selectedId);
  document.querySelectorAll<HTMLElement>(".theme-picker").forEach((picker) => {
    const foldBtn = picker.querySelector<HTMLButtonElement>(".theme-picker-fold");
    if (foldBtn) {
      foldBtn.setAttribute("aria-label", `Theme colour: ${current.name}`);
    }
    picker.querySelectorAll<HTMLButtonElement>(".theme-swatch").forEach((b) => {
      const isSelected = b.dataset.theme === selectedId;
      b.setAttribute("aria-pressed", String(isSelected));
    });
  });
}

export function initThemePicker() {
  const pickers = document.querySelectorAll<HTMLElement>(".theme-picker");
  if (!pickers.length) return;

  const current = savedTheme();

  pickers.forEach((picker) => {
    const swatchesContainer = picker.querySelector<HTMLElement>(".theme-swatches");
    const foldBtn = picker.querySelector<HTMLButtonElement>(".theme-picker-fold");

    if (swatchesContainer) {
      swatchesContainer.replaceChildren(
        ...THEMES.map((t) => {
          const b = document.createElement("button");
          b.type = "button";
          b.className = "theme-swatch";
          b.setAttribute("aria-label", t.name);
          b.setAttribute("aria-pressed", String(t.id === current.id));
          b.dataset.theme = t.id;
          b.style.setProperty("--swatch", t.accent);
          b.addEventListener("click", () => {
            chooseTheme(t.id);
            picker.classList.remove("open");
            foldBtn?.setAttribute("aria-expanded", "false");
          });
          return b;
        }),
      );

      const swatches = Array.from(swatchesContainer.querySelectorAll<HTMLButtonElement>(".theme-swatch"));
      swatches.forEach((s, idx) => {
        s.addEventListener("keydown", (e) => {
          if (e.key === "ArrowRight" || e.key === "ArrowDown") {
            e.preventDefault();
            const next = swatches[(idx + 1) % swatches.length];
            next.focus();
          } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
            e.preventDefault();
            const prev = swatches[(idx - 1 + swatches.length) % swatches.length];
            prev.focus();
          }
        });
      });
    }

    if (foldBtn) {
      foldBtn.addEventListener("click", (e) => {
        e.stopPropagation();
        const isOpen = picker.classList.toggle("open");
        foldBtn.setAttribute("aria-expanded", String(isOpen));
        if (isOpen) {
          const activeSwatch = swatchesContainer?.querySelector<HTMLButtonElement>('.theme-swatch[aria-pressed="true"]');
          activeSwatch?.focus();
        }
      });
    }
  });

  window.addEventListener("click", (e) => {
    pickers.forEach((picker) => {
      if (picker.classList.contains("open") && !picker.contains(e.target as Node)) {
        picker.classList.remove("open");
        const foldBtn = picker.querySelector<HTMLButtonElement>(".theme-picker-fold");
        foldBtn?.setAttribute("aria-expanded", "false");
      }
    });
  });

  window.addEventListener("keydown", (e) => {
    if (e.key === "Escape") {
      pickers.forEach((picker) => {
        if (picker.classList.contains("open")) {
          picker.classList.remove("open");
          const foldBtn = picker.querySelector<HTMLButtonElement>(".theme-picker-fold");
          foldBtn?.setAttribute("aria-expanded", "false");
          foldBtn?.focus();
        }
      });
    }
  });
}

// Paint immediately on module import before first paint
paint(savedTheme());
