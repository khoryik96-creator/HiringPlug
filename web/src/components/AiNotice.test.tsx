import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { AiNotice, sentence } from "./AiNotice";

const down: Schemas["AiStatus"] = {
  down: true, provider: "DeepSeek", message: "the AI provider could not be reached",
  since: "2026-11-13T02:40:00Z", failedAt: "2026-11-13T02:45:00Z",
};
const fine: Schemas["AiStatus"] = { down: false, provider: null, message: null, since: null, failedAt: null };

beforeEach(() => { sessionStorage.clear(); });
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("the notice when the AI is not answering", () => {
  it("says the AI is down, why, and what will not work", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(down));
    renderWithClient(<AiNotice permissions={[]} />);
    const notice = await screen.findByRole("alert");
    expect(notice.textContent).toContain("DeepSeek is not answering");
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
