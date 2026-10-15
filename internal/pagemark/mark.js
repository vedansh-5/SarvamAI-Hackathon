(function (P) {
  try {
    if (!window.CSS || !CSS.highlights || typeof Highlight === "undefined") {
      return JSON.stringify({ ok: false, error: "this browser cannot mark text on a page; it needs Chrome 105 or Safari 17.2 or later" });
    }
    if (window.__vansMarks) {
      try { window.__vansMarks.off(); } catch (e) {}
    }

    const skip = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "TEXTAREA", "INPUT", "SELECT", "OPTION", "TEMPLATE", "CANVAS", "HEAD", "TITLE"]);
    const cut = /[^\s,;:|()\[\]{}<>"'“”‘’]+/g;
    const want = P.text.toLowerCase();
    const phrase = /\s/.test(want);

    function shapeOf(token) {
      return token.replace(/\p{L}+/gu, "a").replace(/\p{N}/gu, "9");
    }

    function matches(token) {
      const t = token.toLowerCase();
      if (P.how === "start") return t.startsWith(want);
      if (P.how === "end") return t.endsWith(want);
      if (P.how === "equal") return t === want;
      return t.includes(want);
    }

    function blockOf(el, root) {
      while (el && el !== root) {
        const d = getComputedStyle(el).display;
        if (d && d !== "inline" && d !== "contents") return el;
        el = el.parentElement;
      }
      return root;
    }

    const docs = [];
    function collect(doc, root, list) {
      const pieces = [];
      let flat = "";
      let block = null;
      const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT | NodeFilter.SHOW_ELEMENT, {
        acceptNode(n) {
          if (n.nodeType === 1) {
            if (skip.has(n.tagName)) return NodeFilter.FILTER_REJECT;
            if (n.shadowRoot) list.push(n.shadowRoot);
            if (n.tagName === "IFRAME") {
              try { if (n.contentDocument && n.contentDocument.body) docs.push(n.contentDocument); } catch (e) {}
            }
            return NodeFilter.FILTER_SKIP;
          }
          return n.data.trim() ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP;
        },
      });
      let n;
      while ((n = walker.nextNode())) {
        const b = blockOf(n.parentElement, root);
        if (b !== block) {
          flat += "\n";
          block = b;
        }
        pieces.push({ node: n, at: flat.length });
        flat += n.data;
      }
      return { pieces, flat };
    }

    function rangeOf(doc, pieces, from, to) {
      function find(at) {
        let lo = 0, hi = pieces.length - 1;
        while (lo < hi) {
          const mid = (lo + hi + 1) >> 1;
          if (pieces[mid].at <= at) lo = mid; else hi = mid - 1;
        }
        return pieces[lo];
      }
      const a = find(from), b = find(to - 1);
      const r = doc.createRange();
      r.setStart(a.node, from - a.at);
      r.setEnd(b.node, to - b.at);
      return r;
    }

    function shows(r) {
      const rects = r.getClientRects();
      return rects.length > 0 && Array.from(rects).some((x) => x.width > 0 && x.height > 0);
    }

    const sheetText = "::highlight(" + P.name + "){background-color:" + P.soft + ";color:" + P.ink + ";}";
    const sheets = [];
    function adopt(holder, win) {
      const sheet = new win.CSSStyleSheet();
      sheet.replaceSync(sheetText);
      holder.adoptedStyleSheets = [...holder.adoptedStyleSheets, sheet];
      sheets.push({ holder, sheet });
    }

    const found = [];
    const shapes = {};
    const registries = [];
    docs.push(document);
    for (let d = 0; d < docs.length; d++) {
      const doc = docs[d];
      const win = doc.defaultView;
      if (!win || !win.CSS || !win.CSS.highlights) continue;
      const roots = [doc.body];
      const ranges = [];
      for (let i = 0; i < roots.length; i++) {
        const root = roots[i];
        const { pieces, flat } = collect(doc, root, roots);
        if (!pieces.length) continue;
        let hits = 0;
        if (phrase) {
          const lower = flat.toLowerCase();
          let at = lower.indexOf(want);
          while (at >= 0) {
            const r = rangeOf(doc, pieces, at, at + want.length);
            if (shows(r)) { ranges.push(r); found.push(r); hits++; }
            at = lower.indexOf(want, at + want.length);
          }
        } else {
          let m;
          cut.lastIndex = 0;
          while ((m = cut.exec(flat))) {
            let token = m[0].replace(/[.!?]+$/, "");
            if (!token) continue;
            const shape = shapeOf(token);
            if (matches(token)) {
              const r = rangeOf(doc, pieces, m.index, m.index + token.length);
              if (!shows(r)) continue;
              ranges.push(r);
              found.push(r);
              hits++;
              shapes[shape] = (shapes[shape] || 0) + 1;
            }
          }
        }
        if (hits && root !== doc.body) adopt(root, win);
      }
      if (ranges.length) {
        win.CSS.highlights.set(P.name, new win.Highlight(...ranges));
        adopt(doc, win);
        registries.push(win.CSS.highlights);
      }
    }

    if (!phrase && (P.how === "start" || P.how === "end")) {
      const longer = found.filter((r) => r.toString().toLowerCase() !== want);
      if (longer.length && longer.length < found.length) {
        const exact = found.filter((r) => r.toString().toLowerCase() === want);
        for (const r of exact) {
          for (const reg of registries) {
            for (const [, h] of reg) { try { h.delete(r); } catch (e) {} }
          }
        }
        const shape = shapeOf(want);
        if (shapes[shape]) shapes[shape] -= exact.length;
        found.splice(0, found.length, ...longer);
      }
    }

    let alike = 0;
    const common = Object.keys(shapes).sort((a, b) => shapes[b] - shapes[a])[0];
    if (common && !phrase) {
      for (let d = 0; d < docs.length; d++) {
        try {
          const flat = docs[d].body.innerText || "";
          let m;
          cut.lastIndex = 0;
          while ((m = cut.exec(flat))) {
            if (shapeOf(m[0].replace(/[.!?]+$/, "")) === common) alike++;
          }
        } catch (e) {}
      }
    }

    function off() {
      for (const reg of registries) { try { reg.delete(P.name); } catch (e) {} }
      for (const s of sheets) {
        try { s.holder.adoptedStyleSheets = s.holder.adoptedStyleSheets.filter((x) => x !== s.sheet); } catch (e) {}
      }
      document.removeEventListener("keydown", onKey, true);
      if (window.__vansMarks && window.__vansMarks.off === off) delete window.__vansMarks;
    }
    function onKey(e) {
      if (e.key === "Escape") off();
    }

    let scrolled = false;
    if (found.length) {
      document.addEventListener("keydown", onKey, true);
      window.__vansMarks = { off };
      const inView = found.some((r) => {
        const x = r.getBoundingClientRect();
        return x.bottom > 0 && x.top < innerHeight && x.right > 0 && x.left < innerWidth;
      });
      if (!inView) {
        const el = found[0].startContainer.parentElement;
        if (el) { el.scrollIntoView({ block: "center", behavior: "smooth" }); scrolled = true; }
      }
    }

    const painted = registries.every((reg) => reg.has(P.name)) && sheets.length > 0;
    return JSON.stringify({ ok: true, matched: found.length, alike: Math.max(alike, found.length), scrolled, painted: found.length === 0 || painted });
  } catch (e) {
    return JSON.stringify({ ok: false, error: String((e && e.message) || e) });
  }
})
