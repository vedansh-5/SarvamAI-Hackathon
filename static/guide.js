(() => {
  const box = document.getElementById("guide");
  const sayEl = document.getElementById("guide-say");
  const list = document.getElementById("guide-steps");
  const history = document.getElementById("history");
  const island = () => window.appIsland;

  const memory = document.getElementById("memory");
  const memoryText = document.getElementById("memory-text");
  const hint = document.getElementById("command-hint");
  const MAX_TURNS = 4;

  let WORDS = {};
  let turns = [];

  let current = null;
  let at = 0;
  let player = null;

  const stepText = (s) => (typeof s === "string" ? s : (s && s.text) || "");
  const lines = (g) => (Array.isArray(g.steps_text) ? g.steps_text : (g.steps || []).map(stepText)).filter(Boolean);

  function command(text) {
    const t = text.toLowerCase().replace(/[.!?।,]/g, "").trim();
    if (t.split(/\s+/).length > 4) return null;
    for (const [name, words] of Object.entries(WORDS)) if (words.some((w) => ` ${t} `.includes(` ${w} `))) return name;
    return null;
  }

  async function loadCommands() {
    try {
      WORDS = await (await fetch("/api/commands")).json();
      const pick = (name) => (WORDS[name] || []).filter((w) => !/^[a-z ]+$/.test(w)).slice(0, 2).join(", ");
      hint.textContent = `During a guide, say “next”, “back”, “again” or “stop” (${["next", "back", "again", "stop"].map(pick).filter(Boolean).join("; ")}).`;
    } catch {
      hint.textContent = "During a guide, use the buttons above.";
    }
  }

  function remember(g) {
    turns = [...turns, { question: g.question, say: g.say, steps: g.steps }].slice(-MAX_TURNS);
    showMemory();
  }

  function forget() {
    turns = [];
    showMemory();
  }

  function showMemory() {
    memory.hidden = !turns.length;
    memoryText.textContent = `Follow-up questions remember your last ${turns.length === 1 ? "answer" : `${turns.length} answers`}.`;
  }

  function hush() {
    if (player) {
      player.pause();
      player = null;
    }
  }

  async function speak(text, language) {
    hush();
    if (!text) return;
    island().show("speaking", text);
    try {
      const res = await fetch("/api/speak", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, language }),
      });
      const out = await res.json();
      if (!res.ok) throw new Error(out.error);
      player = new Audio(`data:${out.mime};base64,${out.audio}`);
      player.onended = () => island().hide(900);
      await player.play();
    } catch {
      island().hide(2500);
    }
  }

  function render() {
    list.replaceChildren(
      ...current.lines.map((s, i) => {
        const li = document.createElement("li");
        li.textContent = s;
        if (i < at) li.className = "done";
        if (i === at) li.className = "now";
        return li;
      }),
    );
  }

  function go(to) {
    if (!current || !current.lines.length) return;
    if (to >= current.lines.length) {
      at = current.lines.length;
      render();
      speak(current.language.startsWith("hi") ? "हो गया!" : "All done!", current.language);
      return;
    }
    at = Math.max(0, to);
    render();
    speak(current.lines[at], current.language);
  }

  function stop() {
    hush();
    current = null;
    box.hidden = true;
    island().hide();
  }

  function showGuide(g) {
    current = { ...g, lines: lines(g) };
    at = 0;
    box.hidden = false;
    sayEl.textContent = g.say;
    render();
    const first = current.lines.length ? `${g.say} ${current.lines[0]}` : g.say;
    speak(first, g.language);
  }

  async function ask(text, language) {
    if (current && current.lines.length) {
      const c = command(text);
      if (c === "next") return go(at + 1);
      if (c === "back") return go(at - 1);
      if (c === "again") return go(at);
      if (c === "stop") return stop();
    }
    island().show("thinking", text);
    try {
      const res = await fetch("/api/ask", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text, language, turns }),
      });
      const out = await res.json();
      if (!res.ok) throw new Error(out.error);
      remember(out);
      showGuide(out);
      loadHistory();
    } catch (e) {
      island().show("note", String(e.message || e));
      island().hide(4000);
    }
  }

  async function loadHistory() {
    try {
      const rows = await (await fetch("/api/history")).json();
      history.replaceChildren(
        ...rows.map((r) => {
          const li = document.createElement("li");
          li.textContent = r.question;
          const lang = document.createElement("small");
          lang.textContent = r.language;
          li.append(lang);
          li.onclick = () => showGuide(r);
          return li;
        }),
      );
    } catch {}
  }

  window.addEventListener("app:heard", (e) => ask(e.detail.text, e.detail.language));
  window.addEventListener("app:hush", hush);
  document.getElementById("type-form").addEventListener("submit", (e) => {
    e.preventDefault();
    const input = document.getElementById("type-box");
    if (input.value.trim()) ask(input.value.trim());
    input.value = "";
    input.blur();
  });
  document.getElementById("g-next").onclick = () => go(at + 1);
  document.getElementById("g-back").onclick = () => go(at - 1);
  document.getElementById("g-again").onclick = () => go(at);
  document.getElementById("g-stop").onclick = stop;
  document.getElementById("fresh").onclick = forget;
  loadCommands();
  loadHistory();
})();
