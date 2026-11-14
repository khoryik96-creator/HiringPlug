import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { AiProvider } from "./AiProvider";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const none = { model: null, keyHint: null, updatedAt: null, check: null };
const state = (over: Record<string, unknown> = {}) => ({
  active: "server", serverAvailable: false, keysSupported: true,
  providers: [{ provider: "deepseek", ...none }, { provider: "claude", ...none }, { provider: "openai", ...none }],
  ...over,
});
const withClaude = state({
  active: "claude",
  providers: [
    { provider: "deepseek", ...none },
    { provider: "claude", model: "claude-opus-5-5", keyHint: "…5678", updatedAt: "2026-11-06T00:00:00Z",
      check: { status: "failed", message: "the AI provider refused the API key", checkedAt: "2026-11-06T00:00:00Z" } },
    { provider: "openai", ...none },
  ],
});

describe("AI provider settings", () => {
  it("saves a key with the default model and never shows it back", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(state()));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(withClaude));
    renderWithClient(<AiProvider />);
    const card = await screen.findByRole("region", { name: "Claude" });
    fireEvent.click(within(card).getByRole("button", { name: "Save" })); // no key yet
    expect(put).not.toHaveBeenCalled();
    expect(within(card).getByLabelText<HTMLInputElement>("API key").validity.valueMissing).toBe(true);
    expect(screen.getByRole("radio", { name: "Claude" })).toHaveProperty("disabled", true);
    fireEvent.change(within(card).getByLabelText("API key"), { target: { value: " sk-ant-secret-5678 " } });
    fireEvent.click(within(card).getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ model: "claude-opus-5-5", apiKey: "sk-ant-secret-5678" });
    expect(await within(card).findByText("The AI provider refused the API key")).toBeTruthy();
    expect(within(card).getByLabelText("API key (saved: …5678)")).toHaveProperty("value", "");
    expect(screen.queryByText(/sk-ant-secret/)).toBeNull();
  });

  it("switches the provider in use", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(withClaude));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...withClaude, active: "off" }));
    renderWithClient(<AiProvider />);
    expect(await screen.findByRole("radio", { name: "Claude" })).toHaveProperty("checked", true);
    fireEvent.click(screen.getByRole("radio", { name: "Off" }));
    await waitFor(() => { expect(sentBody(put)).toEqual({ provider: "off" }); });
    expect(await screen.findByRole("radio", { name: "Off" })).toHaveProperty("checked", true);
  });

  it("offers the server's own AI only when the server has one, and says when keys cannot be saved", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(state({ serverAvailable: true, keysSupported: false })));
    renderWithClient(<AiProvider />);
    expect(await screen.findByRole("radio", { name: "Server default" })).toHaveProperty("checked", true);
    expect(screen.getByRole("alert").textContent).toContain("HP_AI_KEYS_SECRET");
  });
});
