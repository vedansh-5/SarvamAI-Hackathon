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

paint({ state: "idle" });
listen<State>("pill://state", (e) => paint(e.payload));

pill.addEventListener("click", () => {
  invoke("summon").catch(() => {});
});
