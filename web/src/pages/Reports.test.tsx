import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { sourceLabel } from "../labels";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { Reports } from "./Reports";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const report = (over: Partial<Schemas["Report"]> = {}): Schemas["Report"] => ({
  from: "2026-08-06", to: "2026-11-03", feesHidden: false,
  funnel: [
    { stage: "applied", reached: 4, percentOfPrevious: null }, { stage: "screening", reached: 3, percentOfPrevious: 75 },
    { stage: "submitted", reached: 3, percentOfPrevious: 100 }, { stage: "interview", reached: 3, percentOfPrevious: 100 },
    { stage: "offer", reached: 2, percentOfPrevious: 67 }, { stage: "placed", reached: 2, percentOfPrevious: 100 },
  ],
  sources: [{ source: "jobstreet", applications: 4, interviewed: 2, placed: 1 }, { source: "unknown", applications: 1, interviewed: 0, placed: 0 }],
  recruiters: [{ recruiterId: "usr_1", recruiterName: "Aina", placements: 2, fellThrough: 1, fees: [{ currency: "MYR", amount: "35400.00" }] }],
  timeToFill: { medianDaysToFill: 30, medianDaysToPlace: 21, jobs: [{ jobId: "jo_1", jobTitle: "Accountant", daysToFill: 30, placements: 2 }] },
  ...over,
});

/** The report for the summary, the company's settings for source names. */
const reads = (summary: unknown) => vi.spyOn(api, "GET").mockImplementation(((path: string) =>
  Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(summary))) as never);

describe("reports", () => {
  it("shows conversion, sources, recruiters' fees and time to fill", async () => {
    reads(report());
    renderWithClient(<Reports />);
    const funnel = await screen.findByRole("region", { name: "Pipeline conversion" });
    expect(within(funnel).getByText("67%")).toBeTruthy();
    const sources = screen.getByRole("region", { name: "Sources" });
    expect(within(sources).getByText("JobStreet")).toBeTruthy();
    expect(within(sources).getByText("Not recorded")).toBeTruthy();
    expect(within(sources).getByText("25%")).toBeTruthy(); // 1 placed of 4
    expect(screen.getByText("MYR 35,400.00")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Accountant" }).getAttribute("href")).toBe("#/jobs/jo_1");
  });

  it("asks for the chosen period, and says when fees are hidden", async () => {
    const get = reads(report({ feesHidden: true, recruiters: [] }));
    renderWithClient(<Reports />);
    expect(await screen.findByText(/Fees are not shown to your role/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("From"), { target: { value: "2026-01-01" } });
    await waitFor(() => {
      expect(get).toHaveBeenLastCalledWith("/v1/reports/summary", { params: { query: { from: "2026-01-01", to: undefined } } });
    });
  });

  it("names a company's own sources", async () => {
    const settings = testFieldSettings() as unknown as Schemas["FieldSettings"];
    settings.lists[0]?.options.push({ key: "job_fair", label: "Job fair", hint: null, color: null, hidden: false, builtIn: false });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields" ? ok(settings)
      : ok(report({ sources: [{ source: "job_fair", applications: 2, interviewed: 1, placed: 0 }] })))) as never);
    renderWithClient(<Reports />);
    expect(await screen.findByRole("cell", { name: "Job fair" })).toBeTruthy();
  });

  it("names sources from either list", () => {
    expect(sourceLabel("careers_page")).toBe("Careers page");
    expect(sourceLabel("direct")).toBe("Direct");
    expect(sourceLabel("something_new")).toBe("something_new");
  });
});
