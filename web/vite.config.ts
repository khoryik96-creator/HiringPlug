/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

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
  test: {
    environment: "jsdom",
  },
});
