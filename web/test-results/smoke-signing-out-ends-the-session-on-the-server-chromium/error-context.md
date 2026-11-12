# Instructions

- Following Playwright test failed.
- Explain why, be concise, respect Playwright best practices.
- Provide a snippet of code with the fix, if possible.

# Test info

- Name: smoke.spec.ts >> signing out ends the session on the server
- Location: e2e/smoke.spec.ts:90:1

# Error details

```
Error: expect(received).toBe(expected) // Object.is equality

Expected: 200
Received: 409
```

# Test source

```ts
  1   | // Every page and every part of Settings opens for each role without breaking
  2   | // (no error boundary, no console errors such as a blocked script), and signing
  3   | // out ends the session. The tokens come from `hp dev-setup`'s output.
  4   | import { readFileSync } from "node:fs";
  5   | 
  6   | import { expect, type Page, test } from "@playwright/test";
  7   | 
  8   | type Role = "owner" | "recruiter" | "viewer";
  9   | 
  10  | function devSetup(): string[] {
  11  |   const file = process.env.HP_E2E_TOKENS_FILE;
  12  |   if (!file) throw new Error("HP_E2E_TOKENS_FILE must name hp dev-setup's output (it lists each role's email and token)");
  13  |   return readFileSync(file, "utf8").split("\n");
  14  | }
  15  | 
  16  | function tokens(): Record<Role, string> {
  17  |   const lines = devSetup();
  18  |   const file = process.env.HP_E2E_TOKENS_FILE;
  19  |   const token = (role: Role) => {
  20  |     const i = lines.findIndex((l) => l.startsWith(role + " "));
  21  |     const t = lines[i + 1]?.trim();
  22  |     if (i < 0 || !t) throw new Error(`no ${role} token in ${file}`);
  23  |     return t;
  24  |   };
  25  |   return { owner: token("owner"), recruiter: token("recruiter"), viewer: token("viewer") };
  26  | }
  27  | 
  28  | const TOKENS = tokens();
  29  | 
  30  | /** Opens the app signed in as role (or with token), watching for console errors. */
  31  | async function signIn(page: Page, role: Role, token = TOKENS[role]): Promise<string[]> {
  32  |   const errors: string[] = [];
  33  |   page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
  34  |   page.on("pageerror", (e) => errors.push(e.message));
  35  |   await page.goto("/");
  36  |   await page.evaluate((t) => { sessionStorage.setItem("hiringplug.token", t); }, token);
  37  |   await page.reload();
  38  |   await expect(page.getByRole("navigation", { name: "Main" })).toBeVisible();
  39  |   return errors;
  40  | }
  41  | 
  42  | async function opens(page: Page, hash: string, heading: RegExp | string) {
  43  |   await page.goto("/" + hash);
  44  |   await expect(page.getByRole("heading", { level: 2, name: heading }).first()).toBeVisible();
  45  |   await expect(page.getByText("This page could not be shown")).toHaveCount(0);
  46  | }
  47  | 
  48  | const MAIN: [string, RegExp | string][] = [
  49  |   ["#/jobs", "Jobs"], ["#/candidates", "Candidates"], ["#/companies", "Companies"], ["#/contacts", "Contacts"], ["#/import", /Import/],
  50  | ];
  51  | 
  52  | for (const role of ["owner", "recruiter", "viewer"] as const) {
  53  |   test(`every page opens for the ${role}`, async ({ page }) => {
  54  |     const errors = await signIn(page, role);
  55  |     for (const [hash, heading] of MAIN) await opens(page, hash, heading);
  56  |     // Every part of Settings this role has, from the Settings menu itself.
  57  |     await page.getByRole("link", { name: "Settings" }).click();
  58  |     const menu = page.getByRole("navigation", { name: "Settings" });
  59  |     await expect(menu).toBeVisible();
  60  |     const parts = await menu.getByRole("link").allTextContents();
  61  |     expect(parts).toContain("Your account");
  62  |     if (role === "owner") expect(parts).toEqual(expect.arrayContaining(["General", "AI provider", "Team", "System"]));
  63  |     else expect(parts).not.toContain("General");
  64  |     for (const part of parts) {
  65  |       await menu.getByRole("link", { name: part, exact: true }).click();
  66  |       await expect(page.getByText("This page could not be shown")).toHaveCount(0);
  67  |       await expect(menu.getByRole("link", { name: part, exact: true })).toHaveAttribute("aria-current", "page");
  68  |     }
  69  |     if (role !== "owner") { // where the AI is set up, and who may do it
  70  |       await menu.getByRole("link", { name: "AI provider", exact: true }).click();
  71  |       await expect(page.getByText("Only owners and admins choose the AI provider")).toBeVisible();
  72  |     }
  73  |     expect(errors).toEqual([]);
  74  |   });
  75  | }
  76  | 
  77  | test("a candidate's page opens, with their CV", async ({ page, request }) => {
  78  |   const errors = await signIn(page, "recruiter");
  79  |   const res = await request.post("/v1/candidates", {
  80  |     headers: { Authorization: `Bearer ${TOKENS.recruiter}` },
  81  |     data: { firstName: "Smoke", lastName: `Test ${Date.now()}` },
  82  |   });
  83  |   expect(res.status()).toBe(201);
  84  |   const { id, fullName } = (await res.json()) as { id: string; fullName: string };
  85  |   await opens(page, `#/candidates/${id}`, fullName);
  86  |   await expect(page.getByRole("button", { name: "Edit profile" })).toBeVisible();
  87  |   expect(errors).toEqual([]);
  88  | });
  89  | 
  90  | test("signing out ends the session on the server", async ({ page, request }) => {
  91  |   // A session of its own (signed in with the password), so the other tests' tokens keep working.
  92  |   const lines = devSetup();
  93  |   const password = /password "([^"]+)"/.exec(lines.join("\n"))?.[1];
  94  |   const email = lines.find((l) => l.startsWith("viewer "))?.trim().split(/\s+/)[1];
  95  |   const res = await request.post("/v1/auth/sign-in", { data: { email, password } });
> 96  |   expect(res.status()).toBe(200);
      |                        ^ Error: expect(received).toBe(expected) // Object.is equality
  97  |   const { token } = (await res.json()) as { token: string };
  98  |   const errors = await signIn(page, "viewer", token);
  99  |   await page.getByRole("button", { name: "Sign out" }).click();
  100 |   await expect(page.getByRole("button", { name: "Sign in" })).toBeVisible();
  101 |   const after = await request.get("/v1/me", { headers: { Authorization: `Bearer ${token}` } });
  102 |   expect(after.status(), "a copied token stops working at sign-out").toBe(401);
  103 |   expect(errors).toEqual([]);
  104 | });
  105 | 
  106 | 
  107 | test("industry search stays inside the viewport and shows names in applied filters", async ({ page }) => {
  108 |   await page.setViewportSize({ width: 1440, height: 1000 });
  109 |   const errors = await signIn(page, "recruiter");
  110 |   await opens(page, "#/candidates", "Candidates");
  111 |   await page.getByRole("button", { name: /Advanced search/ }).click();
  112 |   const form = page.getByRole("form", { name: "Advanced search" });
  113 |   // Main industries in one dropdown, their subindustries in the next.
  114 |   await form.getByRole("button", { name: "Industry", exact: true }).click();
  115 |   await form.getByRole("listbox", { name: "Industry", exact: true }).getByLabel("Financial Services", { exact: true }).check();
  116 |   await form.getByRole("button", { name: "Industry", exact: true }).click();
  117 |   await form.getByRole("button", { name: "Subindustry", exact: true }).click();
  118 |   const menu = form.getByRole("listbox", { name: "Subindustry", exact: true });
  119 |   await menu.getByLabel("FSI – Banking", { exact: true }).waitFor();
  120 |   const box = await menu.boundingBox();
  121 |   expect(box).not.toBeNull();
  122 |   expect(box?.x).toBeGreaterThanOrEqual(0);
  123 |   expect((box?.x ?? 0) + (box?.width ?? 0)).toBeLessThanOrEqual(1440);
  124 |   await menu.getByLabel("FSI – Banking", { exact: true }).check();
  125 |   await form.getByRole("button", { name: "Subindustry", exact: true }).click();
  126 |   await form.getByRole("button", { name: "Search", exact: true }).click();
  127 |   await expect(page.getByLabel("Search filters")).toContainText("FSI – Banking");
  128 |   await expect(page.getByLabel("Search filters")).not.toContainText("industry_");
  129 |   expect(errors).toEqual([]);
  130 | });
  131 | 
```