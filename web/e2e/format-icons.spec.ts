// The message editor's icon toolbar in the new-meeting window, in a real
// browser: every tool's icon sits in the middle of its round button (text
// padding once pushed each icon off to the right).
import { readFileSync } from "node:fs";
import { expect, test } from "@playwright/test";

const lines = readFileSync(process.env.HP_E2E_TOKENS_FILE ?? "", "utf8").split("\n");
const token = lines[lines.findIndex((l) => l.startsWith("owner ")) + 1]?.trim();
if (!token) throw new Error("HP_E2E_TOKENS_FILE needs the demo owner's token");

test("centres each formatting icon in its button", async ({ page }) => {
  await page.addInitScript((t) => { if (window === window.top) sessionStorage.setItem("hiringplug.token", t); }, token);
  await page.goto("/#/calendar");
  await page.getByRole("button", { name: "New appointment", exact: true }).click();
  const toolbar = page.getByRole("toolbar", { name: "Formatting" });
  await toolbar.getByRole("button", { name: "More formatting" }).click();
  await expect(toolbar.getByRole("button", { name: "Clear format" })).toBeVisible();
  const offsets = await toolbar.locator("button.format-tool").evaluateAll((buttons) => buttons.map((b) => {
    const box = b.getBoundingClientRect();
    const icon = (b.querySelector("svg") ?? b.firstElementChild ?? b).getBoundingClientRect();
    return {
      tool: b.getAttribute("aria-label"),
      dx: Math.round(Math.abs(icon.left + icon.width / 2 - (box.left + box.width / 2))),
      dy: Math.round(Math.abs(icon.top + icon.height / 2 - (box.top + box.height / 2))),
    };
  }));
  expect(offsets).toHaveLength(14);
  for (const o of offsets) expect(o, `${o.tool ?? ""} off centre`).toEqual({ tool: o.tool, dx: 0, dy: 0 });
});
