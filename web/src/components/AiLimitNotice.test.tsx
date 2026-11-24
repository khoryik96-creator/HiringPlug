import { useQuery } from "@tanstack/react-query";
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { AiLimitNotice, aiUsageStatusKey, resetDay } from "./AiLimitNotice";

// The agency's 1 Dec (Kuala Lumpur, 8 hours ahead of UTC).
const reached: Schemas["AiUsageStatus"] = { usedPercent: 103, warn: false, reached: true, resetsAt: "2026-11-30T16:00:00Z" };
const warned: Schemas["AiUsageStatus"] = { usedPercent: 85, warn: true, reached: false, resetsAt: "2026-11-30T16:00:00Z" };

/**
 * Shows once the status has loaded (it renders with the notice), so a test
 * can tell "the notice shows nothing" from "the status has not come yet".
 */
function Loaded() {
  const status = useQuery<Schemas["AiUsageStatus"]>({ queryKey: aiUsageStatusKey, queryFn: () => new Promise(() => undefined), enabled: false });
  return status.data ? <span>status loaded</span> : null;
}

function renderNotice() {
  renderWithClient(<><AiLimitNotice permissions={["aiusage:read"]} /><Loaded /></>);
  return screen.findByText("status loaded");
}

beforeEach(() => { sessionStorage.clear(); });
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the notice when the company's AI limit is reached", () => {
  it("tells everyone AI features are paused, and until when", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(reached));
    renderWithClient(<AiLimitNotice permissions={[]} />);
    const notice = await screen.findByRole("alert");
    expect(get.mock.calls[0]?.[0]).toBe("/v1/ai-usage/status");
    expect(notice.textContent).toContain("This month's AI limit is reached: AI features are paused until 1 Dec.");
    expect(notice.textContent).toContain("fit scores");
    expect(notice.textContent, "they refuse until then: nothing waits").toContain("try them again from 1 Dec.");
    expect(screen.queryByRole("link", { name: "See the company's AI use" }), "only for those who may see it").toBeNull();
  });

  it("links to the company's AI use for those who may see it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(reached));
    renderWithClient(<AiLimitNotice permissions={["aiusage:read"]} />);
    expect((await screen.findByRole("link", { name: "See the company's AI use" })).getAttribute("href")).toBe("#/settings/ai-usage");
  });

  it("shows nothing before the limit, the warning level included", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(warned));
    await renderNotice();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("stays closed for this month and comes back for the next", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(reached));
    renderWithClient(<AiLimitNotice permissions={[]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Close the AI limit notice" }));
    expect(screen.queryByRole("alert")).toBeNull();
    cleanup();

    await renderNotice();
    expect(get).toHaveBeenCalledTimes(2);
    expect(screen.queryByRole("alert"), "the same month").toBeNull();
    cleanup();

    get.mockResolvedValue(ok({ ...reached, resetsAt: "2026-12-31T16:00:00Z" }));
    renderWithClient(<AiLimitNotice permissions={[]} />);
    expect((await screen.findByRole("alert")).textContent, "next month's limit reached").toContain("paused until 1 Jan");
  });

  it("names the first of the month whatever the agency's zone", () => {
    expect(resetDay("2026-11-30T16:00:00Z")).toBe("1 Dec"); // 8 hours ahead
    expect(resetDay("2026-12-01T00:00:00Z")).toBe("1 Dec"); // UTC
    expect(resetDay("2026-12-01T10:00:00Z")).toBe("1 Dec"); // 10 hours behind
    expect(resetDay("2026-12-31T10:00:00Z")).toBe("1 Jan"); // 14 hours ahead
  });
});
