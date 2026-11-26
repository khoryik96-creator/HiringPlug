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
  offsiteStatus: "done", offsiteAt: new Date().toISOString(), offsiteError: null,
};

const status = (over: Partial<Schemas["SystemStatus"]> = {}): Schemas["SystemStatus"] => ({
  jobs: [{ kind: "email.send", waiting: 2, running: 1, retrying: 0, failed: 1, oldestWaitingSince: "2026-11-07T06:00:00Z" }],
  failed: [{ id: "job_1", kind: "email.send", attempts: 5, error: "smtp: 550 mailbox unavailable", failedAt: "2026-11-07T05:00:00Z" }],
  backup: lastBackup, backupAuto: true, held: [], heldEveryCompany: false, restore: null, server: null,
  paidAi: { paused: false, reason: null, changedAt: null, pausedByServer: false, canChange: true },
  ...over,
});

const running = (name: "worker" | "ai-worker", version = "1.52.0"): Schemas["ServerProcess"] => ({
  name, state: "running", version, startedAt: "2026-11-26T08:00:00Z", seenAt: new Date().toISOString(),
});
const server = (over: Partial<Schemas["ServerChecks"]> = {}): Schemas["ServerChecks"] => ({
  version: "1.52.0", processes: [running("worker"), running("ai-worker")], files: null,
  freeBytes: 40 * 1024 ** 3, totalBytes: 100 * 1024 ** 3, problems: [], ...over,
});

describe("System › server checks", () => {
  it("says all is well when the workers run, files work and the disk has room", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ heldEveryCompany: true, server: server() })));
    renderWithClient(<System />);
    const panel = await screen.findByRole("region", { name: "Server checks" });
    expect(panel.textContent).toContain("All is well. HiringPlug 1.52.0.");
    expect(panel.textContent).toContain("40.0 GB free of 100.0 GB.");
    expect(panel.textContent).toContain("Files can be saved and opened.");
  });

  it("shows a worker left on the old version, a files folder it cannot use and what the alerts say", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({
      heldEveryCompany: true,
      server: server({
        processes: [running("worker"), { ...running("ai-worker", "1.51.0"), state: "old_version" }],
        files: "cannot write: permission denied",
        problems: ["The AI worker still runs version 1.51.0, the rest of HiringPlug 1.52.0: restart it after the update."],
      }),
    })));
    renderWithClient(<System />);
    const panel = await screen.findByRole("region", { name: "Server checks" });
    expect(panel.textContent).toContain("Something needs looking at.");
    expect(panel.textContent).toContain("Old version");
    expect(panel.textContent).toContain("Runs 1.51.0, the rest of HiringPlug 1.52.0: restart it after the update.");
    expect(panel.textContent).toContain("Files cannot be saved or opened: cannot write: permission denied");
    expect(screen.getByText(/The AI worker still runs version 1\.51\.0/, { selector: "li" })).toBeTruthy();
  });

  it("is not shown to other companies", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status()));
    renderWithClient(<System />);
    await screen.findAllByText("Sending emails");
    expect(screen.queryByRole("region", { name: "Server checks" })).toBeNull();
  });
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

  it("says whether the last backup was also copied off the server", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok(status()));
    renderWithClient(<System />);
    expect(await screen.findByText(/Also copied off the server/)).toBeTruthy();
    cleanup();
    get.mockResolvedValue(ok(status({ backup: { ...lastBackup, offsiteStatus: "failed", offsiteError: "rclone: access denied" } })));
    renderWithClient(<System />);
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("copy off the server failed");
    expect(alert.textContent).toContain("rclone: access denied");
    cleanup();
    get.mockResolvedValue(ok(status({ backup: { ...lastBackup, offsiteStatus: "off", offsiteAt: null } })));
    renderWithClient(<System />);
    expect(await screen.findByText(/no off-site storage is set up/)).toBeTruthy();
  });

  // IMP-002: after a restore, what reaches people waits for someone.
  it("lists the jobs a restore held, to release or cancel, and says the alert emails wait", async () => {
    const held: Schemas["SystemStatus"]["held"] = [
      { id: "job_h1", company: "Acme Recruitment", kind: "email.send", attempts: 0, createdAt: "2026-11-24T23:50:00Z", heldAt: "2026-11-25T03:00:00Z", runAfter: "2026-11-24T23:50:00Z" },
      { id: "job_h2", company: "Borneo Talent", kind: "webhook.deliver", attempts: 1, createdAt: "2026-11-24T23:55:00Z", heldAt: "2026-11-25T03:00:00Z", runAfter: "2026-11-24T23:56:00Z" },
    ];
    const restore = { everyCompany: true, backupName: "hiringplug-20261125-023050", restoredAt: "2026-11-25T03:00:00Z", fromAnotherServer: true, held: 2, resumed: false, releasedAt: null };
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ held, heldEveryCompany: true, restore })));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(status({ held: held.slice(1), heldEveryCompany: true, restore })));
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderWithClient(<System />);
    const section = await screen.findByRole("region", { name: "Held after the restore" });
    expect(section.textContent).toContain("Sending webhooks to your other systems");
    expect(section.textContent).toContain("Borneo Talent"); // the agency that runs the server sees every company's
    expect(screen.getByRole("status").textContent).toContain("from a backup from another server");
    expect(screen.getByRole("status").textContent).toContain("alert emails wait");
    const [first] = screen.getAllByRole("button", { name: "Release" });
    if (first) fireEvent.click(first);
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/settings/system/jobs/{jobId}/release", { params: { path: { jobId: "job_h1" } } }); });
    fireEvent.click(await screen.findByRole("button", { name: "Cancel" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/settings/system/jobs/{jobId}/cancel", { params: { path: { jobId: "job_h2" } } }); });
    // Releasing them all may email again: asked first, like cancelling them all.
    fireEvent.click(screen.getByRole("button", { name: /^Release all/ }));
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining("may go a second time"));
    expect(post).not.toHaveBeenCalledWith("/v1/settings/system/held", { body: { action: "release" } });
    confirm.mockReturnValue(true);
    fireEvent.click(screen.getByRole("button", { name: /^Release all/ }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/settings/system/held", { body: { action: "release" } }); });
  });

  // Other companies on the server: only that some of theirs are held, and how many.
  it("tells another company only about its own held jobs", async () => {
    const held: Schemas["SystemStatus"]["held"] = [
      { id: "job_h1", company: null, kind: "email.send", attempts: 0, createdAt: "2026-11-24T23:50:00Z", heldAt: "2026-11-25T03:00:00Z", runAfter: "2026-11-24T23:50:00Z" },
    ];
    const restore = { everyCompany: false, backupName: null, restoredAt: "2026-11-25T03:00:00Z", fromAnotherServer: null, held: 1, resumed: false, releasedAt: null };
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ held, restore })));
    renderWithClient(<System />);
    const notice = await screen.findByRole("status");
    expect(notice.textContent).toContain("1 job of your company");
    expect(notice.textContent).not.toContain("hiringplug-");
    expect(notice.textContent).not.toContain("alert emails");
    expect(screen.queryByRole("columnheader", { name: "Company" })).toBeNull();
  });

  // IMP-001: the server's paid-AI switch, for the agency that runs it.
  it("pauses paid AI for the whole server, and says what still works", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status()));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const paused = { paused: true, reason: "Paused in Settings > System.", changedAt: "2026-11-25T04:00:00Z", pausedByServer: false, canChange: true };
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(status({ paidAi: paused })));
    renderWithClient(<System />);
    fireEvent.click(await screen.findByRole("button", { name: "Pause paid AI" }));
    await waitFor(() => { expect(put).toHaveBeenCalledWith("/v1/settings/system/paid-ai", { body: { paused: true } }); });
    const panel = await screen.findByRole("region", { name: "Paid AI" });
    await waitFor(() => { expect(panel.textContent).toContain("Paused on this server"); });
    expect(panel.textContent).toContain("read by the rules alone");
    expect(screen.getByRole("button", { name: "Turn paid AI back on" })).toBeTruthy();
  });

  it("shows the switch to other agencies without letting them turn it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(status({ paidAi: { paused: false, reason: null, changedAt: null, pausedByServer: false, canChange: false } })));
    renderWithClient(<System />);
    const note = await screen.findByText(/Only an owner of the agency that runs this server/);
    expect(note.textContent).toContain("HP_BACKUP_TENANT"); // who that is, and how to turn it back on without them
    expect(note.textContent).toContain("hp paid-ai on");
    expect(screen.queryByRole("button", { name: "Pause paid AI" })).toBeNull();
  });

  it("writes sizes people read", () => {
    expect(megabytes(2048)).toBe("2 KB");
    expect(megabytes(5 * 1024 * 1024)).toBe("5.0 MB");
  });
});
