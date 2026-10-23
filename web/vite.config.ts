import { writeFileSync } from "node:fs";
import path from "node:path";
import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";

const outDir = path.resolve(import.meta.dirname, "../internal/server/webui/dist");

// go:embed requires the dist directory to exist in a fresh checkout, so the
// tracked placeholder is restored after Vite empties the directory.
function keepGitkeep(): Plugin {
  return {
    name: "keep-gitkeep",
    closeBundle() {
      writeFileSync(path.join(outDir, ".gitkeep"), "");
    },
  };
}

// In development the admin listener serves both the public and admin APIs.
const backend = process.env.FENGHUO_DEV_BACKEND ?? "http://127.0.0.1:8081";

export default defineConfig({
  plugins: [react(), tailwindcss(), keepGitkeep()],
  resolve: {
    alias: { "@": path.resolve(import.meta.dirname, "src") },
  },
  build: {
    outDir,
    emptyOutDir: true,
    chunkSizeWarningLimit: 800,
  },
  server: {
    proxy: {
      "/api": backend,
      "/admin/api": backend,
    },
  },
});
