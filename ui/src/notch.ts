import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";

interface State {
  state: "idle" | "working" | "guiding" | "asking";
}

const pill = document.getElementById("pill") as HTMLElement;

function paint(s: State) {
  pill.classList.toggle("working", s.state === "working");
  pill.classList.toggle("guiding", s.state === "guiding" || s.state === "asking");
}

interface Watching {
  on: boolean;
}

function watched(w: Watching) {
  pill.classList.toggle("unwatched", !w.on);
  pill.title = w.on ? "VANS (⌥Space) · learning from your clicks" : "VANS (⌥Space)";
}

paint({ state: "idle" });
listen<State>("pill://state", (e) => paint(e.payload));
listen<Watching>("pill://watching", (e) => watched(e.payload));
invoke<Watching>("watching").then(watched).catch(() => {});

pill.addEventListener("click", () => {
  invoke("summon").catch(() => {});
});
