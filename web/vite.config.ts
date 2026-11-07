import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { configDefaults } from "vitest/config";

import { securityHeaders } from "./securityHeaders.ts";

// In development the API runs on :8080 (`task dev`); Vite proxies to it so
// the browser sees one origin.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/v1": "http://localhost:8080",
      "/openapi.json": "http://localhost:8080",
    },
  },
  // `vite preview` serves the built app as nginx does: same proxy, same security headers.
  preview: {
    port: 4173,
    headers: securityHeaders(new URL("../deploy/security-headers.conf", import.meta.url).pathname),
    proxy: {
      "/v1": "http://localhost:8080",
      "/openapi.json": "http://localhost:8080",
    },
  },
  test: {
    environment: "jsdom",
    exclude: [...configDefaults.exclude, "e2e/**"], // browser tests: pnpm e2e
  },
});
