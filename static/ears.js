(() => {
  const island = document.getElementById("island");
  const islandWords = document.getElementById("island-words");
  const talk = document.getElementById("talk");
  const bars = Array.from(document.querySelectorAll(".wave i"));
  const appName = document.body.dataset.appName || "";
  const PROFILE = [0.35, 0.6, 0.85, 1, 0.85, 0.6, 0.35];

  let stream = null;
  let recorder = null;
  let chunks = [];
  let analyser = null;
  let raf = 0;
  let holding = false;
  let closeTimer = 0;

  function show(state, words = "") {
    clearTimeout(closeTimer);
    island.classList.remove("shake");
    island.dataset.state = state;
    islandWords.textContent = words;
    if (state === "note") {
      void island.offsetWidth;
      island.classList.add("shake");
    }
  }

  function hide(after = 0) {
    clearTimeout(closeTimer);
    closeTimer = setTimeout(() => (island.dataset.state = "idle"), after);
  }

  window.appIsland = { show, hide };

  function level() {
    const data = new Uint8Array(analyser.fftSize);
    analyser.getByteTimeDomainData(data);
    let sum = 0;
    for (const v of data) sum += ((v - 128) / 128) ** 2;
    const rms = Math.sqrt(sum / data.length);
    const lvl = Math.min(1, Math.max(0, (20 * Math.log10(Math.max(rms, 1e-6)) + 55) / 45));
    bars.forEach((b, i) => (b.style.transform = `scaleY(${Math.min(1, 0.16 + lvl * PROFILE[i] * 0.9)})`));
    island.style.setProperty("--lvl", lvl.toFixed(3));
    raf = requestAnimationFrame(level);
  }

  async function start() {
    if (holding) return;
    holding = true;
    talk.classList.add("held");
    window.dispatchEvent(new CustomEvent("app:hush"));
    try {
      stream = stream || (await navigator.mediaDevices.getUserMedia({ audio: true }));
    } catch {
      holding = false;
      talk.classList.remove("held");
      show("note", `${appName} can't hear you. Allow the microphone for this page.`);
      hide(4000);
      return;
    }
    if (!holding) return;
    const ctx = new AudioContext();
    analyser = ctx.createAnalyser();
    analyser.fftSize = 1024;
    ctx.createMediaStreamSource(stream).connect(analyser);
    chunks = [];
    recorder = new MediaRecorder(stream, { mimeType: MediaRecorder.isTypeSupported("audio/webm") ? "audio/webm" : "" });
    recorder.ondataavailable = (e) => e.data.size && chunks.push(e.data);
    recorder.onstop = () => {
      cancelAnimationFrame(raf);
      island.style.setProperty("--lvl", "0");
      ctx.close();
      send(new Blob(chunks, { type: recorder.mimeType || "audio/webm" }));
    };
    recorder.start();
    show("listening");
    level();
  }

  function stop() {
    if (!holding) return;
    holding = false;
    talk.classList.remove("held");
    if (recorder && recorder.state === "recording") recorder.stop();
  }

  async function send(blob) {
    if (blob.size < 1200) {
      show("note", "Didn't hear anything.");
      hide(1600);
      return;
    }
    show("thinking", "…");
    const form = new FormData();
    form.append("audio", blob, "speech.webm");
    try {
      const res = await fetch("/api/listen", { method: "POST", body: form });
      const out = await res.json();
      if (!res.ok) throw new Error(out.error || "Couldn't hear that.");
      if (!out.text) {
        show("note", "Didn't catch that.");
        hide(1600);
        return;
      }
      show("thinking", out.text);
      window.dispatchEvent(new CustomEvent("app:heard", { detail: { text: out.text, language: out.language } }));
    } catch (e) {
      show("note", String(e.message || e));
      hide(4000);
    }
  }

  const typing = (e) => e.target.closest && e.target.closest("input, textarea");
  window.addEventListener("keydown", (e) => {
    if (e.code === "Space" && !e.repeat && !typing(e)) {
      e.preventDefault();
      start();
    }
    if (e.code === "Escape") {
      holding = false;
      talk.classList.remove("held");
      if (recorder && recorder.state === "recording") {
        recorder.onstop = () => cancelAnimationFrame(raf);
        recorder.stop();
      }
      window.dispatchEvent(new CustomEvent("app:hush"));
      hide();
    }
  });
  window.addEventListener("keyup", (e) => e.code === "Space" && !typing(e) && stop());
  talk.addEventListener("pointerdown", (e) => {
    talk.setPointerCapture(e.pointerId);
    start();
  });
  talk.addEventListener("pointerup", stop);
  talk.addEventListener("pointercancel", stop);
})();
