import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { FormatCv } from "./FormatCv";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const reads = () =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/cv-templates"
    ? { items: [{ id: "standard", name: "HiringPlug standard", builtIn: true, uploadedByName: null, createdAt: null }, { id: "ctpl_1", name: "Branded", builtIn: false, uploadedByName: "Aina", createdAt: "2026-11-01T00:00:00Z" }], placeholders: [], blocks: [] }
    : { items: [{ id: "appl_1", jobTitle: "Finance Manager", companyName: "Contoh" }] }))) as never);

describe("formatted CVs", () => {
  it("makes one for a job, contact hidden unless asked", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ documentId: "doc_1", fileName: "Siti - CV.docx" }));
    const onDone = vi.fn();
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create", "pipeline:read"]} onDone={onDone} />);
    fireEvent.change(await screen.findByLabelText("For"), { target: { value: "appl_1" } });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "ctpl_1" } });
    expect(screen.queryByLabelText(/Include current and expected salary/)).toBeNull(); // only with salary permission
    fireEvent.click(screen.getByRole("button", { name: "Make formatted CV" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ templateId: "ctpl_1", blind: false, showContact: false, includeSalary: false, applicationId: "appl_1" });
  });

  it("offers salary to those who may see it", async () => {
    reads();
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create", "candidates.compensation:read"]} onDone={vi.fn()} />);
    expect(await screen.findByLabelText(/Include current and expected salary/)).toBeTruthy();
  });

  it("a blind CV never shows contact details", async () => {
    reads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ documentId: "doc_2", fileName: "Candidate C-1 - Blind CV.docx" }));
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create"]} onDone={vi.fn()} />);
    fireEvent.click(await screen.findByLabelText("Show email and phone"));
    fireEvent.click(screen.getByLabelText(/Blind CV/));
    expect(screen.queryByLabelText("Show email and phone")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Make formatted CV" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ blind: true, showContact: false });
  });

  it("formats one of their own CVs CV Studio's way, never one the app made", async () => {
    const doc = (id: string, source: string, contentType: string, fileName: string) => ({ id, source, contentType, fileName, createdAt: "2026-11-01T00:00:00Z" });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/candidates/{candidateId}/documents"
      ? { items: [doc("doc_f", "formatted", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "Siti - CV.docx"),
        doc("doc_o", "original", "application/pdf", "siti.pdf")] }
      : { items: [] }))) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "scv_1", status: "queued" }));
    const onDone = vi.fn();
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create"]} onDone={onDone} />);
    const cv = await screen.findByLabelText<HTMLSelectElement>("CV");
    expect(Array.from(cv.options).map((o) => o.value)).toEqual(["doc_o"]);
    expect(screen.getByRole("radio", { name: /From their CV/ })).toHaveProperty("checked", true);
    fireEvent.click(screen.getByLabelText(/Blind CV/));
    fireEvent.click(screen.getByLabelText(/instead of he or she/));
    fireEvent.change(screen.getByLabelText("Paragraphs"), { target: { value: "justify" } });
    fireEvent.click(screen.getByRole("button", { name: "Format CV" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/candidates/{candidateId}/studio-cvs");
    expect(sentBody(post)).toEqual({ documentId: "doc_o", blind: true, neutralGender: true, correctLanguage: true, alignment: "justify" });
  });
});
