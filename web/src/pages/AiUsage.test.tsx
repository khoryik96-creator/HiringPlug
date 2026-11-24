import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { failure, invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { AiUsage } from "./AiUsage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const emptyMonth = (month: string): Schemas["AiUsageMonth"] => ({ month, uses: 0, inputTokens: 0, outputTokens: 0, cost: null, byFeature: [] });
const months = (latest: Schemas["AiUsageMonth"], october: Schemas["AiUsageMonth"]) => [
  latest, october,
  ...["2026-09", "2026-08", "2026-07", "2026-06", "2026-05", "2026-04", "2026-03", "2026-02", "2026-01", "2025-12"].map(emptyMonth),
];

const moneyUsage: Schemas["AiUsage"] = {
  month: "2026-11", uses: 612, inputTokens: 1_000_000, outputTokens: 210_442, cost: "3.8412",
  byFeature: [
    { feature: "cv_reading", uses: 318, inputTokens: 402_100, outputTokens: 61_300, cost: "1.3700" },
    { feature: "fit_score", uses: 164, inputTokens: 288_400, outputTokens: 40_200, cost: "0.9800" },
  ],
  byPerson: [
    { userId: "usr_01jbz6w0m0k8q9x2r3s4t5v6w7", name: "Aina Rahman", uses: 400, tokens: 900_000, cost: "2.9000" },
    { userId: null, name: "Automatic", uses: 212, tokens: 310_442, cost: "0.9412" },
  ],
  months: months(
    { month: "2026-11", uses: 612, inputTokens: 1_000_000, outputTokens: 210_442, cost: "3.8412", byFeature: [] },
    { month: "2026-10", uses: 90, inputTokens: 500_000, outputTokens: 20_000, cost: "1234.5000",
      byFeature: [{ feature: "interview_prep", uses: 90, inputTokens: 500_000, outputTokens: 20_000, cost: "1234.5000" }] },
  ),
  limit: { kind: "money", tokens: null, money: "10.00", currency: "USD", warnPercent: 80, usedPercent: 38, reached: false, resetsAt: "2026-11-30T16:00:00Z" },
  pricesSet: true,
};

const tokenUsage: Schemas["AiUsage"] = {
  ...moneyUsage, cost: null, pricesSet: false,
  byFeature: moneyUsage.byFeature.map((f) => ({ ...f, cost: null })),
  byPerson: moneyUsage.byPerson.map((p) => ({ ...p, cost: null })),
  months: moneyUsage.months.map((m) => ({ ...m, cost: null })),
  limit: { ...moneyUsage.limit, kind: "tokens", tokens: 2_000_000, money: null, usedPercent: 85 },
};

const defaults: Schemas["AiUsageSettings"] = {
  limitKind: "tokens", limitTokens: null, serverDefaultTokens: 2_000_000, limitMoney: null, currency: "USD",
  priceInputPerMillion: null, priceOutputPerMillion: null, warnPercent: 80, version: 0, updatedAt: null,
};
const moneySettings: Schemas["AiUsageSettings"] = {
  ...defaults, limitKind: "money", limitMoney: "10.00", priceInputPerMillion: "0.2800", priceOutputPerMillion: "0.4200", version: 3, updatedAt: "2026-11-20T00:00:00Z",
};

function reads(usage: Schemas["AiUsage"], settings: Schemas["AiUsageSettings"] | (() => Schemas["AiUsageSettings"])) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(ok(path === "/v1/ai-usage/settings" ? (typeof settings === "function" ? settings() : settings) : usage))) as never);
}

describe("Settings › AI usage", () => {
  it("shows a money limit with the cost exactly as sent, tokens beside it and the meter", async () => {
    const get = reads(moneyUsage, moneySettings);
    renderWithClient(<AiUsage canManage />);
    const month = await screen.findByRole("region", { name: "This month" });
    expect(within(month).getByText("November 2026")).toBeTruthy();
    expect(within(month).getByText("Within limit")).toBeTruthy();
    expect(within(month).getByText("USD 3.8412")).toBeTruthy();
    expect(within(month).getByText("of USD 10.00 this month")).toBeTruthy();
    expect(within(month).getByText("1.21M")).toBeTruthy();
    expect(within(month).getByText("tokens · 612 uses")).toBeTruthy();
    expect(within(month).getByText("38% used · resets 1 Dec · warning at 80%")).toBeTruthy();
    expect(within(month).getByRole("meter").getAttribute("aria-valuenow")).toBe("38");
    const asked: unknown[][] = get.mock.calls;
    expect(asked.find((c) => c[0] === "/v1/ai-usage")?.[1]).toEqual({ params: { query: { months: 12 } } });
    const table = screen.getByRole("table", { name: "This month by feature" });
    expect(within(table).getByText("CV reading")).toBeTruthy();
    expect(within(table).getByText("USD 1.3700")).toBeTruthy();
    expect(screen.getByText("Costs are worked out from the token counts and your prices; your provider's bill is the final word.")).toBeTruthy();
  });

  it("shows a token limit near the limit, without costs until prices are set", async () => {
    reads(tokenUsage, defaults);
    renderWithClient(<AiUsage canManage />);
    const month = await screen.findByRole("region", { name: "This month" });
    expect(within(month).getByText("Near the limit")).toBeTruthy();
    expect(within(month).getByText("of 2M tokens this month")).toBeTruthy();
    expect(within(month).queryByText(/USD/)).toBeNull();
    expect(screen.getByText("Set the prices per million tokens to see what each use costs.")).toBeTruthy();
    expect(within(screen.getByRole("table", { name: "This month by feature" })).getAllByText("—")).toHaveLength(2);
    // The form starts on the server's token limit.
    expect(await screen.findByLabelText<HTMLInputElement>("Use the server default (2,000,000 tokens)")).toHaveProperty("checked", true);
    expect(screen.getByLabelText<HTMLInputElement>("Monthly limit (tokens)").disabled).toBe(true);
  });

  it("says when the limit is reached", async () => {
    reads({ ...moneyUsage, limit: { ...moneyUsage.limit, usedPercent: 104, reached: true } }, moneySettings);
    renderWithClient(<AiUsage canManage />);
    const month = await screen.findByRole("region", { name: "This month" });
    expect(within(month).getByText("Limit reached")).toBeTruthy();
    expect(within(month).getByRole("meter").getAttribute("aria-valuenow")).toBe("100");
  });

  it("shows the values, not a form, to those who only read them", async () => {
    reads(moneyUsage, moneySettings);
    renderWithClient(<AiUsage canManage={false} />);
    const limit = await screen.findByRole("region", { name: "Limit and prices" });
    expect(await within(limit).findByText("USD 10.00 a month")).toBeTruthy();
    expect(within(limit).getByText("USD 0.2800 in, USD 0.4200 out")).toBeTruthy();
    expect(within(limit).queryByRole("textbox")).toBeNull();
    expect(within(limit).queryByRole("button", { name: "Save" })).toBeNull();
  });

  it("saves a money limit with its prices", async () => {
    reads(tokenUsage, defaults);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(moneySettings));
    renderWithClient(<AiUsage canManage />);
    fireEvent.change(await screen.findByLabelText("Limit in"), { target: { value: "money" } });
    fireEvent.change(screen.getByLabelText("Monthly limit (USD)"), { target: { value: "1,000.50" } });
    fireEvent.change(screen.getByLabelText("Price per 1M tokens in"), { target: { value: "0.27" } });
    fireEvent.change(screen.getByLabelText("Price per 1M tokens out"), { target: { value: "1.10" } });
    fireEvent.change(screen.getByLabelText("Warn at (%)"), { target: { value: "90" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(put.mock.calls[0]?.[0]).toBe("/v1/ai-usage/settings");
    expect(sentBody(put)).toEqual({
      limitKind: "money", limitTokens: null, limitMoney: "1000.50", currency: "USD",
      priceInputPerMillion: "0.27", priceOutputPerMillion: "1.10", warnPercent: 90, version: 0,
    });
    expect(await screen.findByText("Saved.")).toBeTruthy();
  });

  it("goes back to the server's default token limit", async () => {
    reads(moneyUsage, moneySettings);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(defaults));
    renderWithClient(<AiUsage canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Use the server default" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ limitKind: "tokens", limitTokens: null, version: 3 });
  });

  it("shows the API's problem beside its field", async () => {
    reads(moneyUsage, moneySettings);
    vi.spyOn(api, "PUT").mockResolvedValue(invalidField("priceOutputPerMillion", "must be a price with at most 4 decimal places, e.g. 0.27"));
    renderWithClient(<AiUsage canManage />);
    fireEvent.change(await screen.findByLabelText("Price per 1M tokens out"), { target: { value: "0.123456" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    const label = (await screen.findByText("must be a price with at most 4 decimal places, e.g. 0.27")).closest("label");
    expect(label?.textContent).toContain("Price per 1M tokens out");
  });

  it("reloads someone else's newer settings and says so", async () => {
    let saved = moneySettings;
    const get = reads(moneyUsage, () => saved);
    vi.spyOn(api, "PUT").mockImplementation((() => {
      saved = { ...moneySettings, limitMoney: "25.00", version: 4 };
      return Promise.resolve(failure("CONFLICT_STALE_VERSION", "Someone else changed the AI usage settings.", undefined, 409));
    }) as never);
    renderWithClient(<AiUsage canManage />);
    fireEvent.change(await screen.findByLabelText("Monthly limit (USD)"), { target: { value: "12.00" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/Someone else changed these settings while you were editing/)).toBeTruthy();
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Monthly limit (USD)").value).toBe("25.00"); });
    expect(get.mock.calls.filter((c) => c[0] === "/v1/ai-usage/settings").length).toBeGreaterThan(1);
  });

  it("lists who used the AI this month, with the system's own use as Automatic", async () => {
    reads(moneyUsage, moneySettings);
    renderWithClient(<AiUsage canManage />);
    fireEvent.click(await screen.findByRole("tab", { name: "By person" }));
    const table = await screen.findByRole("table", { name: "This month by person" });
    const rows = within(table).getAllByRole("row").slice(1).map((r) => r.textContent);
    expect(rows).toEqual(["Aina Rahman400900,000USD 2.9000", "Automatic212310,442USD 0.9412"]);
  });

  it("shows a month's features when its bar is clicked", async () => {
    reads(moneyUsage, moneySettings);
    renderWithClient(<AiUsage canManage />);
    const october = await screen.findByRole("button", { name: "October 2026: USD 1,234.5000, 520,000 tokens" });
    expect(screen.queryByRole("table", { name: "October 2026 by feature" })).toBeNull();
    fireEvent.click(october);
    const table = await screen.findByRole("table", { name: "October 2026 by feature" });
    expect(within(table).getByText("Interview prep")).toBeTruthy();
    expect(within(table).getByText("USD 1,234.5000")).toBeTruthy();
    expect(october.getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(october);
    expect(screen.queryByRole("table", { name: "October 2026 by feature" })).toBeNull();
  });
});
