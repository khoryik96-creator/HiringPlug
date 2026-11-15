// Folders on the candidate list, in a real browser: make one, drag a row onto
// it, open it, put chosen rows in a new folder from the menu, share, rename
// and delete.
import { readFileSync } from "node:fs";
import { expect, type APIRequestContext, test } from "@playwright/test";

import type { Schemas } from "../src/api/client";

const lines = readFileSync(process.env.HP_E2E_TOKENS_FILE ?? "", "utf8").split("\n");
const token = lines[lines.findIndex((l) => l.startsWith("owner ")) + 1]?.trim();
if (!token) throw new Error("HP_E2E_TOKENS_FILE needs the demo owner's token");
const headers = { Authorization: `Bearer ${token}` };

async function candidate(request: APIRequestContext, firstName: string, lastName: string) {
  const res = await request.post("/v1/candidates", { headers, data: { firstName, lastName } });
  expect(res.ok(), await res.text()).toBe(true);
  return (await res.json()) as Schemas["Candidate"];
}

test("keeps candidates in folders: drag, add, share, rename and delete", async ({ page, request }) => {
  const stamp = String(Date.now());
  await candidate(request, "Folder", `Alpha ${stamp}`);
  await candidate(request, "Folder", `Beta ${stamp}`);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.addInitScript((t) => { if (window === window.top) sessionStorage.setItem("hiringplug.token", t); }, token);
  await page.goto("/#/candidates");
  await page.getByLabel("Search candidates").fill(stamp);
  const table = page.locator("table.candidates-table");
  await expect(table.getByRole("link", { name: `Folder Alpha ${stamp}` })).toBeVisible();

  const folders = page.getByRole("navigation", { name: "Folders" });
  await folders.getByRole("button", { name: "+ New folder" }).click();
  await folders.getByLabel("New folder name").fill(`Shortlist ${stamp}`);
  await folders.getByRole("button", { name: "Create" }).click();
  await expect(page.getByRole("heading", { name: `Shortlist ${stamp}` })).toBeVisible(); // it opens
  await folders.getByRole("button", { name: "All candidates" }).click();

  // Drag Alpha's row onto the folder.
  const shortlist = folders.getByRole("button", { name: new RegExp(`Shortlist ${stamp}`) });
  await table.getByRole("link", { name: `Folder Alpha ${stamp}` }).dragTo(shortlist);
  await expect(folders.getByRole("status")).toContainText(`1 candidate put in Shortlist ${stamp}`);
  await expect(shortlist).toContainText("1");

  // Choose Beta and put them in a new folder from the menu.
  await table.getByRole("row", { name: new RegExp(`Folder Beta ${stamp}`) }).getByRole("cell").nth(1).click();
  await page.getByRole("button", { name: "Add to folder ▾" }).click();
  await page.getByLabel("Find or name a new folder").fill(`Call back ${stamp}`);
  await page.getByRole("button", { name: `+ New folder “Call back ${stamp}”` }).click();
  await expect(page.getByRole("checkbox", { name: new RegExp(`Call back ${stamp}`) })).toBeChecked();
  await page.getByRole("button", { name: "Done" }).click();

  // The folder shows only what is in it.
  await shortlist.click();
  await expect(table.getByRole("link", { name: `Folder Alpha ${stamp}` })).toBeVisible();
  await expect(table.getByRole("link", { name: `Folder Beta ${stamp}` })).toHaveCount(0);

  // Share with a colleague, then take them off again.
  await page.getByRole("button", { name: "Share", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: `Share Shortlist ${stamp}` });
  await dialog.getByLabel("Someone to share with").selectOption({ label: "Demo recruiter" });
  await dialog.getByRole("button", { name: "Share" }).click();
  await expect(dialog.getByRole("button", { name: "Remove Demo recruiter" })).toBeVisible();
  await dialog.getByRole("button", { name: "Remove Demo recruiter" }).click();
  await expect(dialog.getByRole("button", { name: "Remove Demo recruiter" })).toHaveCount(0);
  await dialog.getByRole("button", { name: "Done" }).click();

  // Rename, then delete: the candidates stay.
  await page.getByRole("button", { name: "Rename" }).click();
  await page.getByLabel("Folder name").fill(`Final ${stamp}`);
  await page.getByRole("button", { name: "Save" }).click();
  await expect(page.getByRole("heading", { name: `Final ${stamp}` })).toBeVisible();
  page.once("dialog", (d) => { void d.accept(); });
  await page.getByRole("button", { name: "Delete folder" }).click();
  await expect(folders.getByRole("button", { name: new RegExp(`Final ${stamp}`) })).toHaveCount(0);
  await expect(table.getByRole("link", { name: `Folder Alpha ${stamp}` })).toBeVisible();
  expect(errors).toEqual([]);
});
