import { readFileSync } from "node:fs";

import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { configDefaults } from "vitest/config";

import { companionZip } from "./companionZip.ts";
import { securityHeaders } from "./securityHeaders.ts";

// In development the API runs on :8080 (`task dev`); Vite proxies to it so
// the browser sees one origin. HP_DEV_API points it elsewhere (two copies of
// the app side by side, e.g. http://localhost:8081).
const api = process.env.HP_DEV_API ?? "http://localhost:8080";

export default defineConfig({
  // HiringPlug's version (the VERSION file, raised with every update), shown in the top bar.
  define: { __APP_VERSION__: JSON.stringify(readFileSync(new URL("../VERSION", import.meta.url), "utf8").trim()) },
  // The LinkedIn companion, zipped for its download button (Settings › LinkedIn companion).
  plugins: [react(), companionZip(new URL("../extension/linkedin-companion/", import.meta.url).pathname)],
  server: {
    port: 5173,
    proxy: {
      "/v1": api,
      "/openapi.json": api,
    },
  },
  // `vite preview` serves the built app as nginx does: same proxy, same security headers.
  preview: {
    port: 4173,
    headers: securityHeaders(new URL("../deploy/security-headers.conf", import.meta.url).pathname),
    proxy: {
      "/v1": api,
      "/openapi.json": api,
    },
  },
  test: {
    environment: "jsdom",
    exclude: [...configDefaults.exclude, "e2e/**"], // browser tests: pnpm e2e
  },
});
