import { listen } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";

interface Place {
  notch: number;
}

interface State {
  state: "idle" | "working" | "guiding" | "asking";
  text?: string;
}

const pill = document.getElementById("pill") as HTMLElement;
const text = document.getElementById("text") as HTMLElement;

function fit(place: Place | null) {
  const notch = place?.notch ?? 0;
  pill.classList.toggle("bare", notch === 0);
  pill.style.setProperty("--notch", `${notch}px`);
}

function paint(s: State) {
  pill.classList.toggle("working", s.state === "working");
  pill.classList.toggle("guiding", s.state === "guiding" || s.state === "asking");
  text.textContent = s.text ?? (pill.classList.contains("bare") ? "VANS" : "");
}

invoke<Place | null>("notch_place").then(fit).catch(() => fit(null));
paint({ state: "idle" });

listen<Place>("notch://place", (e) => fit(e.payload));
listen<State>("pill://state", (e) => paint(e.payload));

pill.addEventListener("click", () => {
  invoke("summon").catch(() => {});
});
