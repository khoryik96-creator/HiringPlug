// Browser smoke tests: the built app, served with production's security
// headers (vite preview), against a running API with the demo company from
// `hp dev-setup`. CI runs them in the end-to-end job; locally:
//   pnpm build && HP_E2E_TOKENS_FILE=path/to/devsetup.txt pnpm e2e
import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "e2e",
  timeout: 30_000,
  retries: 0,
  reporter: [["list"]],
  use: { baseURL: "http://localhost:4173", trace: "off" },
  webServer: {
    command: "pnpm exec vite preview --port 4173 --strictPort",
    url: "http://localhost:4173",
    reuseExistingServer: true,
    timeout: 60_000,
  },
  projects: [{
    name: "chromium",
    use: {
      browserName: "chromium",
      // A pre-installed Chromium (e.g. a sandbox) when Playwright's own is not downloaded.
      ...(process.env.HP_E2E_CHROMIUM ? { launchOptions: { executablePath: process.env.HP_E2E_CHROMIUM } } : {}),
    },
  }],
});
