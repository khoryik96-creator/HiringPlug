import { describe, expect, it } from "vitest";

import type { Permission } from "./api/client";
import { menu, resolve, ROUTES } from "./routes";
import { RECRUITER } from "./test-utils";

const OWNERISH: Permission[] = [...RECRUITER, "pipeline:read", "settings:manage", "aiproviders:manage", "reports:read", "emails:send"];

describe("the route table", () => {
  it("opens a record's page with its ID", () => {
    const r = resolve("#/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7", RECRUITER);
    expect([r.route.section, r.id]).toEqual(["candidates", "cand_01jbz6w0m0k8q9x2r3s4t5v6w7"]);
    expect(resolve("#/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7/screening", RECRUITER).route.path.source).toContain("screening");
    expect(resolve("#/jobs/jo_01jbz6w0m0k8q9x2r3s4t5v6w7", RECRUITER).id).toBe("jo_01jbz6w0m0k8q9x2r3s4t5v6w7");
  });

  it("never opens a page the user may not open", () => {
    for (const r of ROUTES.filter((x) => x.permission && !RECRUITER.includes(x.permission))) {
      const href = r.menu?.href;
      if (href) expect(resolve(href, RECRUITER).route.section, href).not.toBe(r.section);
    }
  });

  it("opens Settings at its first part the user may open", () => {
    expect(resolve("#/settings", RECRUITER).route.menu?.label).toBe("Your account");
    expect(resolve("#/settings", OWNERISH).route.menu?.label).toBe("General");
  });

  it("starts on the dashboard, or the candidates without the pipeline", () => {
    expect(resolve("", OWNERISH).route.section).toBe("dashboard");
    expect(resolve("#/nowhere", OWNERISH).route.section).toBe("dashboard");
    expect(resolve("", RECRUITER).route.section).toBe("candidates");
  });

  it("lists only what the user may open, in order", () => {
    expect(menu("main", RECRUITER).map((r) => r.menu?.label)).toEqual(["Jobs", "Candidates", "Companies", "Contacts", "Import"]);
    expect(menu("main", OWNERISH).map((r) => r.menu?.label)).toEqual(["Dashboard", "Jobs", "Candidates", "Companies", "Contacts", "Reports", "Import"]);
    expect(menu("settings", OWNERISH).map((r) => r.menu?.label)).toEqual(["General", "AI provider", "Email templates", "System", "Your account"]);
  });

  it("gives every menu entry an address it answers", () => {
    for (const r of ROUTES.filter((x) => x.menu)) expect(r.path.test(r.menu?.href ?? ""), r.section).toBe(true);
  });
});
