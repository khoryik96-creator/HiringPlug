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
    cleanup();

    renderWithClient(<Dashboard permissions={["pipeline:read"]} />);
    await screen.findByRole("region", { name: "Open jobs" });
    expect(screen.queryByRole("region", { name: "CV import" })).toBeNull();
  });
});
