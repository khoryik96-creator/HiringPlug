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
    const parts = await menu.getByRole("link").allTextContents();
    expect(parts).toContain("Your account");
    if (role === "owner") expect(parts).toEqual(expect.arrayContaining(["General", "AI provider", "Team", "System"]));
    else expect(parts).not.toContain("General");
    for (const part of parts) {
      await menu.getByRole("link", { name: part, exact: true }).click();
      await expect(page.getByText("This page could not be shown")).toHaveCount(0);
      await expect(menu.getByRole("link", { name: part, exact: true })).toHaveAttribute("aria-current", "page");
    }
    if (role !== "owner") { // where the AI is set up, and who may do it
      await menu.getByRole("link", { name: "AI provider", exact: true }).click();
      await expect(page.getByText("Only owners and admins choose the AI provider")).toBeVisible();
    }
    expect(errors).toEqual([]);
  });
}

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
  await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  const after = await request.get("/v1/me", { headers: { Authorization: `Bearer ${token}` } });
  expect(after.status(), "a copied token stops working at sign-out").toBe(401);
  expect(errors).toEqual([]);
});
