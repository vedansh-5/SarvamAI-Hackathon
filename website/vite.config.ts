import { defineConfig } from "vite";
import { resolve } from "node:path";
import { buildPages } from "./scripts/build-privacy.js";

function privacyMarkdownPlugin() {
  return {
    name: "privacy-markdown-plugin",
    buildStart() {
      buildPages();
    },
    handleHotUpdate({ file }: { file: string }) {
      if (file.includes("/content/") && file.endsWith(".md")) {
        buildPages();
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
        install: resolve(__dirname, "install/index.html"),
      },
    },
  },
});
