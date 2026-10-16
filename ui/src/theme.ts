import { emit, listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import file from "../../internal/theme/themes.json";

export interface Theme {
  id: string;
  name: string;
  accent: string;
  onAccent: string;
  soft: string;
  ink: string;
}

export const THEMES: Theme[] = file.themes;

const KEY = "vans.accent";
const CHANGED = "vans://accent";

export function themeNamed(id: string | null | undefined): Theme {
  return THEMES.find((t) => t.id === id) ?? THEMES.find((t) => t.id === file.default) ?? THEMES[0];
}

export function savedTheme(): Theme {
  try {
    return themeNamed(localStorage.getItem(KEY));
  } catch {
    return themeNamed(null);
  }
}

function channels(hex: string): string {
  const n = parseInt(hex.slice(1), 16);
  return `${(n >> 16) & 255} ${(n >> 8) & 255} ${n & 255}`;
}

export function paint(t: Theme) {
  const root = document.documentElement.style;
  root.setProperty("--volt", t.accent);
  root.setProperty("--on-volt", t.onAccent);
  root.setProperty("--volt-rgb", channels(t.accent));
  root.setProperty("--soft", t.soft);
  document.documentElement.dataset.accent = t.id;
}

export function followTheme(changed?: (t: Theme) => void): Theme {
  const now = savedTheme();
  paint(now);
  listen<string>(CHANGED, (e) => {
    const t = themeNamed(e.payload);
    paint(t);
    changed?.(t);
  }).catch(() => {});
  return now;
}

export async function chooseTheme(id: string): Promise<Theme> {
  const t = themeNamed(id);
  try {
    localStorage.setItem(KEY, t.id);
  } catch {}
  paint(t);
  await emit(CHANGED, t.id).catch(() => {});
  await invoke("set_accent", { accent: t.id }).catch(() => {});
  return t;
}
