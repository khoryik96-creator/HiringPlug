import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as companion from "../companion";
import { activeRuns, newRun } from "./findHrRun";
import { serve, show } from "./leadFindHRFixtures";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// The panel's own scan stops with it; one from the company's page goes on.
describe("a lead's Find HR panel and the scan", () => {
  it("keeps a scan started on the company's own page going when the panel closes", async () => {
    serve();
    vi.spyOn(companion, "companionCall").mockResolvedValue({});
    const run = newRun(vi.fn());
    activeRuns.set("comp_1", run);
    try {
      show({ companyId: "comp_1" });
      expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
      cleanup();
      expect(run.stopped).toBe(false);
    } finally {
      activeRuns.delete("comp_1");
    }
  });

  it("stops a scan started in the panel when the panel closes", async () => {
    serve();
    const call = vi.spyOn(companion, "companionCall").mockResolvedValue({});
    const run = newRun(vi.fn());
    try {
      show({ companyId: "comp_1" });
      expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
      activeRuns.set("comp_1", run); // started from the panel
      expect(run.stopped).toBe(false);
      cleanup();
      expect(run.stopped).toBe(true);
      expect(call).toHaveBeenCalledWith("cancel", { operation_id: run.operation }, 5_000);
    } finally {
      activeRuns.delete("comp_1");
    }
  });
});
