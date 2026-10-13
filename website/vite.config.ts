import { defineConfig } from "vite";
import { resolve } from "node:path";
import { buildPrivacyPage } from "./scripts/build-privacy.js";

function privacyMarkdownPlugin() {
  return {
    name: "privacy-markdown-plugin",
    buildStart() {
      buildPrivacyPage();
    },
    handleHotUpdate({ file }: { file: string }) {
      if (file.endsWith("privacy.md")) {
        buildPrivacyPage();
      }
    },
  };
}

export default defineConfig({
  plugins: [privacyMarkdownPlugin()],
  build: {
    sourcemap: false,
    outDir: "dist",
    rollupOptions: {
      input: {
        main: resolve(__dirname, "index.html"),
        privacy: resolve(__dirname, "privacy/index.html"),
      },
    },
  },
});
