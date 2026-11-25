import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { AiNotice, sentence } from "./AiNotice";

const down: Schemas["AiStatus"] = {
  down: true, provider: "AI 1", message: "the AI provider could not be reached",
  since: "2026-11-13T02:40:00Z", failedAt: "2026-11-13T02:45:00Z", paidPaused: false,
};
const fine: Schemas["AiStatus"] = { down: false, provider: null, message: null, since: null, failedAt: null, paidPaused: false };

beforeEach(() => { sessionStorage.clear(); });
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the notice when the AI is not answering", () => {
  // IMP-001: paid AI paused on the server says so, not "not answering".
  it("says paid AI is paused on the server, and where it is turned back on", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...fine, paidPaused: true }));
    renderWithClient(<AiNotice permissions={["settings:manage"]} />);
    const notice = await screen.findByRole("alert");
    expect(notice.textContent).toContain("Paid AI is paused on this server.");
    expect(notice.textContent).toContain("read by the rules alone");
    expect(screen.getByRole("link", { name: "See Settings > System" }).getAttribute("href")).toBe("#/system");
    fireEvent.click(screen.getByRole("button", { name: "Close the AI notice" }));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("says the AI is down, why, and what will not work", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(down));
    renderWithClient(<AiNotice permissions={[]} />);
    const notice = await screen.findByRole("alert");
    expect(notice.textContent).toContain("AI 1 is not answering");
    expect(notice.textContent).toContain("The AI provider could not be reached.");
    expect(notice.textContent).toContain("fit scores");
    expect(screen.queryByRole("link", { name: "Check the AI provider" }), "only for those who manage it").toBeNull();
  });

  it("gives those who manage the AI provider a link to it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(down));
    renderWithClient(<AiNotice permissions={["aiproviders:manage"]} />);
    expect((await screen.findByRole("link", { name: "Check the AI provider" })).getAttribute("href")).toBe("#/ai-provider");
  });

  it("shows nothing while the AI answers", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(fine));
    renderWithClient(<AiNotice permissions={[]} />);
    await waitFor(() => { expect(get).toHaveBeenCalled(); });
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("stays closed for that outage and comes back for a new one", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(down));
    renderWithClient(<AiNotice permissions={[]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Close the AI notice" }));
    expect(screen.queryByRole("alert")).toBeNull();
    cleanup();

    renderWithClient(<AiNotice permissions={[]} />);
    await waitFor(() => { expect(get).toHaveBeenCalledTimes(2); });
    expect(screen.queryByRole("alert"), "the same outage").toBeNull();
    cleanup();

    get.mockResolvedValue(ok({ ...down, since: "2026-11-13T09:00:00Z" }));
    renderWithClient(<AiNotice permissions={[]} />);
    expect(await screen.findByRole("alert"), "a new outage").toBeTruthy();
  });

  it("writes a reason as a sentence", () => {
    expect(sentence("the AI provider did not answer in time")).toBe("The AI provider did not answer in time.");
    expect(sentence("")).toBe("");
  });
});
