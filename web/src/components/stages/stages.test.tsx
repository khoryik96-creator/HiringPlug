import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Application, type Schemas } from "../../api/client";
import { ok, renderWithClient, sentBody } from "../../test-utils";
import { PipelineBoard } from "../PipelineBoard";
import { columnOf, columnsFor, nextColumn, STANDARD } from "./columns";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });
beforeEach(() => { localStorage.setItem("hp.pipelineView", "board"); });

const tech: Schemas["StageStep"][] = [
  { key: "applied", label: "Applied", stage: "applied", category: "applied" }, { key: "call", label: "Phone screen", stage: "screening", category: "screening" },
  { key: "tech", label: "Tech test", stage: "interview", category: "interview" }, { key: "panel", label: "Panel", stage: "interview", category: "interview" },
  { key: "offer", label: "Offer", stage: "offer", category: "offer" }, { key: "placed", label: "Placed", stage: "placed", category: "placed" },
];
const jobId = "jo_01jbz6w0m0k8q9x2r3s4t5v6w7";
const person = (n: number, name: string, stage: Application["stage"], step: string | null = null): Application => ({
  id: `appl_01jbz6w0m0k8q9x2r3s4t5v6w${String(n)}`, version: 3, jobId, jobTitle: "Developer", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maju",
  candidateId: `cand_01jbz6w0m0k8q9x2r3s4t5v6w${String(n)}`, candidateName: name, candidateTitle: null, candidateAvailability: null, stage, step,
  reason: null, reasonNote: null, category: null, channel: null, stageChangedAt: new Date().toISOString(), createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
});
const jobStages = (over: Partial<Schemas["JobStages"]> = {}): Schemas["JobStages"] => ({
  jobId, version: 1, source: "template", templateId: "stpl_01jbz6w0m0k8q9x2r3s4t5v6w7", templateName: "Tech hiring", steps: tech, ...over,
});

describe("a job's stages as columns", () => {
  it("puts each candidate in their stage, else the first of their kind, else a column of their kind", () => {
    const panel = person(1, "Siti", "interview", "panel");
    const unsure = person(2, "Wei", "interview"); // no stage of its own yet: the first interview stage
    const submitted = person(3, "Aina", "submitted"); // the job has no submitted stage
    const columns = columnsFor(tech, [panel, unsure, submitted]);
    expect(columns.map((c) => c.label)).toEqual(["Applied", "Phone screen", "Submitted", "Tech test", "Panel", "Offer", "Placed"]);
    expect(columnOf(panel, columns)?.label).toBe("Panel");
    expect(columnOf(unsure, columns)?.label).toBe("Tech test");
    expect(columnOf(submitted, columns)?.step).toBeUndefined(); // moving there names only the stage
    expect(nextColumn(unsure, columns)?.label).toBe("Panel");
    expect(columnOf(person(4, "Lee", "rejected"), columns)).toBeUndefined();
    expect(columnsFor(STANDARD, [], false).every((c) => c.step === undefined)).toBe(true); // standard: moves as before
  });
});

describe("the board with a job's own stages", () => {
  function serve(apps: Application[], stages = jobStages()) {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/jobs/{jobId}/stages" ? stages : path === "/v1/stage-templates" ? { items: [], standard: STANDARD } : { items: apps },
    ))) as never);
  }

  it("shows the job's stages and moves people to one, naming it", async () => {
    serve([person(1, "Siti Aisyah", "interview", "tech")]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(person(1, "Siti Aisyah", "interview", "panel")));
    renderWithClient(<PipelineBoard jobId={jobId} canUpdate />);
    const testCol = await screen.findByRole("list", { name: "Tech test" });
    expect(within(testCol).getByRole("link", { name: "Siti Aisyah" })).toBeTruthy();
    expect(screen.getByRole("list", { name: "Phone screen" })).toBeTruthy();
    expect(screen.queryByRole("list", { name: "Submitted" })).toBeNull(); // not one of this job's
    fireEvent.click(within(testCol).getByRole("button", { name: "Panel: Siti Aisyah" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ version: 3, stage: "interview", step: "panel" });
    expect(screen.getByRole("button", { name: "Stages: Tech hiring" })).toBeTruthy();
  });

  it("chooses a template for the job, or changes its stages for this job only", async () => {
    serve([], jobStages({ source: "default", templateName: "Standard plus" }));
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/jobs/{jobId}/stages" ? jobStages({ source: "default", version: 0 }) :
        path === "/v1/stage-templates" ? { items: [{ id: "stpl_01jbz6w0m0k8q9x2r3s4t5v6w8", version: 1, name: "Graduates", isDefault: false, steps: tech, jobCount: 0, createdAt: "", updatedAt: "" }], standard: STANDARD } : { items: [] },
    ))) as never);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(jobStages()));
    renderWithClient(<PipelineBoard jobId={jobId} canUpdate />);
    fireEvent.click(await screen.findByRole("button", { name: /^Stages:/ }));
    const panel = screen.getByRole("region", { name: "Stages of this job" });
    const use = await within(panel).findByRole("combobox", { name: "Use" });
    await waitFor(() => { expect((use as HTMLSelectElement).disabled).toBe(false); });
    fireEvent.change(use, { target: { value: "t:stpl_01jbz6w0m0k8q9x2r3s4t5v6w8" } });
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(1); });
    expect(sentBody(put)).toEqual({ version: 0, templateId: "stpl_01jbz6w0m0k8q9x2r3s4t5v6w8" });

    fireEvent.click(within(panel).getByRole("button", { name: "Change the stages for this job" }));
    fireEvent.change(within(panel).getByRole("textbox", { name: "Stage 2 name" }), { target: { value: "Call" } });
    fireEvent.click(within(panel).getByRole("button", { name: "Remove Panel" }));
    fireEvent.click(within(panel).getByRole("button", { name: "Save for this job" }));
    await waitFor(() => { expect(put).toHaveBeenCalledTimes(2); });
    const body = sentBody(put, 1) as Schemas["JobStagesInput"];
    expect(body.steps?.map((s) => s.label)).toEqual(["Applied", "Call", "Tech test", "Offer", "Placed"]);
    expect(body.steps?.[1]?.key).toBe("call"); // renamed: still the same stage
  });

  it("only shows the stages to someone who cannot change them", async () => {
    serve([]);
    renderWithClient(<PipelineBoard jobId={jobId} canUpdate={false} />);
    fireEvent.click(await screen.findByRole("button", { name: "Stages: Tech hiring" }));
    const panel = screen.getByRole("region", { name: "Stages of this job" });
    expect(within(panel).queryByRole("combobox", { name: "Use" })).toBeNull();
    expect(within(panel).queryByRole("button", { name: "Change the stages for this job" })).toBeNull();
    expect(within(within(panel).getByRole("list", { name: "Stages" })).getAllByRole("listitem")).toHaveLength(6);
  });
});
