import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { BackupsPage, fileSize } from "./Backups";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  delete (window as unknown as { showSaveFilePicker?: unknown }).showSaveFilePicker;
});

const backups = (over: Partial<Schemas["Backups"]> = {}): Schemas["Backups"] => ({
  available: true, auto: true, serviceRunning: true, serviceSeenAt: "2026-11-12T02:01:30Z", requestedAt: null,
  last: { name: "hiringplug-20261112-020000", startedAt: "2026-11-12T02:00:00Z", finishedAt: "2026-11-12T02:01:00Z", status: "done", sizeBytes: 3_565_158, error: null },
  backups: [
    { name: "hiringplug-20261112-020000", createdAt: "2026-11-12T02:00:00Z", sizeBytes: 3_565_158, files: 120 },
    { name: "hiringplug-20261111-020000", createdAt: "2026-11-11T02:00:00Z", sizeBytes: 1_288_490_189, files: 118 },
  ],
  totalBytes: 1_292_055_347,
  ...over,
});

const link = { url: "/v1/backup-downloads/hiringplug-20261112-020000?token=abc", fileName: "hiringplug-20261112-020000.tar", sizeBytes: 3_565_158, expiresAt: "2026-11-12T10:10:00Z" };

describe("Settings › Backups", () => {
  it("writes sizes as a person reads them", () => {
    expect(fileSize(2048)).toBe("2 KB");
    expect(fileSize(3_565_158)).toBe("3.4 MB");
    expect(fileSize(1_288_490_189)).toBe("1.20 GB");
  });

  it("lists the backups with their sizes, and turns the nightly backup off", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(backups()));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(backups({ auto: false })));
    renderWithClient(<BackupsPage />);
    const list = await screen.findByRole("region", { name: "Backups on this server" });
    expect(within(list).getByText("2 backups · 1.20 GB in all")).toBeTruthy();
    expect(within(list).getByText("3.4 MB")).toBeTruthy();
    expect(within(list).getByText("1.20 GB")).toBeTruthy();
    expect(screen.getByText(/The backup service is running/)).toBeTruthy();

    const toggle = screen.getByRole("switch");
    expect((toggle as HTMLInputElement).checked).toBe(true);
    fireEvent.click(toggle);
    await waitFor(() => { expect(sentBody(put)).toEqual({ auto: false }); });
    expect(await screen.findByText(/Off: none are made unless you press Back up now/)).toBeTruthy();
  });

  it("asks for a backup now, and says when the backup service is not running", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(backups({ serviceRunning: false, serviceSeenAt: "2026-11-01T00:00:00Z" })));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(backups({ serviceRunning: false, serviceSeenAt: "2026-11-01T00:00:00Z", requestedAt: new Date().toISOString() })));
    renderWithClient(<BackupsPage />);
    expect(await screen.findByText(/The backup service is not running/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back up now" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/settings/backups/run"); });
    expect(await screen.findByText(/it is made within a minute/)).toBeTruthy();
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Back up now" }).disabled).toBe(true);
  });

  it("saves a download into the folder chosen, streaming it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(backups()));
    vi.spyOn(api, "POST").mockResolvedValue(ok(link));
    const written: number[] = [];
    const picker = vi.fn().mockResolvedValue({
      createWritable: () => Promise.resolve(new WritableStream<Uint8Array>({ write(chunk) { written.push(chunk.byteLength); } })),
    });
    (window as unknown as { showSaveFilePicker: unknown }).showSaveFilePicker = picker;
    const fetched = vi.fn().mockResolvedValue(new Response(new Uint8Array(4096)));
    vi.stubGlobal("fetch", fetched);
    renderWithClient(<BackupsPage />);
    const row = (await screen.findByText("hiringplug-20261112-020000")).closest("tr");
    if (!row) throw new Error("row");
    fireEvent.click(within(row).getByRole("button", { name: "Download" }));
    expect(await within(row).findByText("Saved to the folder you chose.")).toBeTruthy();
    expect(picker).toHaveBeenCalledWith(expect.objectContaining({ suggestedName: "hiringplug-20261112-020000.tar" }));
    expect(fetched).toHaveBeenCalledWith(link.url);
    expect(written.reduce((a, b) => a + b, 0)).toBe(4096);
  });

  it("lets other browsers save it to their downloads folder", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(backups()));
    vi.spyOn(api, "POST").mockResolvedValue(ok(link));
    const clicked: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) { clicked.push(this.getAttribute("href") ?? ""); });
    renderWithClient(<BackupsPage />);
    const row = (await screen.findByText("hiringplug-20261112-020000")).closest("tr");
    if (!row) throw new Error("row");
    fireEvent.click(within(row).getByRole("button", { name: "Download" }));
    expect(await within(row).findByText(/saving it to its downloads folder/)).toBeTruthy();
    expect(clicked).toEqual([link.url]);
  });

  it("takes the server's word for whether the backup service runs, whatever this computer's clock says", async () => {
    // Checked in "a year ago" by this computer's clock, yet the server says it is running.
    vi.spyOn(api, "GET").mockResolvedValue(ok(backups({ serviceRunning: true, serviceSeenAt: "2025-01-01T00:00:00Z" })));
    renderWithClient(<BackupsPage />);
    expect(await screen.findByText("The backup service is running.")).toBeTruthy();
    expect(screen.queryByText(/not running/)).toBeNull();
  });

  it("deletes one after asking", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(backups()));
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(backups({ backups: [], totalBytes: 0 })));
    const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
    renderWithClient(<BackupsPage />);
    const row = (await screen.findByText("hiringplug-20261111-020000")).closest("tr");
    if (!row) throw new Error("row");
    fireEvent.click(within(row).getByRole("button", { name: "Delete" }));
    expect(del).not.toHaveBeenCalled();
    fireEvent.click(within(row).getByRole("button", { name: "Delete" }));
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    expect(confirm).toHaveBeenCalledTimes(2);
    expect(await screen.findByText("No backups on this server.")).toBeTruthy();
  });

  it("shows another agency nothing", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...backups({ backups: [], totalBytes: 0, last: null }), available: false }));
    renderWithClient(<BackupsPage />);
    expect(await screen.findByText("Backups of this server are managed by the agency that runs it.")).toBeTruthy();
    expect(screen.queryByRole("switch")).toBeNull();
  });
});
