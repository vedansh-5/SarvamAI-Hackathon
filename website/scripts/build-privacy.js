import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const rootDir = path.resolve(__dirname, "..");
const markdownPath = path.resolve(rootDir, "content/privacy.md");
const outputDir = path.resolve(rootDir, "privacy");
const outputPath = path.resolve(outputDir, "index.html");

function escapeHtml(str) {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function inlineFormat(text) {
  // code blocks `code`
  let out = text.replace(/`([^`]+)`/g, (_m, code) => `<code>${escapeHtml(code)}</code>`);
  // bold **text**
  out = out.replace(/\*\*([^*]+)\*\*/g, (_m, bold) => `<strong>${inlineFormat(bold)}</strong>`);
  // email
  out = out.replace(/([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/g, (_m, email) => {
    return `<a href="mailto:${email}" class="text-link">${email}</a>`;
  });
  return out;
}

export function markdownToHtml(md) {
  const lines = md.split(/\r?\n/);
  const html = [];
  let inList = false;

  for (let i = 0; i < lines.length; i++) {
    const rawLine = lines[i];
    const trimmed = rawLine.trim();

    if (!trimmed) {
      if (inList) {
        html.push("</ul>");
        inList = false;
      }
      continue;
    }

    if (trimmed.startsWith("### ")) {
      if (inList) {
        html.push("</ul>");
        inList = false;
      }
      const title = inlineFormat(trimmed.slice(4));
      html.push(`<h3>${title}</h3>`);
      continue;
    }

    if (trimmed.startsWith("## ")) {
      if (inList) {
        html.push("</ul>");
        inList = false;
      }
      const title = inlineFormat(trimmed.slice(3));
      html.push(`<h2>${title}</h2>`);
      continue;
    }

    if (trimmed.startsWith("# ")) {
      if (inList) {
        html.push("</ul>");
        inList = false;
      }
      const title = inlineFormat(trimmed.slice(2));
      html.push(`<h1>${title}</h1>`);
      continue;
    }

    if (trimmed.startsWith("- ")) {
      if (!inList) {
        html.push("<ul>");
        inList = true;
      }
      const itemText = inlineFormat(trimmed.slice(2));
      html.push(`<li>${itemText}</li>`);
      continue;
    }

    // Regular paragraph line
    if (inList) {
      html.push("</ul>");
      inList = false;
    }

    // Collect multi-line paragraphs if followed by text
    let para = trimmed;
    while (i + 1 < lines.length && lines[i + 1].trim() && !lines[i + 1].trim().startsWith("#") && !lines[i + 1].trim().startsWith("- ")) {
      i++;
      para += " " + lines[i].trim();
    }
    html.push(`<p>${inlineFormat(para)}</p>`);
  }

  if (inList) {
    html.push("</ul>");
  }

  return html.join("\n");
}

export function buildPrivacyPage() {
  if (!fs.existsSync(markdownPath)) {
    console.error(`Privacy markdown not found at ${markdownPath}`);
    return;
  }

  const markdown = fs.readFileSync(markdownPath, "utf-8");
  const articleHtml = markdownToHtml(markdown);

  if (!fs.existsSync(outputDir)) {
    fs.mkdirSync(outputDir, { recursive: true });
  }

  const pageHtml = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>Privacy — VANS</title>
    <meta name="description" content="Privacy policy for VANS. Plain about what leaves your Mac, what is kept, and what VANS never does." />
    <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
    <meta property="og:title" content="Privacy — VANS" />
    <meta property="og:description" content="Privacy policy for VANS. Plain about what leaves your Mac, what is kept, and what VANS never does." />
    <meta property="og:image" content="/og-image.svg" />
    <meta property="og:type" content="website" />
    <meta name="twitter:card" content="summary_large_image" />
    <link rel="stylesheet" href="/src/styles.css" />
    <script>
      (function() {
        try {
          var t = localStorage.getItem("vans-theme") || "light";
          document.documentElement.setAttribute("data-theme", t);
        } catch (e) {
          document.documentElement.setAttribute("data-theme", "light");
        }
      })();
    </script>
  </head>
  <body class="privacy-body">
    <header class="site-header">
      <div class="site-header-inner">
        <a href="/" class="header-brand" aria-label="VANS home">VANS</a>
        <div class="header-right">
          <a href="/" class="text-link header-nav-link">Home</a>
          <button type="button" id="header-theme-toggle" class="theme-toggle btn-ghost" aria-label="Toggle light or dark theme">
            <span class="theme-icon" aria-hidden="true"></span>
            <span class="theme-label">Dark mode</span>
          </button>
        </div>
      </div>
    </header>

    <main class="privacy-container">
      <article class="privacy-article">
        ${articleHtml}
      </article>
    </main>

    <footer class="site-footer">
      <div class="site-footer-inner">
        <div class="footer-top">
          <div class="footer-brand">
            <span class="brand-wordmark">VANS</span>
          </div>
          <div class="footer-controls">
            <button type="button" id="footer-theme-toggle" class="theme-toggle btn-ghost" aria-label="Toggle light or dark theme">
              <span class="theme-icon" aria-hidden="true"></span>
              <span class="theme-label">Dark mode</span>
            </button>
          </div>
        </div>
        <div class="footer-bottom">
          <div class="footer-copy">© 2026 Vedansh Saini · <a href="/privacy" class="text-link">Privacy</a></div>
          <div class="footer-disclaimer">VANS is not affiliated with any clothing or footwear brand.</div>
        </div>
      </div>
    </footer>

    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
`;

  fs.writeFileSync(outputPath, pageHtml, "utf-8");
  console.log(`Privacy page built successfully at ${outputPath}`);
}

// Run directly if invoked
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  buildPrivacyPage();
}
