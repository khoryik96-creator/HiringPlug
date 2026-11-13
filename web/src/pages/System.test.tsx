import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { megabytes, System } from "./System";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const lastBackup: NonNullable<Schemas["SystemStatus"]["backup"]> = {
  name: "hiringplug-20261107-020000", startedAt: new Date().toISOString(), finishedAt: new Date().toISOString(), status: "done", sizeBytes: 5 * 1024 * 1024, error: null,
};

const status = (over: Partial<Schemas["SystemStatus"]> = {}): Schemas["SystemStatus"] => ({
  jobs: [{ kind: "email.send", waiting: 2, running: 1, retrying: 0, failed: 1, oldestWaitingSince: "2026-11-07T06:00:00Z" }],
  failed: [{ id: "job_1", kind: "email.send", attempts: 5, error: "smtp: 550 mailbox unavailable", failedAt: "2026-11-07T05:00:00Z" }],
  backup: lastBackup, backupAuto: true,
  ...over,
});

describe("System", () => {
  it("shows what is waiting and tries a failed job again", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status()));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(status({ failed: [] })));
    renderWithClient(<System />);
    expect(await screen.findAllByText("Sending emails")).toHaveLength(2);
    expect(screen.getByText("smtp: 550 mailbox unavailable")).toBeTruthy();
    expect(screen.getByText(/Last backup .*5\.0 MB/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/settings/system/jobs/{jobId}/retry", { params: { path: { jobId: "job_1" } } }); });
    await waitFor(() => { expect(screen.queryByRole("region", { name: "Failed jobs" })).toBeNull(); });
  });

  it("warns when there is no recent backup", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ backup: null })));
    renderWithClient(<System />);
    expect(await screen.findByText(/No backup has been made yet/)).toBeTruthy();
    cleanup();
    const old = new Date(Date.now() - 3 * 24 * 3600 * 1000).toISOString();
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ backup: { ...lastBackup, startedAt: old } })));
    renderWithClient(<System />);
    expect(await screen.findByText(/more than two days ago/)).toBeTruthy();
  });

  it("does not warn about an old backup once the nightly backup is turned off", async () => {
    const old = new Date(Date.now() - 30 * 24 * 3600 * 1000).toISOString();
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ backupAuto: false, backup: { ...lastBackup, startedAt: old } })));
    renderWithClient(<System />);
    expect(await screen.findByText(/The nightly backup is turned off/)).toBeTruthy();
    expect(screen.getByText(/Last backup/)).toBeTruthy();
    expect(screen.queryByText(/more than two days ago/)).toBeNull();
  });

  it("says when the last backup failed", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ backup: { ...lastBackup, status: "failed", error: "pg_dump: disk full" } })));
    renderWithClient(<System />);
    expect((await screen.findByRole("alert")).textContent).toContain("pg_dump: disk full");
  });

  it("writes sizes people read", () => {
    expect(megabytes(2048)).toBe("2 KB");
    expect(megabytes(5 * 1024 * 1024)).toBe("5.0 MB");
  });
});
