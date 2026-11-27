import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { periodLabel, PerformanceReport, rangeDates } from "./PerformanceReport";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const row = (from: string, to: string, over: Partial<Schemas["PerformanceRow"]> = {}): Schemas["PerformanceRow"] => ({
  from, to, cvsSent: 0, categories: {}, placements: 0, fees: [], ...over,
});
const perf = (over: Partial<Schemas["Performance"]> = {}): Schemas["Performance"] => ({
  grain: "month", from: "2026-01-01", to: "2026-02-20", users: [], team: true, feesHidden: false,
  rows: [row("2026-01-01", "2026-01-31", { cvsSent: 4, categories: { interview: 2 } }),
    row("2026-02-01", "2026-02-20", { cvsSent: 6, categories: { interview: 1, interview_final: 1 }, placements: 1, fees: [{ currency: "MYR", amount: "15000.00" }] })],
  total: row("2026-01-01", "2026-02-20", { cvsSent: 10, categories: { interview: 3, interview_final: 1 }, placements: 1, fees: [{ currency: "MYR", amount: "15000.00" }] }),
  conversions: [{ from: "submitted", to: "interview", entered: 9, reached: 6, percent: 67 }, { from: "offer", to: "placed", entered: 0, reached: 0, percent: null }],
  daysToFirstCv: { average: 3.5, jobs: 4 }, daysToFill: { average: 41, jobs: 1 },
  ...over,
});
const categories = { version: "0", items: [
  { key: "submitted", label: "Submitted", kind: "submitted" }, { key: "interview", label: "1st Interview", kind: "interview" },
  { key: "interview_final", label: "Final Interview", kind: "interview" }, { key: "offer", label: "Offer", kind: "offer" },
  { key: "placed", label: "Placed", kind: "placed" },
] };
function serve(answer: Schemas["Performance"]) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/stage-categories" ? categories
    : path === "/v1/reports/performance" ? answer
      : { period: "week", from: "2026-11-23", to: "2026-11-29", team: true, rows: [{ userId: "usr_a", userName: "Aina", done: { calls: 0, cvsSent: 0, interviews: 0, placements: 0, categories: {} }, target: null }] }))) as never);
}

describe("performance over time", () => {
  it("names periods and ranges", () => {
    expect(periodLabel("month", "2026-11-01")).toBe("Nov 2026");
    expect(periodLabel("quarter", "2026-10-01")).toBe("Q4 2026");
    expect(periodLabel("year", "2026-01-01")).toBe("2026");
    expect(periodLabel("week", "2026-11-23")).toBe("Week of 23 Nov 2026");
    expect(rangeDates("last_year", new Date("2026-11-27T00:00:00Z"))).toEqual({ from: "2025-01-01", to: "2025-12-31" });
    expect(rangeDates("last_12", new Date("2026-11-27T00:00:00Z"))).toEqual({ from: "2025-12-01", to: "" });
    expect(rangeDates("ytd", new Date("2026-11-27T00:00:00Z"))).toEqual({ from: "", to: "" });
  });

  it("shows billings, CVs sent, each category and placements by period, the year to date, conversions and times", async () => {
    const get = serve(perf());
    renderWithClient(<PerformanceReport canChoose meId="usr_me" />);
    const panel = await screen.findByRole("region", { name: "Performance" });
    expect(await within(panel).findByRole("columnheader", { name: "1st Interview" })).toBeTruthy();
    expect(within(panel).queryByRole("columnheader", { name: "Submitted" }), "CVs sent counts those").toBeNull();
    const ytd = within(panel).getByRole("row", { name: /Year to date/ });
    expect(within(ytd).getByText("10")).toBeTruthy();
    expect(within(ytd).getByText("MYR 15,000.00")).toBeTruthy();
    expect(within(panel).getByRole("row", { name: /Jan 2026/ })).toBeTruthy();
    expect(within(panel).getByText("Submitted → 1st Interview").parentElement?.textContent).toContain("67%6 of 9");
    expect(within(panel).getByText("3.5 days")).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "Quarter" }));
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/reports/performance", { params: { query: { grain: "quarter", from: undefined, to: undefined, user: undefined } } }); });
    fireEvent.click(within(panel).getByRole("button", { name: /Showing Whole team/ }));
    fireEvent.click(await within(panel).findByRole("checkbox", { name: "Aina" }));
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/reports/performance", { params: { query: { grain: "quarter", from: undefined, to: undefined, user: ["usr_a"] } } }); });
  });

  it("leaves billings out for a role that may not see fees", async () => {
    serve(perf({ feesHidden: true, total: row("2026-01-01", "2026-02-20"), rows: [] }));
    renderWithClient(<PerformanceReport canChoose={false} />);
    expect(await screen.findByText("Billings are not shown to your role.")).toBeTruthy();
    expect(screen.queryByRole("columnheader", { name: "Billings" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Showing/ }), "no one else's numbers to choose").toBeNull();
  });
});
