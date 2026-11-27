import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { Dashboard } from "./Dashboard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const summary = { openJobs: 0, stageCounts: { applied: 0, screening: 0, submitted: 0, interview: 0, offer: 0, placed: 0 }, placedLast30Days: 0, movedLast7Days: 0, jobs: [], stuck: [] };

describe("dashboard", () => {
  it("imports profiles (with the permission), loads past imports only when asked, and has no recent moves", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/pipeline/summary" ? summary : path === "/v1/settings/fields" ? testFieldSettings() : { items: [] }))) as never);
    renderWithClient(<Dashboard permissions={["intake:create", "pipeline:read"]} />);
    expect(await screen.findByRole("region", { name: "CV import" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Recent moves" })).toBeNull();
    expect(get).not.toHaveBeenCalledWith("/v1/cv-imports");
    fireEvent.click(screen.getByRole("button", { name: "Show recent imports" }));
    expect(await screen.findByText("No imports yet.")).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/cv-imports");
    fireEvent.click(screen.getByRole("button", { name: "Hide recent imports" }));
    expect(screen.queryByText("No imports yet.")).toBeNull();
    cleanup();

    renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
    await screen.findByRole("region", { name: "Open jobs" });
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
  });

  it("opens a job at its stage from a number in the open jobs, and the jobs from Open jobs", async () => {
    const job = { jobId: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", title: "Finance Manager", companyName: "Maju", status: "open",
      stageCounts: { applied: 0, screening: 3, submitted: 1, interview: 0, offer: 0, placed: 0 } };
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/pipeline/summary"
      ? { ...summary, openJobs: 1, jobs: [job] } : path === "/v1/settings/fields" ? testFieldSettings() : { items: [] }))) as never);
    renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
    const three = await screen.findByRole("link", { name: "3 in Screening for Finance Manager" });
    expect(three.getAttribute("href")).toBe("#/jobs/jo_01jbz6w0m0k8q9x2r3s4t5v6w7?stage=screening");
    expect(screen.getByRole("link", { name: "1 in Submitted for Finance Manager" }).getAttribute("href")).toBe("#/jobs/jo_01jbz6w0m0k8q9x2r3s4t5v6w7?stage=submitted");
    expect(screen.getByRole("link", { name: /^Open jobs/ }).getAttribute("href")).toBe("#/jobs");
    const company = screen.getByText("Maju");
    expect(company.className, "the company sits on its own line under the job title").toContain("job-company");
    expect(company.previousElementSibling?.textContent).toBe("Finance Manager");
  });

  it("lets owners and admins choose whose numbers: someone, several people, or the whole team", async () => {
    localStorage.removeItem("hyplug.dashboard.people");
    const done = { calls: 1, cvsSent: 2, interviews: 0, placements: 0, categories: {} };
    const activity = { period: "week", from: "2026-11-23", to: "2026-11-29", team: true, rows: [
      { userId: "usr_me", userName: "Demo owner", done, target: null },
      { userId: "usr_aina", userName: "Aina", done: { ...done, calls: 5 }, target: { calls: 10, cvsSent: null, interviews: null, placements: null } },
      { userId: "usr_wei", userName: "Wei", done: { ...done, calls: 3 }, target: { calls: 10, cvsSent: null, interviews: null, placements: null } },
    ] };
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/pipeline/summary" ? summary
      : path === "/v1/reports/activity" ? activity : path === "/v1/settings/fields" ? testFieldSettings() : { items: [] }))) as never);
    renderWithClient(<Dashboard permissions={["pipeline:read", "reports:read", "reports.team:read"]} meId="usr_me" />);
    const picker = await screen.findByRole("button", { name: /Showing Me/ });
    expect(await screen.findByRole("region", { name: "Your activity" })).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/pipeline/summary", { params: { query: { owner: ["usr_me"] } } });

    fireEvent.click(picker);
    fireEvent.click(screen.getByRole("checkbox", { name: "Demo owner (me)" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Aina" }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Wei" }));
    expect(screen.getByRole("button", { name: /Showing Aina, Wei/ })).toBeTruthy();
    const both = await screen.findByRole("region", { name: "Activity: Aina, Wei" });
    expect(await within(both).findByRole("row", { name: /Together/ })).toBeTruthy();
    expect(within(both).getByText("8 of 20"), "their calls and targets added up").toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/pipeline/summary", { params: { query: { owner: ["usr_aina", "usr_wei"] } } });
    expect(screen.getByRole("heading", { name: "Open jobs: Aina, Wei" })).toBeTruthy();

    fireEvent.click(screen.getByRole("checkbox", { name: "Whole team" }));
    const all = await screen.findByRole("region", { name: "Activity: Whole team" });
    expect(within(all).getByText("9"), "everyone's calls together").toBeTruthy();
    expect(get).toHaveBeenLastCalledWith("/v1/pipeline/summary", { params: { query: {} } });
    expect(JSON.parse(localStorage.getItem("hyplug.dashboard.people") ?? "null"), "remembered").toBe("team");
  });
});
