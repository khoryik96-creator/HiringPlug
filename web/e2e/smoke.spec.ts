// Every page and every part of Settings opens for each role without breaking
// (no error boundary, no console errors such as a blocked script), and signing
// out ends the session. The tokens come from `hp dev-setup`'s output.
import { readFileSync } from "node:fs";

import { expect, type Page, test } from "@playwright/test";

type Role = "owner" | "recruiter" | "viewer";

function devSetup(): string[] {
  const file = process.env.HP_E2E_TOKENS_FILE;
  if (!file) throw new Error("HP_E2E_TOKENS_FILE must name hp dev-setup's output (it lists each role's email and token)");
  return readFileSync(file, "utf8").split("\n");
}

function tokens(): Record<Role, string> {
  const lines = devSetup();
  const file = process.env.HP_E2E_TOKENS_FILE;
  const token = (role: Role) => {
    const i = lines.findIndex((l) => l.startsWith(role + " "));
    const t = lines[i + 1]?.trim();
    if (i < 0 || !t) throw new Error(`no ${role} token in ${file}`);
    return t;
  };
  return { owner: token("owner"), recruiter: token("recruiter"), viewer: token("viewer") };
}

const TOKENS = tokens();

/** Opens the app signed in as role (or with token), watching for console errors. */
async function signIn(page: Page, role: Role, token = TOKENS[role]): Promise<string[]> {
  const errors: string[] = [];
  page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/");
  await page.evaluate((t) => { sessionStorage.setItem("hiringplug.token", t); }, token);
  await page.reload();
  await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();
  return errors;
}

async function opens(page: Page, hash: string, heading: RegExp | string) {
  await page.goto("/" + hash);
  await expect(page.getByRole("heading", { level: 2, name: heading }).first()).toBeVisible();
  await expect(page.getByText("This page could not be shown")).toHaveCount(0);
}

const MAIN: [string, RegExp | string][] = [
  ["#/jobs", "Jobs"], ["#/candidates", "Candidates"], ["#/companies", "Companies"], ["#/contacts", "Contacts"], ["#/import", /Import/],
];

for (const role of ["owner", "recruiter", "viewer"] as const) {
  test(`every page opens for the ${role}`, async ({ page }) => {
    const errors = await signIn(page, role);
    for (const [hash, heading] of MAIN) await opens(page, hash, heading);
    // Every part of Settings this role has, from the Settings menu itself.
    await page.getByRole("link", { name: "Settings" }).click();
    const menu = page.getByRole("navigation", { name: "Settings" });
    await expect(menu).toBeVisible();
    const paths = await menu.locator("a").evaluateAll((links) => links.map((link) => link.getAttribute("href") ?? ""));
    expect(paths).toContain("#/account");
    if (role === "owner") expect(paths).toEqual(expect.arrayContaining(["#/settings", "#/ai-provider", "#/team", "#/system"]));
    else expect(paths).not.toContain("#/settings");
    for (const path of paths) {
      const link = menu.locator(`a[href="${path}"]`);
      const group = link.locator("xpath=ancestor::details");
      if (await group.getAttribute("open") === null) await group.locator("summary").click();
      await link.click();
      await expect(page.getByText("This page could not be shown")).toHaveCount(0);
      await expect(link).toHaveAttribute("aria-current", "page");
      await expect(link).toBeVisible();
    }
    if (role !== "owner") {
      await opens(page, "#/ai-provider", "AI provider");
      await expect(page.getByText("Only owners and admins choose the AI provider")).toBeVisible();
    }
    expect(errors).toEqual([]);
  });
}

test("settings search opens an exact editor and folding preserves unsaved list edits", async ({ page }) => {
  await signIn(page, "owner");
  await opens(page, "#/settings", "General");
  const search = page.getByRole("searchbox", { name: "Search settings and features" });
  await search.fill("who can see salary");
  await expect(page.getByRole("link", { name: "Salary in emails" })).toBeVisible();
  await search.press("Enter");
  await expect(page).toHaveURL(/#\/settings\/salary#salary-in-emails$/);
  await expect(page.getByRole("region", { name: "Salary in emails" })).toBeFocused();
  await search.fill("hiring manager");
  await page.getByRole("link", { name: "Contact roles" }).click();
  await expect(page).toHaveURL(/#\/settings\/lists#contact-roles$/);
  const editor = page.locator("details#contact-roles");
  const option = editor.getByRole("textbox", { name: "Contact roles option 1" });
  await expect(option).toBeVisible();
  await option.fill("Unsaved contact role");
  await editor.locator("summary").click();
  await expect(option).toBeHidden();
  await search.fill("Contact roles");
  await search.press("Enter");
  await expect(option).toHaveValue("Unsaved contact role");
  await search.fill("Company categories");
  await search.press("Enter");
  await expect(page.locator("details#company-categories")).toHaveAttribute("open", "");
  await expect(option).toHaveValue("Unsaved contact role");
});

test("a candidate's page opens, with their CV", async ({ page, request }) => {
  const errors = await signIn(page, "recruiter");
  const res = await request.post("/v1/candidates", {
    headers: { Authorization: `Bearer ${TOKENS.recruiter}` },
    data: { firstName: "Smoke", lastName: `Test ${Date.now()}` },
  });
  expect(res.status()).toBe(201);
  const { id, fullName } = (await res.json()) as { id: string; fullName: string };
  await opens(page, `#/candidates/${id}`, fullName);
  await expect(page.getByRole("button", { name: "Edit profile" })).toBeVisible();
  expect(errors).toEqual([]);
});

test("signing out ends the session on the server", async ({ page, request }) => {
  // A session of its own (signed in with the password), so the other tests' tokens keep working.
  const lines = devSetup();
  const password = /password "([^"]+)"/.exec(lines.join("\n"))?.[1];
  const email = lines.find((l) => l.startsWith("viewer "))?.trim().split(/\s+/)[1];
  const res = await request.post("/v1/auth/sign-in", { data: { email, password } });
  expect(res.status()).toBe(200);
  const { token } = (await res.json()) as { token: string };
  const errors = await signIn(page, "viewer", token);
  await page.getByRole("button", { name: "Sign out" }).click();
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
  const after = await request.get("/v1/me", { headers: { Authorization: `Bearer ${token}` } });
  expect(after.status(), "a copied token stops working at sign-out").toBe(401);
  expect(errors).toEqual([]);
});


test("industry search stays inside the viewport and shows names in applied filters", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  const errors = await signIn(page, "recruiter");
  await opens(page, "#/candidates", "Candidates");
  await page.getByRole("button", { name: /^Filters/ }).click();
  const form = page.getByRole("form", { name: "Advanced search" });
  // Main industries in one dropdown, their subindustries in the next.
  await form.getByRole("button", { name: "Industry", exact: true }).click();
  await form.getByRole("listbox", { name: "Industry", exact: true }).getByLabel("Financial Services", { exact: true }).check();
  await form.getByRole("button", { name: "Industry", exact: true }).click();
  await form.getByRole("button", { name: "Subindustry", exact: true }).click();
  const menu = form.getByRole("listbox", { name: "Subindustry", exact: true });
  await menu.getByLabel("FSI – Banking", { exact: true }).waitFor();
  const box = await menu.boundingBox();
  expect(box).not.toBeNull();
  expect(box?.x).toBeGreaterThanOrEqual(0);
  expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(1440);
  await menu.getByLabel("FSI – Banking", { exact: true }).check();
  await form.getByRole("button", { name: "Subindustry", exact: true }).click();
  await form.getByRole("button", { name: "Search", exact: true }).click();
  await expect(page.getByLabel("Search filters")).toContainText("FSI – Banking");
  await expect(page.getByLabel("Search filters")).not.toContainText("industry_");
  expect(errors).toEqual([]);
});
