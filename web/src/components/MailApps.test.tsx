import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { EmailConnection } from "../pages/EmailConnection";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const status: Schemas["MailboxStatus"] = { storageReady: true, microsoftReady: false, googleReady: true, serverMode: "test", connected: false, version: 0 };
const apps = (over: Partial<Schemas["MailApps"]> = {}): Schemas["MailApps"] => ({
  storageReady: true, callbackUrl: "http://localhost:3000/v1/mailbox/callback",
  microsoft: { source: "none", clientId: null, secretEnding: null, updatedAt: null },
  google: { source: "server", clientId: "server-google", secretEnding: null, updatedAt: null },
  ...over,
});
const reads = (a: Schemas["MailApps"]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/settings/email-apps" ? a : status))) as never);

it("lets owners and admins enter the Microsoft app; the secret is never shown again", async () => {
  reads(apps());
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(apps({ microsoft: { source: "agency", clientId: "my-app-id", secretEnding: "9f2a", updatedAt: "2026-11-13T00:00:00Z" } })));
  renderWithClient(<EmailConnection canManageApps />);
  const ms = await screen.findByRole("form", { name: "Microsoft app" });
  expect(within(ms).getByText("Not set up")).toBeTruthy();
  expect(within(screen.getByRole("form", { name: "Google app" })).getByText("Using the server's settings")).toBeTruthy();
  expect(screen.getByText("http://localhost:3000/v1/mailbox/callback")).toBeTruthy();

  fireEvent.change(within(ms).getByLabelText("Microsoft client ID"), { target: { value: " my-app-id " } });
  fireEvent.change(within(ms).getByLabelText("Microsoft client secret"), { target: { value: "s3cret-9f2a" } });
  fireEvent.click(within(ms).getByRole("button", { name: "Save" }));
  await waitFor(() => { expect(sentBody(put)).toEqual({ clientId: "my-app-id", clientSecret: "s3cret-9f2a" }); });
  const saved = await screen.findByRole("form", { name: "Microsoft app" });
  expect(await within(saved).findByText("Saved · secret ending 9f2a")).toBeTruthy();
  expect(within(saved).getByLabelText<HTMLInputElement>("Microsoft client secret").value, "the secret is not shown").toBe("");
  expect(within(saved).getByLabelText<HTMLInputElement>("Microsoft client secret").type).toBe("password");
});

it("keeps the saved secret when only the client ID changes", async () => {
  reads(apps({ microsoft: { source: "agency", clientId: "old-id", secretEnding: "9f2a", updatedAt: "2026-11-13T00:00:00Z" } }));
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(apps()));
  renderWithClient(<EmailConnection canManageApps />);
  const ms = await screen.findByRole("form", { name: "Microsoft app" });
  expect(within(ms).getByLabelText<HTMLInputElement>("Microsoft client secret").placeholder).toContain("9f2a");
  fireEvent.change(within(ms).getByLabelText("Microsoft client ID"), { target: { value: "new-id" } });
  fireEvent.click(within(ms).getByRole("button", { name: "Save" }));
  await waitFor(() => { expect(sentBody(put)).toEqual({ clientId: "new-id" }); });
});

it("shows the apps only to those who may manage them", async () => {
  const get = reads(apps());
  renderWithClient(<EmailConnection />);
  expect(await screen.findByRole("region", { name: "Connect email" })).toBeTruthy();
  expect(screen.queryByRole("region", { name: "Mail apps" })).toBeNull();
  expect(get.mock.calls.some((c) => (c as unknown[])[0] === "/v1/settings/email-apps")).toBe(false);
  expect(screen.getByText(/once an owner or admin sets up their apps/)).toBeTruthy();
});
