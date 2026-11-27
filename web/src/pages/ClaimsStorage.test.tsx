import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { UploadNote } from "../components/claims/UploadNote";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ClaimsStorage, examplePath } from "./ClaimsStorage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const storage = (over: Partial<Schemas["ClaimStorage"]> = {}): Schemas["ClaimStorage"] => ({
  provider: null, account: null, rootFolder: "Claims", status: "disconnected", error: null,
  callbackUrl: "http://app.test/v1/claims/storage/callback", connectedAt: null, version: 1,
  options: [
    { provider: "google_drive", ready: true, problem: null },
    { provider: "onedrive", ready: true, problem: null },
    { provider: "dropbox", ready: false, problem: "Set HP_DROPBOX_APP_KEY and HP_DROPBOX_APP_SECRET on the server." },
  ],
  uploads: { pending: 0, failed: 0, done: 0 },
  ...over,
});

const connected = (over: Partial<Schemas["ClaimStorage"]> = {}) => storage({
  provider: "onedrive", account: "finance@agency.test", status: "ok", connectedAt: "2026-11-19T02:00:00Z", version: 2,
  uploads: { pending: 0, failed: 2, done: 14 }, ...over,
});

describe("Settings › Claims storage", () => {
  it("shows where a receipt goes: month, person, receipt type", () => {
    expect(examplePath(" Finance / Claims ", new Date("2026-11-03T08:00:00Z")))
      .toBe("Finance/Claims/2026-11/Aisyah Rahman/Grab fare/2026-11-03 Grab MYR 23.00.pdf");
    expect(examplePath(" / .. ", new Date("2026-11-03T08:00:00Z"))).toMatch(/^Claims\/2026-11\//);
  });

  it("offers the providers the server can connect, and says why one cannot", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(storage()));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://login.example/authorize" }));
    const assign = vi.fn();
    vi.stubGlobal("location", { assign, hash: "#/settings/claims-storage" });
    renderWithClient(<ClaimsStorage />);
    const dropbox = await screen.findByRole<HTMLButtonElement>("button", { name: "Connect Dropbox" });
    expect(dropbox.disabled).toBe(true);
    expect(screen.getByText("Set HP_DROPBOX_APP_KEY and HP_DROPBOX_APP_SECRET on the server.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Connect OneDrive" }));
    await waitFor(() => { expect(assign).toHaveBeenCalledWith("https://login.example/authorize"); });
    expect(post).toHaveBeenCalledWith("/v1/claims/storage/authorize", { body: { provider: "onedrive" } });
  });

  it("finishes the sign-in with the ticket, and takes it out of the address", async () => {
    window.location.hash = "#/settings/claims-storage?storage=confirm&ticket=synthetic-ticket";
    vi.spyOn(api, "GET").mockResolvedValue(ok(storage()));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(connected()));
    renderWithClient(<ClaimsStorage />);
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/claims/storage/complete", { body: { ticket: "synthetic-ticket" } }); });
    expect(window.location.hash).toBe("#/settings/claims-storage");
    expect(await screen.findByText(/finance@agency\.test/)).toBeTruthy();
  });

  it("shows the connected account and its uploads, and tries the failed ones again", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(connected()));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ queued: 2 }));
    renderWithClient(<ClaimsStorage />);
    const account = await screen.findByRole("region", { name: "Account" });
    expect(within(account).getByText("OneDrive")).toBeTruthy();
    expect(within(account).getByText("Uploading")).toBeTruthy();
    expect(within(account).queryByRole("button", { name: /^Connect / })).toBeNull();
    const uploads = screen.getByRole("region", { name: "Uploads" });
    expect(within(uploads).getByRole("status").textContent).toBe("14 uploaded · 2 failed");
    fireEvent.click(within(uploads).getByRole("button", { name: "Try again" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/claims/storage/retry"); });
  });

  it("asks to connect again when the provider stopped accepting HyPlug", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(connected({ status: "reconnect", error: "OneDrive no longer accepts HyPlug's access" })));
    renderWithClient(<ClaimsStorage />);
    expect(await screen.findByText("Connect again")).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Connect OneDrive" }).disabled).toBe(false);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Try again" }).disabled).toBe(true);
  });

  it("saves the top folder with the version it was loaded at", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(connected()));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(connected({ rootFolder: "Finance/Claims", version: 3 })));
    renderWithClient(<ClaimsStorage />);
    const folder = await screen.findByLabelText("Top folder");
    fireEvent.change(folder, { target: { value: "Finance/Claims" } });
    expect(screen.getByText(/^Finance\/Claims\/\d{4}-\d{2}\/Aisyah Rahman\/Grab fare\//)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ rootFolder: "Finance/Claims", version: 2 });
  });
});

describe("a claim's upload", () => {
  it("links to the receipt in the drive, or says why it is not there", () => {
    const { rerender } = renderWithClient(<UploadNote upload={{ status: "done", path: "Claims/2026-11/A/Food/x.png", url: "https://drive.test/1", error: null, uploadedAt: null }} />);
    expect(screen.getByRole("link", { name: "In the drive" }).getAttribute("href")).toBe("https://drive.test/1");
    rerender(<UploadNote upload={{ status: "failed", path: null, url: null, error: "Dropbox is full", uploadedAt: null }} />);
    expect(screen.getByText("Not uploaded: Dropbox is full")).toBeTruthy();
    rerender(<UploadNote upload={{ status: "none", path: null, url: null, error: null, uploadedAt: null }} />);
    expect(screen.queryByText(/drive|uploaded/i)).toBeNull();
  });
});
