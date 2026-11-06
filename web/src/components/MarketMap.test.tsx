import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { MarketMap } from "./MarketMap";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const map = (over: Partial<Schemas["MarketPool"]> = {}, insights: Schemas["MarketInsights"] | null = null): Schemas["MarketMap"] => ({
  jobId: "jo_1", jobTitle: "Senior Finance Manager", searchedFor: "finance", insights,
  pool: {
    total: 3, medianNoticeDays: 60, salaryRestricted: false,
    availability: [{ key: "looking", count: 1 }, { key: "passive", count: 1 }, { key: "unknown", count: 1 }],
    workAuthorization: [{ key: "local", count: 2 }, { key: "expat", count: 1 }],
    companies: [{ key: "Nestlé Malaysia", count: 2 }, { key: "Grab", count: 1 }],
    locations: [{ key: "Petaling Jaya, Malaysia", count: 1 }],
    salary: [{ currency: "MYR", period: "month", current: { count: 3, low: "9000", median: "12000", high: "16000" }, expected: null }],
    ...over,
  },
});

function reads(m: Schemas["MarketMap"], aiOn = true) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/assistant" ? ok({ enabled: aiOn, usedTokens: 0, monthlyTokens: 1000 }) : ok(m),
  )) as never);
}

describe("market map", () => {
  it("shows who does the work, where they work and live, and their pay", async () => {
    reads(map());
    renderWithClient(<MarketMap jobId="jo_1" permissions={["market:read", "assistant:use"]} />);
    const panel = await screen.findByRole("region", { name: "Market map" });
    expect(await within(panel).findByText("In your database: current title with “finance”")).toBeTruthy();
    expect(within(panel).getByText("candidates").previousSibling?.textContent).toBe("3");
    expect(within(within(panel).getByRole("list", { name: "Where they work" })).getAllByRole("listitem").map((li) => li.textContent))
      .toEqual(["Nestlé Malaysia2", "Grab1"]);
    expect(panel.textContent).toContain("Spousal / RP-T1");
    const pay = within(panel).getByRole("table", { name: "Pay" });
    expect(pay.textContent).toContain("RM 9,000RM 12,000RM 16,000");
    expect(await within(panel).findByRole("button", { name: "Get insights" })).toBeTruthy(); // the AI is on
  });

  it("says when pay is restricted, and asks the AI for insights", async () => {
    reads(map({ salary: [], salaryRestricted: true }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "ask_1" }));
    renderWithClient(<MarketMap jobId="jo_1" permissions={["market:read", "assistant:use"]} />);
    expect(await screen.findByText(/Pay is restricted for your role/)).toBeTruthy();
    fireEvent.click(await screen.findByRole("button", { name: "Get insights" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ subjectType: "job", subjectId: "jo_1", kind: "market_map" });
  });

  it("shows the AI's insights, or says the AI is off; nothing without the permission", async () => {
    reads(map({}, { answerId: "ask_1", status: "done", text: "Target companies\n- Nestlé\n- Dutch Lady", model: "m", askedAt: "2026-11-06T00:00:00Z" }), false);
    renderWithClient(<MarketMap jobId="jo_1" permissions={["market:read"]} />);
    expect(await screen.findByText("Dutch Lady")).toBeTruthy();
    expect(screen.getByText(/AI estimates/)).toBeTruthy();
    expect(screen.getByText("Off: no AI provider is set up.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /insights/ })).toBeNull();
    cleanup();
    renderWithClient(<MarketMap jobId="jo_1" permissions={["jobs:read"]} />);
    expect(screen.queryByRole("region", { name: "Market map" })).toBeNull();
  });
});
