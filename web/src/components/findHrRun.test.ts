import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import * as companion from "../companion";
import { ok } from "../test-utils";
import { matchWords, newRun, runScan, splitName } from "./findHrRun";

afterEach(() => { vi.restoreAllMocks(); });

const scan = (over: Partial<Schemas["HrScan"]> = {}): Schemas["HrScan"] => ({
  companyId: "comp_1", linkedinCompanyId: "123", linkedinCompanyName: "Maju Bank", country: "MY", countryName: "Malaysia", geoId: "106808692",
  keywords: ["HR"], queries: ['"HR"'], pages: 2, status: "running", next: { query: 0, page: 1 }, pagesRead: 0, error: null, people: [],
  startedAt: "2026-11-17T00:00:00Z", updatedAt: "2026-11-17T00:00:00Z", ...over,
});

describe("Find HR's run", () => {
  it("tries a page that failed again on its own, then goes on to the next", async () => {
    const call = vi.spyOn(companion, "companionCall")
      .mockRejectedValueOnce(new Error("LinkedIn did not finish loading. Check again."))
      .mockResolvedValue({ people: [{ url: "https://www.linkedin.com/in/farah", name: "Farah Aziz", title: "HR Manager", location: "KL" }], has_next: false });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(scan({ status: "done", next: null, pagesRead: 1 })));
    const waits: number[] = [];
    const notes: string[] = [];
    const run = newRun((m) => { notes.push(m); }, (ms) => { waits.push(ms); return Promise.resolve(); });
    const end = await runScan(scan(), run, vi.fn());
    expect(end.status).toBe("done");
    expect(waits).toEqual([5000]);
    expect(notes[0]).toContain("Trying again in 5s");
    expect(call.mock.calls[1]?.[1]).toMatchObject({ company_id: "123", geo_id: "106808692", country: "Malaysia", query: '"HR"', page: 1 });
    expect((post.mock.calls[0]?.[1] as unknown as { body: unknown }).body).toMatchObject({ query: 0, page: 1, hasNext: false });
  });

  it("stops at the page that keeps failing, to resume there; a sign-in is not retried", async () => {
    vi.spyOn(companion, "companionCall").mockRejectedValue(new Error("LinkedIn needs you to sign in, or the profile or company could not be confirmed."));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(scan({ status: "stopped", error: "LinkedIn needs you to sign in" })));
    const wait = vi.fn(() => Promise.resolve());
    const end = await runScan(scan({ next: { query: 0, page: 2 } }), newRun(vi.fn(), wait), vi.fn());
    expect(wait).not.toHaveBeenCalled();
    expect(end.status).toBe("stopped");
    expect((post.mock.calls[0]?.[1] as unknown as { body: unknown }).body).toMatchObject({ query: 0, page: 2, error: expect.stringContaining("sign in") as unknown });
  });

  it("resumes a stopped scan at the page that failed", async () => {
    const call = vi.spyOn(companion, "companionCall").mockResolvedValue({ people: [], has_next: false });
    vi.spyOn(api, "POST").mockResolvedValue(ok(scan({ status: "done", next: null })));
    await runScan(scan({ status: "stopped", next: { query: 0, page: 2 }, error: "LinkedIn did not finish loading." }), newRun(vi.fn()), vi.fn());
    expect(call.mock.calls[0]?.[1]).toMatchObject({ page: 2 });
  });

  it("matches \"People & Culture\" titles for \"people and culture\", and the other way round", () => {
    expect(matchWords(["people and culture", "HR"])).toBe("people and culture,people & culture,HR");
    expect(matchWords(["People & Culture"])).toBe("People & Culture,People and Culture");
  });

  it("keeps a new contact's names within what a contact holds", () => {
    expect(splitName(`${"A".repeat(150)} Lee`).firstName).toHaveLength(100);
  });

  it("splits a name for a new contact", () => {
    expect(splitName("Farah binti Aziz")).toEqual({ firstName: "Farah binti", lastName: "Aziz" });
    expect(splitName("Madonna")).toEqual({ firstName: "Madonna", lastName: "" });
  });
});
