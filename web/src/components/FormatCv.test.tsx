import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";
import { FormatCv } from "./FormatCv";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const reads = () =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/cv-templates"
    ? { items: [{ id: "standard", name: "HiringPlug standard", builtIn: true, uploadedByName: null, createdAt: null }, { id: "ctpl_1", name: "Branded", builtIn: false, uploadedByName: "Aina", createdAt: "2026-11-01T00:00:00Z" }], placeholders: [], blocks: [] }
    : { items: [{ id: "appl_1", jobTitle: "Finance Manager", companyName: "Contoh" }] }))) as never);

const studioReads = () =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/candidates/{candidateId}/documents"
    ? { items: [{ id: "doc_o", source: "original", contentType: "application/pdf", fileName: "siti.pdf", createdAt: "2026-11-01T00:00:00Z" }] }
    : { items: [] }))) as never);

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
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ documentId: "doc_2", fileName: "Contoh CV - Candidate C-1 (Blinded).docx" }));
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
    expect(sentBody(post)).toEqual({ documentId: "doc_o", blind: true, neutralGender: true, correctLanguage: true, alignment: "justify", summary: "none" });
  });

  it("writes the About Him / Her summary as asked: length, focus, your direction and tone; never for a blind CV", async () => {
    localStorage.removeItem("hiringplug.studio.summary");
    studioReads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "scv_1", status: "queued" }));
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create"]} onDone={vi.fn()} />);
    const write = await screen.findByLabelText<HTMLInputElement>("AI summary");
    expect(write.checked).toBe(true);
    expect(screen.getByLabelText<HTMLSelectElement>("Summary length").value).toBe("normal");
    fireEvent.change(screen.getByLabelText("Summary length"), { target: { value: "detailed" } });
    fireEvent.change(screen.getByLabelText("Summary focus"), { target: { value: "leadership" } });
    fireEvent.change(screen.getByLabelText("Direction and tone"), { target: { value: "  Confident; lead with team leadership " } });
    fireEvent.click(screen.getByRole("button", { name: "Format CV" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ blind: false, summary: "detailed", summaryFocus: "leadership", summaryDirection: "Confident; lead with team leadership" });
    await waitFor(() => { expect(localStorage.getItem("hiringplug.studio.summary")).toContain("leadership"); });

    fireEvent.click(screen.getByLabelText(/Blind CV/));
    expect(screen.queryByLabelText("AI summary"), "a blind CV keeps its own summary").toBeNull();
  });

  it("remembers the choice for the next CV, and makes the summary optional", async () => {
    localStorage.setItem("hiringplug.studio.summary", JSON.stringify({ write: true, length: "short", focus: "commercial", direction: "Punchy" }));
    studioReads();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "scv_1", status: "queued" }));
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create"]} onDone={vi.fn()} />);
    expect((await screen.findByLabelText<HTMLSelectElement>("Summary length")).value).toBe("short");
    expect(screen.getByLabelText<HTMLSelectElement>("Summary focus").value).toBe("commercial");
    expect(screen.getByLabelText<HTMLInputElement>("Direction and tone").value).toBe("Punchy");

    fireEvent.click(screen.getByLabelText("AI summary"));
    expect(screen.queryByLabelText("Summary focus"), "no summary: no options").toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Format CV" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ documentId: "doc_o", blind: false, neutralGender: false, correctLanguage: true, alignment: "left", summary: "none" });
    localStorage.removeItem("hiringplug.studio.summary");
  });

  it("shows why a direction about pay is refused, next to it", async () => {
    localStorage.removeItem("hiringplug.studio.summary");
    studioReads();
    vi.spyOn(api, "POST").mockResolvedValue(invalidField("summaryDirection", "Leave pay out of the direction: the summary never mentions the candidate's pay, and pay is never sent to the AI."));
    renderWithClient(<FormatCv candidateId="cand_1" permissions={["cvformats:create"]} onDone={vi.fn()} />);
    fireEvent.change(await screen.findByLabelText("Direction and tone"), { target: { value: "mention RM 9,000" } });
    fireEvent.click(screen.getByRole("button", { name: "Format CV" }));
    expect(await screen.findByText(/Leave pay out of the direction/)).toBeTruthy();
  });
});
