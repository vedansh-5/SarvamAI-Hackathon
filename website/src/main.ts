import { livingPill } from "./pill.ts";
import { initGuideDemo } from "./guide-demo.ts";
import { initLivePanel } from "./panel-demo.ts";
import { initConfirmDemo } from "./confirm-demo.ts";
import { initHeroDotBackground } from "./bg-dots.ts";

function initTheme() {
  const toggles = document.querySelectorAll<HTMLElement>(".theme-toggle");
  if (!toggles.length) return;

  const getSaved = (): "light" | "dark" => {
    try {
      const v = localStorage.getItem("vans-theme");
      if (v === "light" || v === "dark") return v;
    } catch {}
    return "light"; // First visit is light even if system is dark
  };

  const apply = (theme: "light" | "dark") => {
    document.documentElement.setAttribute("data-theme", theme);
    toggles.forEach((t) => {
      const label = t.querySelector(".theme-label");
      if (label) {
        label.textContent = theme === "dark" ? "Light mode" : "Dark mode";
      }
    });
    try {
      localStorage.setItem("vans-theme", theme);
    } catch {}
  };

  apply(getSaved());

  toggles.forEach((t) => {
    t.addEventListener("click", () => {
      const cur = document.documentElement.getAttribute("data-theme") || "light";
      apply(cur === "dark" ? "light" : "dark");
    });
  });
}

function initPills() {
  const pills = document.querySelectorAll<HTMLElement>(".vans-pill");
  pills.forEach((p) => livingPill(p));
}

function initKeycapFeedback() {
  const keyMap: Record<string, string> = {
    Alt: "⌥",
    " ": "Space",
    Escape: "esc",
    Enter: "↵",
    Meta: "⌘",
    Shift: "⇧",
  };

  window.addEventListener("keydown", (e) => {
    const symbol = keyMap[e.key];
    if (!symbol) return;
    document.querySelectorAll("kbd").forEach((kbd) => {
      if (kbd.textContent?.trim() === symbol) {
        kbd.classList.add("pressed");
      }
    });
  });

  window.addEventListener("keyup", (e) => {
    const symbol = keyMap[e.key];
    if (!symbol) return;
    document.querySelectorAll("kbd").forEach((kbd) => {
      if (kbd.textContent?.trim() === symbol) {
        kbd.classList.remove("pressed");
      }
    });
  });
}

function initScrollFade() {
  if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  const sections = document.querySelectorAll<HTMLElement>("section");
  const observer = new IntersectionObserver(
    (entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          entry.target.classList.remove("fade-in-init");
          entry.target.classList.add("fade-in-done");
          observer.unobserve(entry.target);
        }
      });
    },
    { threshold: 0.08 },
  );

  sections.forEach((sec) => {
    sec.classList.add("fade-in-init");
    observer.observe(sec);
  });
}

function initRequestCardClicks() {
  const cards = document.querySelectorAll<HTMLElement>(".request-card");
  cards.forEach((card) => {
    card.addEventListener("click", () => {
      const category = card.getAttribute("data-category");
      const requestText = card.getAttribute("data-request");

      if (category === "teach") {
        const hero = document.getElementById("hero");
        if (hero) hero.scrollIntoView({ behavior: "smooth" });
      } else {
        const twoWays = document.getElementById("two-ways");
        if (twoWays) twoWays.scrollIntoView({ behavior: "smooth" });

        if (requestText) {
          const askInput = document.getElementById("panel-ask-input") as HTMLInputElement;
          if (askInput) {
            askInput.value = requestText;
            askInput.dispatchEvent(new Event("input", { bubbles: true }));
            const enterEvt = new KeyboardEvent("keydown", { key: "Enter", bubbles: true });
            askInput.dispatchEvent(enterEvt);
          }
        }
      }
    });
  });
}

document.addEventListener("DOMContentLoaded", () => {
  initTheme();
  initPills();
  initKeycapFeedback();
  initScrollFade();
  initHeroDotBackground("bg-canvas");

  const guideRoot = document.getElementById("guide-demo-root");
  if (guideRoot) initGuideDemo(guideRoot);

  const panelRoot = document.getElementById("panel-demo-root");
  if (panelRoot) initLivePanel(panelRoot);

  const confirmRoot = document.getElementById("confirm-demo-root");
  if (confirmRoot) initConfirmDemo(confirmRoot);

  initRequestCardClicks();
});
