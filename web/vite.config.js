import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// base:'./' makes built asset URLs relative so go:embed + Echo static serving
// works under "/" with no configured prefix (J arch §6/§7).
export default defineConfig({
  plugins: [svelte()],
  base: "./",
  build: {
    outDir: "dist",
    emptyOutDir: true,
    assetsDir: "assets",
    target: "es2022",
  },
  server: {
    proxy: {
      // dev: forward /api → a locally running node so `npm run dev` is live.
      "/api": { target: "http://localhost:8080", ws: true },
    },
  },
  test: {
    environment: "jsdom",
  },
});
