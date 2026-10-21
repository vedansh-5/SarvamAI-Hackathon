import { listen } from "@tauri-apps/api/event";
import { followTheme } from "./theme";

followTheme();

interface Says {
  state: "listening" | "thinking" | "heard" | "note";
  words: string;
}

const capsule = document.getElementById("capsule") as HTMLElement;
const words = document.getElementById("words") as HTMLElement;
const bars = Array.from(document.querySelectorAll<HTMLElement>("#bars i"));
const SHAPE = [0.55, 0.8, 1, 0.8, 0.55];
const LONGEST = 90;

let hint = "";

function tail(text: string): string {
  const t = text.trim();
  return t.length > LONGEST ? "…" + t.slice(t.length - LONGEST) : t;
}

function write(text: string) {
  const shown = tail(text);
  words.classList.toggle("hint", !shown);
  words.textContent = shown || hint;
}

function level(n: number) {
  bars.forEach((bar, i) => {
    bar.style.transform = `scaleY(${Math.max(0.15, n * SHAPE[i])})`;
  });
}

listen<Says>("voice://says", (e) => {
  const { state, words: said } = e.payload;
  capsule.dataset.state = state;
  capsule.classList.add("on");
  if (state === "listening") {
    hint = said || "Listening…";
    level(0);
    write("");
    return;
  }
  hint = "";
  bars.forEach((bar) => (bar.style.transform = ""));
  words.classList.remove("hint");
  words.textContent = state === "note" ? said : tail(said) || "…";
});

listen<string>("voice://words", (e) => {
  if (capsule.dataset.state === "listening" || capsule.dataset.state === "thinking") write(e.payload);
});

listen<number>("voice://level", (e) => {
  if (capsule.dataset.state === "listening") level(e.payload);
});
