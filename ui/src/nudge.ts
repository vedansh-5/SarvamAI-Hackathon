import { listen, emit } from "@tauri-apps/api/event";
import { invoke } from "@tauri-apps/api/core";
import { followTheme } from "./theme";

followTheme();

interface Offer {
  key: string;
  line: string;
  when?: string;
  always?: boolean;
  countdown?: boolean;
  morning?: boolean;
  usual?: string[];
  update?: boolean;
}

interface Answer {
  instruction?: string;
  open?: string;
}

const SHOWN_FOR = 8000;
const AFTER_LEAVING = 4000;
const COUNT_FROM = 5;
const ROW = 46;
const USUAL_ROW = 18;

const $ = (id: string) => document.getElementById(id) as HTMLElement;
const card = $("card");
const queue: Offer[] = [];
let current: Offer | null = null;
let fade: number | undefined;
let tick: number | undefined;

function stopTimers() {
  window.clearTimeout(fade);
  window.clearInterval(tick);
}

function fadeIn(after: number) {
  window.clearTimeout(fade);
  fade = window.setTimeout(() => answer("ignored"), after);
}

async function act(got: Answer) {
  if (got.open) {
    await invoke("open_app", { name: got.open }).catch(console.error);
  } else if (got.instruction) {
    await emit("nudge://run", { instruction: got.instruction });
  }
}

async function answer(what: string) {
  const offer = current;
  if (!offer) return;
  stopTimers();
  current = null;
  card.classList.remove("shown");
  await invoke("hide_nudge").catch(() => {});
  if (offer.update) {
    if (what === "run") await invoke("restart_app").catch(console.error);
    void next();
    return;
  }
  try {
    const got = await invoke<Answer>("answer_offer", { key: offer.key, answer: what });
    if (what === "run" || what === "always" || what === "ran") await act(got);
  } catch (e) {
    console.error(e);
  }
  void next();
}

function buttons(offer: Offer, counting: boolean) {
  $("run").textContent = offer.update ? "Restart" : "Run";
  const shown: Record<string, boolean> = {
    run: !offer.morning && !counting,
    always: !offer.morning && !counting && !!offer.always,
    later: !offer.morning && !counting,
    skip: !offer.morning && !counting && !offer.update,
    cancel: counting,
    ok: !!offer.morning,
  };
  for (const [id, on] of Object.entries(shown)) $(id).hidden = !on;
}

function countDown(offer: Offer) {
  let left = COUNT_FROM;
  const doing = offer.line.replace(/, like usual\?$/, "");
  const say = () => ($("line").textContent = `${doing} in ${left} s`);
  say();
  tick = window.setInterval(() => {
    left -= 1;
    if (left <= 0) {
      void answer("ran");
      return;
    }
    say();
  }, 1000);
}

async function next() {
  if (current || queue.length === 0) return;
  const offer = queue.shift()!;
  current = offer;
  $("line").textContent = offer.line;
  const usual = $("usual");
  usual.replaceChildren(
    ...(offer.usual ?? []).slice(0, 4).map((u) => {
      const li = document.createElement("li");
      li.textContent = u;
      return li;
    }),
  );
  usual.hidden = !offer.morning;
  buttons(offer, !!offer.countdown);
  const rows = offer.morning ? Math.min(offer.usual?.length ?? 0, 4) : 0;
  await invoke("show_nudge", { height: ROW + rows * USUAL_ROW }).catch(console.error);
  if (current !== offer) return;
  card.classList.add("shown");
  if (offer.countdown) {
    countDown(offer);
  } else {
    fadeIn(SHOWN_FOR);
  }
}

card.addEventListener("mouseenter", () => {
  if (!current?.countdown) window.clearTimeout(fade);
});
card.addEventListener("mouseleave", () => {
  if (current && !current.countdown) fadeIn(AFTER_LEAVING);
});

$("run").addEventListener("click", () => void answer("run"));
$("always").addEventListener("click", () => void answer("always"));
$("later").addEventListener("click", () => void answer("later"));
$("skip").addEventListener("click", () => void answer("skip"));
$("cancel").addEventListener("click", () => void answer("cancel"));
$("ok").addEventListener("click", () => void answer("ok"));

listen<{ params?: Offer }>("engine://offer", (e) => {
  const offer = e.payload?.params;
  if (!offer?.key) return;
  queue.push(offer);
  void next();
});

listen<string>("vans://updated", (e) => {
  queue.push({ key: "update", line: `VANS ${e.payload} is ready`, update: true });
  void next();
});
