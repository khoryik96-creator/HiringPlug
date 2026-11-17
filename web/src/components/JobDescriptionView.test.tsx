import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import * as files from "../api/files";
import { failure, ok, renderWithClient } from "../test-utils";
import { JobDescriptionView } from "./JobDescriptionView";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const jd = (over: Partial<Schemas["JdFile"]>): Schemas["JdFile"] => ({
  id: "jdf_1", jobId: "job_1", fileName: "Finance Manager JD.pdf", contentType: "application/pdf", sizeBytes: 1000, status: "read", error: null,
  details: null, pdfStatus: null, createdAt: "2026-11-17T00:00:00Z", ...over,
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

  it("shows a Word file as it looks (its PDF copy), not the text read from it", async () => {
    const word = { fileName: "JD.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      details: { title: null, location: null, country: null, workType: null, workArrangement: null, salaryText: null, closesOn: null, description: "Lead the finance team." } };
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [jd({ ...word, pdfStatus: "ready" })] }));
    const fetch = vi.spyOn(files, "fetchFile").mockResolvedValue(new Blob(["%PDF"], { type: "application/pdf" }));
    globalThis.URL.createObjectURL = vi.fn(() => "blob:jd");
    globalThis.URL.revokeObjectURL = vi.fn();
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect((await screen.findByTitle("JD.docx")).getAttribute("src")).toContain("blob:jd");
    expect(fetch.mock.calls[0]?.[0]).toBe("/v1/jd-files/jdf_1/pdf");
    expect(screen.queryByText("Lead the finance team.")).toBeNull();
  });

  it("asks once for the PDF copy of a Word file uploaded before previews, and falls back to its text if it fails", async () => {
    const word = { fileName: "JD.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      details: { title: null, location: null, country: null, workType: null, workArrangement: null, salaryText: null, closesOn: null, description: "Lead the finance team." } };
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [jd(word)] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(jd({ ...word, pdfStatus: "queued" })));
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect((await screen.findByRole("status")).textContent).toContain("Making a preview of the Word file");
    expect(screen.getByText("Lead the finance team."), "its text meanwhile").toBeTruthy();
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
    cleanup();
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [jd({ ...word, pdfStatus: "failed" })] }));
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect(await screen.findByText("Lead the finance team.")).toBeTruthy();
  });

  it("says why when the PDF copy cannot be asked for, and keeps the text", async () => {
    const word = { fileName: "JD.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
      details: { title: null, location: null, country: null, workType: null, workArrangement: null, salaryText: null, closesOn: null, description: "Lead the finance team." } };
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [jd(word)] }));
    vi.spyOn(api, "POST").mockResolvedValue(failure("INTERNAL", "Something went wrong", undefined, 500));
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect(await screen.findByText(/Something went wrong/)).toBeTruthy();
    expect(screen.queryByText(/Making a preview/)).toBeNull();
    expect(screen.getByText("Lead the finance team.")).toBeTruthy();
  });

  it("shows what was pasted when no file was uploaded", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    renderWithClient(<JobDescriptionView jobId="job_1" pasted="Pasted text" />);
    expect(await screen.findByText("Pasted text")).toBeTruthy();
  });
});
