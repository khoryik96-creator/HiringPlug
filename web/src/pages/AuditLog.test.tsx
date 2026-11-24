import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { AuditLog } from "./AuditLog";

const USERS = [{ id: "usr_aina", name: "Aina Rahman", role: "admin", canInterview: true }, { id: "usr_wei", name: "Wei Jie Tan", role: "recruiter", canInterview: true }];

const entry = (over: Partial<Schemas["AuditEntry"]>): Schemas["AuditEntry"] => ({
  id: "aud_01", at: "2026-11-24T07:42:00Z", actorId: "usr_aina", actorKind: "user", actorName: "Aina Rahman",
  action: "candidate.updated", entityType: "candidate", entityId: "cand_01daniel", entityName: "Daniel Lim", candidateId: null, changes: {}, ...over,
});

const view = (over: Partial<Schemas["SalaryView"]>): Schemas["SalaryView"] => ({
  id: "sv_01", day: "2026-11-24", userId: "usr_aina", userName: "Aina Rahman", kind: "candidate", candidateId: "cand_01daniel",
  candidateName: "Daniel Lim", candidates: 1, count: 3, firstAt: "2026-11-24T01:00:00Z", lastAt: "2026-11-24T07:40:00Z", ...over,
});

type Page<T> = { items: T[]; nextCursor: string | null };
type Query = Record<string, unknown>;

function serve(log: (q: Query) => Page<Schemas["AuditEntry"]>, views: (q: Query) => Page<Schemas["SalaryView"]> = () => ({ items: [], nextCursor: null })) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: Query } }) => {
    const q = init?.params?.query ?? {};
    if (path === "/v1/users") return Promise.resolve(ok({ items: USERS }));
    if (path === "/v1/audit-log") return Promise.resolve(ok(log(q)));
    if (path === "/v1/salary-views") return Promise.resolve(ok(views(q)));
    return Promise.reject(new Error(`unexpected ${path}`));
  }) as never);
}

const calls = (get: ReturnType<typeof serve>, path: string) =>
  get.mock.calls.filter((c) => c[0] === path).map((c) => (c[1] as { params: { query: Query } }).params.query);

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date(2026, 10, 24, 16, 0));
  window.history.replaceState(null, "", "#/settings/audit-log");
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Settings › Audit log", () => {
  it("shows the Salary views tab only with its permission", async () => {
    serve(() => ({ items: [entry({})], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews={false} />);
    expect(await screen.findByRole("link", { name: "Daniel Lim" })).toBeTruthy();
    expect(screen.queryByRole("tab")).toBeNull();
    expect(screen.queryByText(/Salary views/)).toBeNull();
    cleanup();

    renderWithClient(<AuditLog canSeeSalaryViews />);
    expect((await screen.findAllByRole("tab")).map((t) => t.textContent)).toEqual(["Changes", "Salary views"]);
  });

  it("names a candidate on a job and opens the candidate", async () => {
    serve(() => ({ items: [entry({ action: "application.moved", entityType: "application", entityId: "appl_01siti",
      entityName: "Siti Aisyah · Accountant", candidateId: "cand_01siti", changes: { stage: { from: "screening", to: "interview" } } })], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews={false} />);
    expect(await screen.findByText("Moved to Interview")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Siti Aisyah · Accountant" }).getAttribute("href")).toBe("#/candidates/cand_01siti");
  });

  it("sends the filters, with What as the action, and keeps them in the address", async () => {
    const get = serve(() => ({ items: [entry({})], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews />);
    await screen.findByText("Changed a candidate");
    expect(calls(get, "/v1/audit-log")[0], "To left at today: no end, so nothing from the agency's today is cut off").toEqual({ from: "2026-10-26", limit: 50 });

    await screen.findByRole("option", { name: "Wei Jie Tan" });
    fireEvent.change(screen.getByLabelText("Who"), { target: { value: "usr_wei" } });
    fireEvent.change(screen.getByLabelText("What"), { target: { value: "sign-ins" } });
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "2026-11-01" } });
    await waitFor(() => {
      expect(calls(get, "/v1/audit-log").at(-1)).toEqual({ actor: "usr_wei", action: "user.signed_in", from: "2026-11-01", limit: 50 });
    });
    expect(window.location.hash).toBe("#/settings/audit-log?who=usr_wei&what=sign-ins&from=2026-11-01&to=2026-11-24");
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-11-20" } });
    await waitFor(() => { expect(calls(get, "/v1/audit-log").at(-1)).toMatchObject({ from: "2026-11-01", to: "2026-11-20" }); });
    fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-11-24" } });
    await waitFor(() => { expect(calls(get, "/v1/audit-log").at(-1)).not.toHaveProperty("to"); });

    fireEvent.change(screen.getByLabelText("Who"), { target: { value: "system" } });
    fireEvent.change(screen.getByLabelText("What"), { target: { value: "pipeline" } });
    await waitFor(() => { expect(calls(get, "/v1/audit-log").at(-1)).toMatchObject({ actor: "system", action: "application" }); });
  });

  it("opens from a link with its filters", async () => {
    window.history.replaceState(null, "", "#/settings/audit-log?tab=salary&who=usr_aina&what=export&from=2026-11-02&to=2026-11-20");
    const get = serve(() => ({ items: [], nextCursor: null }), () => ({ items: [], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews />);
    await waitFor(() => { expect(calls(get, "/v1/salary-views")[0]).toEqual({ user: "usr_aina", kind: "export", from: "2026-11-02", to: "2026-11-20", limit: 50 }); });
    expect(screen.getByRole("tab", { name: "Salary views" }).getAttribute("aria-selected")).toBe("true");
  });

  it("opens a row to show what changed, and a salary only as changed", async () => {
    serve(() => ({ items: [entry({ changes: {
      noticePeriod: { from: { value: 1, unit: "month" }, to: { value: 2, unit: "month" } },
      compensation: { changed: true, redacted: true },
    } })], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews={false} />);
    const row = (await screen.findByText("Changed a candidate")).closest("tr");
    expect(document.querySelector(".audit-diff")).toBeNull();
    fireEvent.click(row as HTMLElement);
    const box = await waitFor(() => {
      const found = document.querySelector<HTMLElement>(".audit-diff");
      if (!found) throw new Error("not open yet");
      return found;
    });
    expect(box.textContent).toContain("Notice period: 1 month → 2 months");
    expect(box.textContent).toContain("Compensation: changed (amounts are never kept here)");
    expect(box.textContent).not.toMatch(/\d{4}/);
    expect(screen.getByRole("button", { name: /Today/ }).getAttribute("aria-expanded")).toBe("true");
  });

  it("loads more with the cursor", async () => {
    const get = serve((q) => q.cursor
      ? { items: [entry({ id: "aud_02", action: "user.signed_in", entityType: "user", entityId: "usr_aina", entityName: "Aina Rahman" })], nextCursor: null }
      : { items: [entry({})], nextCursor: "aud_01" });
    renderWithClient(<AuditLog canSeeSalaryViews={false} />);
    expect(await screen.findByText("1 entry shown, more below")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Signed in")).toBeTruthy();
    expect(calls(get, "/v1/audit-log").at(-1)).toMatchObject({ cursor: "aud_01" });
    expect(screen.getByText("2 entries")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("lists who was shown a salary, with the candidates a list showed", async () => {
    const get = serve(() => ({ items: [], nextCursor: null }), () => ({ items: [
      view({}),
      view({ id: "sv_02", kind: "export", userName: "Priya Nair", candidateId: null, candidateName: null, candidates: 212, count: 1 }),
      view({ id: "sv_03", day: "2026-11-23", kind: "candidate_list", candidateId: null, candidateName: null, candidates: 1, count: 6 }),
    ], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews />);
    fireEvent.click(await screen.findByRole("tab", { name: "Salary views" }));
    const table = await screen.findByRole("table", { name: "Salary views" });
    const rows = within(table).getAllByRole("row").slice(1).map((r) => [...r.querySelectorAll("td")].slice(1).map((td) => td.textContent));
    expect(rows).toEqual([
      ["Aina Rahman", "Opened the candidate", "Daniel Lim", "3 times"], // "times" shows only on a phone, where the heading is hidden
      ["Priya Nair", "Exported with salary", "212 candidates", "1 time"],
      ["Aina Rahman", "Candidate list with salary", "1 candidate", "6 times"],
    ]);
    expect(within(table).getAllByRole("row")[3]?.textContent).toMatch(/^Yesterday/);
    expect(within(table).getByRole("link", { name: "Daniel Lim" }).getAttribute("href")).toBe("#/candidates/cand_01daniel");
    fireEvent.change(screen.getAllByLabelText("Saw")[0] as HTMLElement, { target: { value: "email_sent" } });
    await waitFor(() => { expect(calls(get, "/v1/salary-views").at(-1)).toMatchObject({ kind: "email_sent" }); });
  });

  it("leaves Automatic out of Who on Salary views (nobody is shown a salary automatically)", async () => {
    const get = serve(() => ({ items: [entry({ actorId: "system", actorKind: "system", actorName: "Automatic" })], nextCursor: null }),
      () => ({ items: [view({})], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews />);
    await screen.findByRole("option", { name: "Automatic" });
    fireEvent.change(screen.getByLabelText("Who"), { target: { value: "system" } });
    await waitFor(() => { expect(calls(get, "/v1/audit-log").at(-1)).toMatchObject({ actor: "system" }); });
    fireEvent.click(screen.getByRole("tab", { name: "Salary views" }));
    await screen.findByRole("table", { name: "Salary views" });
    expect(calls(get, "/v1/salary-views").at(-1)).not.toHaveProperty("user");
    const who = screen.getAllByLabelText<HTMLSelectElement>("Who").find((s) => s.closest("[role=tabpanel]:not([hidden])"));
    expect(who?.value).toBe("");
    expect(window.location.hash).not.toContain("who=");
  });

  it("says when nothing was found", async () => {
    serve(() => ({ items: [], nextCursor: null }));
    renderWithClient(<AuditLog canSeeSalaryViews={false} />);
    expect(await screen.findByText(/Nothing was changed in these days/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Download Excel" }).hasAttribute("disabled")).toBe(true);
  });

  it("downloads what the filters find as Excel", async () => {
    window.history.replaceState(null, "", "#/settings/audit-log?what=candidates&from=2026-11-01&to=2026-11-24");
    serve(() => ({ items: [entry({})], nextCursor: null }));
    const fetch = vi.fn().mockResolvedValue(new Response("x", { status: 200, headers: { "Content-Disposition": `attachment; filename="Audit log.xlsx"` } }));
    vi.stubGlobal("fetch", fetch);
    vi.stubGlobal("URL", class extends URL { static override createObjectURL = vi.fn().mockReturnValue("blob:x"); static override revokeObjectURL = vi.fn(); });
    let saved = "";
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { saved = this.download; });
    renderWithClient(<AuditLog canSeeSalaryViews={false} />);
    await screen.findByText("Changed a candidate");
    fireEvent.click(screen.getByRole("button", { name: "Download Excel" }));
    await waitFor(() => { expect(saved).toBe("Audit log.xlsx"); });
    const [path, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect([path, init.method]).toEqual(["/v1/audit-log/export", "POST"]);
    expect(JSON.parse(init.body as string), "To is today: up to now").toEqual({ format: "xlsx", action: "candidate", from: "2026-11-01" });
  });
});
