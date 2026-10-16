const CLARITY_ID = "yyjihcwwah";
const KEY = "vans-consent";

type Choice = "yes" | "no";

function saved(): Choice | null {
  try {
    const v = localStorage.getItem(KEY);
    return v === "yes" || v === "no" ? v : null;
  } catch {
    return null;
  }
}

function keep(choice: Choice) {
  try {
    localStorage.setItem(KEY, choice);
  } catch {}
}

let loaded = false;

function loadClarity() {
  if (loaded || !CLARITY_ID) return;
  loaded = true;
  const w = window as unknown as { clarity?: { q?: unknown[] } & ((...a: unknown[]) => void) };
  if (!w.clarity) {
    const queue: unknown[] = [];
    const stub = ((...a: unknown[]) => {
      queue.push(a);
    }) as { q?: unknown[] } & ((...a: unknown[]) => void);
    stub.q = queue;
    w.clarity = stub;
  }
  w.clarity("consentv2", { ad_Storage: "denied", analytics_Storage: "granted" });
  const tag = document.createElement("script");
  tag.async = true;
  tag.src = "https://www.clarity.ms/tag/" + encodeURIComponent(CLARITY_ID);
  document.head.appendChild(tag);
}

function banner(): HTMLElement {
  const box = document.createElement("div");
  box.className = "consent";
  box.setAttribute("role", "dialog");
  box.setAttribute("aria-label", "Cookies");

  const text = document.createElement("p");
  text.className = "consent-text";
  text.append(
    "May this site use Microsoft Clarity to see how visitors read it, such as what is scrolled past and where people stop? It sets cookies. Nothing loads until you say yes. ",
  );
  const more = document.createElement("a");
  more.href = "/privacy";
  more.className = "text-link";
  more.textContent = "Privacy";
  text.append(more);

  const actions = document.createElement("div");
  actions.className = "consent-actions";
  const no = document.createElement("button");
  no.type = "button";
  no.className = "btn";
  no.textContent = "No thanks";
  const yes = document.createElement("button");
  yes.type = "button";
  yes.className = "btn consent-yes";
  yes.textContent = "Allow";
  actions.append(no, yes);

  box.append(text, actions);

  const close = (choice: Choice) => {
    keep(choice);
    box.remove();
    if (choice === "yes") loadClarity();
  };
  no.addEventListener("click", () => close("no"));
  yes.addEventListener("click", () => close("yes"));
  return box;
}

function ask() {
  if (document.querySelector(".consent")) return;
  document.body.appendChild(banner());
}

function addChoiceLink() {
  const copy = document.querySelector(".footer-copy");
  if (!copy) return;
  const link = document.createElement("button");
  link.type = "button";
  link.className = "consent-open";
  link.textContent = "Cookie choice";
  link.addEventListener("click", ask);
  copy.append(" · ", link);
}

export function initConsent() {
  if (!CLARITY_ID) return;
  addChoiceLink();
  const choice = saved();
  if (choice === "yes") loadClarity();
  else if (choice === null) ask();
}
