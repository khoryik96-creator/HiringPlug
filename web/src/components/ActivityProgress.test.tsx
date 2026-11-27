import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { ActivityProgress, progress } from "./ActivityProgress";
import { monthName, RevenueForecast } from "./RevenueForecast";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const goal = (calls: number | null, cvsSent: number | null): Schemas["ActivityGoal"] => ({ calls, cvsSent, interviews: null, placements: 2 });
const activity = (over: Partial<Schemas["ActivityReport"]> = {}): Schemas["ActivityReport"] => ({
  period: "week", from: "2026-11-16", to: "2026-11-22", team: true,
  rows: [
    { userId: "usr_a", userName: "Aina", done: { calls: 12, cvsSent: 5, interviews: 2, placements: 0, categories: {} }, target: goal(20, 5) },
    { userId: "usr_w", userName: "Wei", done: { calls: 3, cvsSent: 0, interviews: 0, placements: 1, categories: {} }, target: null },
  ],
  ...over,
});

describe("activity against targets", () => {
  it("says how far along each target is", () => {
    expect(progress(12, 20)).toEqual({ text: "12 of 20", percent: 60 });
    expect(progress(7, 5)).toEqual({ text: "7 of 5", percent: 100 });
    expect(progress(3, null)).toEqual({ text: "3", percent: null });
  });

  it("lists everyone for owners, with progress against each target", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(activity()));
    renderWithClient(<ActivityProgress />);
    const panel = await screen.findByRole("region", { name: "Activity against targets" });
    const aina = await within(panel).findByRole("row", { name: /Aina/ });
    expect(within(aina).getByText("12 of 20")).toBeTruthy();
    expect(within(aina).getByRole("meter", { name: "Aina, Calls: progress" }).getAttribute("aria-valuenow")).toBe("60");
    expect(within(aina).getByRole("meter", { name: "Aina, CVs sent: progress" }).className).toContain("met");
    const wei = within(panel).getByRole("row", { name: /Wei/ });
    expect(within(wei).queryByRole("meter"), "no target: just the count").toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "This month" }));
    await waitFor(() => { expect(get).toHaveBeenLastCalledWith("/v1/reports/activity", { params: { query: { period: "month" } } }); });
  });

  it("shows a stage category someone has a target for", async () => {
    const withRounds = activity({ rows: [
      { userId: "usr_a", userName: "Aina", done: { calls: 0, cvsSent: 0, interviews: 0, placements: 0, categories: { interview: 3, applied: 9 } },
        target: { ...goal(null, null), categories: { interview: 4 } } },
    ] });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/stage-categories"
      ? { version: "0", items: [{ key: "applied", label: "Applied", kind: "applied" }, { key: "interview", label: "1st Interview", kind: "interview" },
        { key: "interview_2", label: "2nd Interview", kind: "interview" }] }
      : withRounds))) as never);
    renderWithClient(<ActivityProgress onlyUser="usr_a" title="Your activity" />);
    const panel = await screen.findByRole("region", { name: "Your activity" });
    expect(await within(panel).findByRole("meter", { name: "1st Interview: progress" })).toBeTruthy();
    expect(within(panel).getByText("of 4")).toBeTruthy();
    expect(within(panel).queryByText("2nd Interview"), "no target for it: left out").toBeNull();
    expect(within(panel).queryByText("Applied")).toBeNull();
  });

  it("shows one person their own as cards (the dashboard)", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(activity()));
    renderWithClient(<ActivityProgress onlyUser="usr_a" title="Your activity" />);
    const panel = await screen.findByRole("region", { name: "Your activity" });
    expect(within(panel).queryByText("Wei")).toBeNull();
    expect(await within(panel).findByText("of 20")).toBeTruthy();
    expect(within(panel).getAllByText("No target set")).toHaveLength(1); // interviews
  });

  it("says where targets are set when there are none", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(activity({ team: false, rows: [{ userId: "usr_w", userName: "Wei", done: { calls: 0, cvsSent: 0, interviews: 0, placements: 0, categories: {} }, target: null }] })));
    renderWithClient(<ActivityProgress />);
    expect(await screen.findByText(/owners and admins set them in Settings/)).toBeTruthy();
  });
});

const forecast: Schemas["Forecast"] = {
  months: [{ month: "2026-11", totals: [{ currency: "MYR", expected: "10080.00", full: "14400.00" }] }],
  recruiters: [{ recruiterId: "usr_a", recruiterName: "Aina", totals: [{ currency: "MYR", expected: "10080.00", full: "14400.00" }] }],
  lines: [{
    applicationId: "appl_1", candidateName: "Siti", jobTitle: "Finance Manager", companyName: "Maju Bhd", stage: "offer", recruiterId: "usr_a",
    recruiterName: "Aina", month: "2026-11", currency: "MYR", fee: "14400.00", percent: 70, expected: "10080.00", feeFrom: "terms",
  }],
  gaps: [{ applicationId: "appl_2", candidateName: "Kumar", jobTitle: "Clerk", stage: "submitted", reason: "no_salary_budget" }],
  stageWeights: [{ stage: "submitted", percent: 10, monthsAhead: 2 }, { stage: "offer", percent: 70, monthsAhead: 0 }],
};

describe("revenue forecast", () => {
  it("shows what is expected per month and per recruiter, and what was not counted", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(forecast));
    renderWithClient(<RevenueForecast />);
    const months = await screen.findByRole("table", { name: "Forecast by month" });
    expect(within(months).getByText("November 2026")).toBeTruthy();
    expect(within(months).getByText("MYR 10,080.00")).toBeTruthy();
    expect(within(screen.getByRole("table", { name: "Forecast by recruiter" })).getByText("Aina")).toBeTruthy();
    expect(screen.getByText(/Submitted 10%, Offer 70%/)).toBeTruthy();
    expect(screen.getByText(/Kumar, Clerk \(Submitted\): the job has no monthly or yearly salary budget/)).toBeTruthy();
  });

  it("names months", () => {
    expect(monthName("2027-01")).toBe("January 2027");
  });
});
