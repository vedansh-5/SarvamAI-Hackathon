import { CONTACT_EMAIL } from "./facts.ts";

const FORM_ENDPOINT = "/api/windows";

let openedByKeyboard = false;

function openSheet(id: string, from: HTMLElement, keyboard = false) {
  const sheet = document.getElementById(id) as HTMLDialogElement | null;
  if (!sheet || typeof sheet.showModal !== "function") return false;
  sheet.dataset.openedAt = String(Date.now());
  sheet.showModal();
  sheet.addEventListener(
    "close",
    () => {
      if (keyboard) {
        from.focus();
      } else {
        from.blur();
      }
    },
    { once: true },
  );
  return true;
}

function initOpeners() {
  document.querySelectorAll<HTMLElement>("[data-open]").forEach((opener) => {
    opener.addEventListener("keydown", (e) => {
      if (e.key === "Enter" || e.key === " ") {
        openedByKeyboard = true;
      }
    });
    opener.addEventListener("pointerdown", () => {
      openedByKeyboard = false;
    });
    opener.addEventListener("click", (e) => {
      const isKeyboard = openedByKeyboard;
      openedByKeyboard = false;
      if (openSheet(opener.dataset.open ?? "", opener, isKeyboard)) {
        e.preventDefault();
        if (!isKeyboard) opener.blur();
      }
    });
  });

  document.querySelectorAll<HTMLDialogElement>("dialog.sheet").forEach((sheet) => {
    sheet.querySelectorAll<HTMLElement>("[data-close]").forEach((b) =>
      b.addEventListener("click", () => {
        openedByKeyboard = false;
        sheet.close();
      }),
    );
    sheet.addEventListener("click", (e) => {
      if (e.target === sheet) {
        openedByKeyboard = false;
        sheet.close();
      }
    });
    sheet.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        openedByKeyboard = true;
      }
    });
  });
}

async function copy(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

function initCopy() {
  document.querySelectorAll<HTMLButtonElement>("[data-copy]").forEach((button) => {
    const idle = button.textContent ?? "Copy address";
    let back = 0;
    button.addEventListener("click", async () => {
      const done = await copy(button.dataset.copy ?? "");
      button.textContent = done ? "Copied" : "Select the address and copy it";
      window.clearTimeout(back);
      back = window.setTimeout(() => (button.textContent = idle), 2000);
    });
  });
}

function looksLikeAnEmail(value: string): boolean {
  return value.length <= 254 && /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(value);
}

function initWindowsForm() {
  const form = document.getElementById("windows-form") as HTMLFormElement | null;
  const sheet = document.getElementById("windows-sheet") as HTMLDialogElement | null;
  if (!form || !sheet) return;

  const status = form.querySelector<HTMLElement>(".sheet-status")!;
  const submit = form.querySelector<HTMLButtonElement>(".sheet-submit")!;
  const done = sheet.querySelector<HTMLElement>(".sheet-done")!;

  form.addEventListener("submit", async (e) => {
    e.preventDefault();
    const data = new FormData(form);
    const email = String(data.get("email") ?? "").trim();
    if (!looksLikeAnEmail(email)) {
      status.textContent = "That does not look like an email address.";
      form.querySelector<HTMLInputElement>('input[name="email"]')?.focus();
      return;
    }

    submit.disabled = true;
    status.textContent = "Sending…";
    const waited = Date.now() - Number(sheet.dataset.openedAt ?? Date.now());

    try {
      const res = await fetch(FORM_ENDPOINT, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          email,
          note: String(data.get("note") ?? "").trim(),
          company: String(data.get("company") ?? ""),
          waited: Math.max(waited, 0),
        }),
      });
      if (!res.ok) throw new Error(String(res.status));
      form.hidden = true;
      done.hidden = false;
    } catch {
      status.textContent = `That did not go through. Write to ${CONTACT_EMAIL} instead.`;
      submit.disabled = false;
    }
  });
}

function initWays() {
  const ways = document.getElementById("ways");
  if (!ways) return;
  const tabs = Array.from(ways.querySelectorAll<HTMLButtonElement>(".ways-tab"));
  const texts = Array.from(ways.querySelectorAll<HTMLElement>(".ways-text"));
  const mark = ways.querySelector<HTMLElement>(".ways-mark");

  const choose = (way: string) => {
    tabs.forEach((t) => {
      const on = t.dataset.way === way;
      t.classList.toggle("on", on);
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
      if (on && mark) {
        mark.style.width = `${t.offsetWidth}px`;
        mark.style.transform = `translate(${t.offsetLeft}px, ${t.offsetTop + t.offsetHeight - (mark.parentElement?.clientHeight ?? 0) + 12}px)`;
      }
    });
    texts.forEach((p) => p.classList.toggle("on", p.dataset.way === way));
  };

  ways.classList.add("live");
  tabs.forEach((t, i) => {
    t.addEventListener("click", () => choose(t.dataset.way ?? "show"));
    t.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowRight" && e.key !== "ArrowLeft") return;
      const next = tabs[(i + (e.key === "ArrowRight" ? 1 : tabs.length - 1)) % tabs.length];
      next.focus();
      choose(next.dataset.way ?? "show");
    });
  });

  const current = () => tabs.find((t) => t.classList.contains("on"))?.dataset.way ?? "show";
  choose(current());
  window.addEventListener("resize", () => choose(current()));
  document.fonts?.ready.then(() => choose(current()));
}

export function initSheets() {
  initOpeners();
  initCopy();
  initWindowsForm();
  initWays();
}


