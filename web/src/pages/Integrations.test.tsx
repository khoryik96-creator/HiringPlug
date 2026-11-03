import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { Integrations } from "./Integrations";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const reads = () =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/webhooks"
    ? { items: [], events: ["candidate.created", "application.moved"] }
    : { items: [{ id: "akey_1", name: "Payroll sync", role: "recruiter", prefix: "hpk_3f9a", createdAt: "2026-11-01T00:00:00Z", createdByName: "Aina", lastUsedAt: null }] }))) as never);

describe("integrations", () => {
  it("creates an API key and shows it once", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ key: "hpk_secretvalue", apiKey: {} }));
    renderWithClient(<Integrations />);
    expect(await screen.findByText("hpk_3f9a…")).toBeTruthy();
    const keys = screen.getByRole("region", { name: "API keys" });
    fireEvent.change(within(keys).getByLabelText("Name"), { target: { value: "Website" } });
    fireEvent.change(within(keys).getByLabelText("Acts as"), { target: { value: "viewer" } });
    fireEvent.click(within(keys).getByRole("button", { name: "Create key" }));
    expect(await screen.findByText("hpk_secretvalue")).toBeTruthy();
    expect(sentBody(post)).toEqual({ name: "Website", role: "viewer" });
  });

  it("adds a webhook for chosen events and shows its secret once", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ secret: "whsec_abc", webhook: {} }));
    renderWithClient(<Integrations />);
    const hooks = await screen.findByRole("region", { name: "Webhooks" });
    fireEvent.change(within(hooks).getByLabelText("Address"), { target: { value: "https://example.com/hooks" } });
    fireEvent.click(await within(hooks).findByLabelText("application.moved"));
    fireEvent.click(within(hooks).getByRole("button", { name: "Add webhook" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ url: "https://example.com/hooks", events: ["application.moved"] });
    expect(await screen.findByText("whsec_abc")).toBeTruthy();
  });
});
