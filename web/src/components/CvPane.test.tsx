import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { Document } from "../api/client";
import { CvPane } from "./CvPane";
import { downloadDocument, fetchDocumentFile, fetchDocumentPdf } from "../api/files";
vi.mock("../api/files", () => ({fetchDocumentFile: vi.fn(), fetchDocumentPdf: vi.fn(), downloadDocument: vi.fn()}));
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
const doc = (id: string): Document => ({id, candidateId: "cand_1", source: "original", fileName: id + ".pdf", contentType: "application/pdf", sizeBytes: 1,
  sha256: "0".repeat(64), status: "extracted", pageCount: 1, extractor: "pdf", errorMessage: null, extractionWarnings: [], parseStatus: null, parseError: null, pdfStatus: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z"});
it("clears the old file immediately when another document is selected", async () => {
  const first = doc("doc_first"), second = doc("doc_second");
  vi.mocked(fetchDocumentFile).mockResolvedValueOnce(new Blob(["first"])).mockImplementationOnce(() => new Promise(() => {}));
  const revoke = vi.fn();
  vi.stubGlobal("URL", Object.assign(URL, {createObjectURL: () => "blob:first", revokeObjectURL: revoke}));
  const view = render(<CvPane docs={[first, second]} doc={first} selected={first.id} onSelect={vi.fn()}/>);
  await waitFor(() => { expect(screen.getByTitle("doc_first.pdf").getAttribute("src")).toBe("blob:first"); });
  view.rerender(<CvPane docs={[first, second]} doc={second} selected={second.id} onSelect={vi.fn()}/>);
  expect(screen.queryByTitle("doc_second.pdf")).toBeNull();
  expect(screen.queryByTitle("doc_first.pdf")).toBeNull();
  expect(screen.getByText("Loading…")).toBeTruthy();
  expect(revoke).toHaveBeenCalledWith("blob:first");
});
it("downloads a formatted CV as its Word file, by name, and hides the PDF viewer's own download", async () => {
  const word: Document = { ...doc("doc_word"), source: "formatted", fileName: "Hyppies CV - Siti Aminah.docx",
    contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdfStatus: "ready" };
  vi.mocked(fetchDocumentPdf).mockResolvedValueOnce(new Blob(["pdf copy"]));
  vi.stubGlobal("URL", Object.assign(URL, {createObjectURL: () => "blob:copy", revokeObjectURL: vi.fn()}));
  render(<CvPane unified docs={[word]} doc={word} selected={word.id} onSelect={vi.fn()}/>);
  await waitFor(() => { expect(screen.getByTitle(word.fileName).getAttribute("src")).toBe("blob:copy#toolbar=0&navpanes=0"); });
  fireEvent.click(screen.getByRole("button", { name: "Download the Word file, Hyppies CV - Siti Aminah.docx" }));
  expect(downloadDocument).toHaveBeenCalledWith(word);
});
it("keeps a PDF's own viewer, and offers its download too", async () => {
  const pdf = doc("doc_pdf");
  vi.mocked(fetchDocumentFile).mockResolvedValueOnce(new Blob(["pdf"]));
  vi.stubGlobal("URL", Object.assign(URL, {createObjectURL: () => "blob:pdf", revokeObjectURL: vi.fn()}));
  render(<CvPane unified docs={[pdf]} doc={pdf} selected={pdf.id} onSelect={vi.fn()}/>);
  await waitFor(() => { expect(screen.getByTitle("doc_pdf.pdf").getAttribute("src")).toBe("blob:pdf"); });
  expect(screen.getByRole("button", { name: "Download doc_pdf.pdf" })).toBeTruthy();
});
