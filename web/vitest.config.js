import { defineConfig } from "vitest/config";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// vitest uses the svelte plugin so `.svelte.js` rune files compile in tests.
export default defineConfig({
  plugins: [svelte()],
  test: {
    environment: "jsdom",
    globals: false,
  },
});
