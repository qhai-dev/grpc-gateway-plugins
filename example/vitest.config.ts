import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";

export default defineConfig({
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  test: {
    // The tests are about the wire format and the types, so they need no DOM.
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
