import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import * as files from "../api/files";
import { ok, renderWithClient } from "../test-utils";
import { JobDescriptionView } from "./JobDescriptionView";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const jd = (over: Partial<Schemas["JdFile"]>): Schemas["JdFile"] => ({
  id: "jdf_1", jobId: "job_1", fileName: "Finance Manager JD.pdf", contentType: "application/pdf", sizeBytes: 1000, status: "read", error: null,
  details: null, createdAt: "2026-11-17T00:00:00Z", ...over,
});

describe("a job's description", () => {
  it("shows the uploaded file as it is, not the text pasted", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [jd({})] }));
    vi.spyOn(files, "fetchFile").mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    globalThis.URL.createObjectURL = vi.fn(() => "blob:jd");
    globalThis.URL.revokeObjectURL = vi.fn();
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect((await screen.findByTitle("Finance Manager JD.pdf")).getAttribute("src")).toContain("blob:jd");
    expect(screen.queryByText("Pasted text")).toBeNull();
  });

  it("shows a Word file as the text read from it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [jd({ fileName: "JD.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      details: { title: null, location: null, country: null, workType: null, workArrangement: null, salaryText: null, closesOn: null, description: "Lead the finance team." } })] }));
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect(await screen.findByText("Lead the finance team.")).toBeTruthy();
  });

  it("shows what was pasted when no file was uploaded", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect(await screen.findByText("Pasted text")).toBeTruthy();
  });
});
