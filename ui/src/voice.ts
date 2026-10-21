import { listen } from "@tauri-apps/api/event";
import { followTheme } from "./theme";

followTheme();

type State = "listening" | "thinking" | "heard" | "speaking" | "note";

interface Says {
  state: State;
  words: string;
}

interface Shape {
  notch: number;
  bar: number;
  width: number;
}

const island = document.getElementById("island") as HTMLElement;
const body = document.getElementById("body") as HTMLElement;
const words = document.getElementById("words") as HTMLElement;
const bars = Array.from(document.querySelectorAll<HTMLElement>(".bars i"));
const PROFILE = [0.45, 0.75, 1, 0.75, 0.45];
const LONGEST = 110;

let hint = "";
let bar = 0;
let target = 0;
let shown = 0;
let moving = false;

function tail(text: string): string {
  const t = text.trim();
  return t.length > LONGEST ? "…" + t.slice(t.length - LONGEST) : t;
}

function fit() {
  if (island.dataset.shape !== "notch") {
    island.style.height = "";
    return;
  }
  island.style.height = island.classList.contains("open") ? `${bar + body.scrollHeight}px` : `${bar}px`;
}

function write(text: string, asHint = false) {
  words.classList.toggle("hint", asHint);
  words.textContent = text;
  fit();
}

function frame(now: number) {
  shown = Math.max(target, shown * 0.88);
  target *= 0.9;
  bars.forEach((b, i) => {
    const pulse = ((Math.sin(now / 160 + i * 0.6) + 1) / 2) * 0.08;
    b.style.transform = `scaleY(${Math.min(1, 0.2 + shown * PROFILE[i] + pulse)})`;
  });
  if (island.dataset.state === "listening" && island.classList.contains("open")) {
    requestAnimationFrame(frame);
  } else {
    moving = false;
  }
}

function animate() {
  if (moving) return;
  moving = true;
  requestAnimationFrame(frame);
}

listen<Shape>("voice://shape", (e) => {
  const { notch, bar: height } = e.payload;
  bar = height;
  island.dataset.shape = notch > 0 ? "notch" : "float";
  document.documentElement.style.setProperty("--notch", `${notch}px`);
  document.documentElement.style.setProperty("--bar", `${height}px`);
  fit();
});

listen<Says>("voice://says", (e) => {
  const { state, words: said } = e.payload;
  island.dataset.state = state;
  island.classList.add("open");
  if (state === "listening") {
    hint = said || "Listening…";
    target = 0;
    shown = 0;
    write(hint, true);
    animate();
    return;
  }
  hint = "";
  write(state === "note" ? said : tail(said) || "…");
});

listen<string>("voice://words", (e) => {
  const state = island.dataset.state;
  if (state !== "listening" && state !== "thinking") return;
  const said = tail(e.payload);
  if (said) write(said);
  else write(hint, true);
});

listen<number>("voice://level", (e) => {
  target = Math.max(target, e.payload);
});

listen("voice://close", () => {
  island.classList.remove("open");
  fit();
});
