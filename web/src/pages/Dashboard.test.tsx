import { cleanup, fireEvent, screen } from "@testing-library/react";
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
  });
});
