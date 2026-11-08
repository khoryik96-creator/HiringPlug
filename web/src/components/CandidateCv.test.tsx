import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { CandidateCv } from "./CandidateCv";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const WORD = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
const doc = (id: string, contentType: string, pdfStatus: "queued" | "ready" | "failed" | null, fileName = "cv.docx") => ({
  id, candidateId: "cand_1", source: "original", fileName, contentType, sizeBytes: 10, sha256: "0".repeat(64),
  status: "extracted", pageCount: 1, extractor: "x", errorMessage: null, extractionWarnings: [], parseStatus: "parsed",
  extractedText: `text of ${fileName}`, parsedProfile: null, pdfStatus, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
});

function serve(docs: ReturnType<typeof doc>[]) {
  vi.spyOn(api, "GET").mockImplementation(((path: string, init: { params: { path: { documentId?: string } } }) => {
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: docs }));
    return Promise.resolve(ok(docs.find((d) => d.id === init.params.path.documentId)));
  }) as typeof api.GET);
  const fetchFile = vi.fn().mockResolvedValue(new Response(new Uint8Array([37, 80, 68, 70])));
  vi.stubGlobal("fetch", fetchFile);
  vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => "blob:cv", revokeObjectURL: vi.fn() }));
  return fetchFile;
}

describe("the candidate's CV", () => {
  it("shows a Word CV as its PDF copy once it is ready", async () => {
    const fetchFile = serve([doc("doc_w", WORD, "ready")]);
    renderWithClient(<CandidateCv candidateId="cand_1" />);
    const frame = await screen.findByTitle("cv.docx");
    expect(frame.getAttribute("src")).toBe("blob:cv");
    expect(fetchFile).toHaveBeenCalledWith("/v1/documents/doc_w/pdf", expect.anything());
  });

  it("shows a PDF as it is", async () => {
    const fetchFile = serve([doc("doc_p", "application/pdf", null, "cv.pdf")]);
    renderWithClient(<CandidateCv candidateId="cand_1" />);
    await screen.findByTitle("cv.pdf");
    expect(fetchFile).toHaveBeenCalledWith("/v1/documents/doc_p/file", expect.anything());
  });

  it("shows the text while the preview is made", async () => {
    serve([doc("doc_w", WORD, "queued")]);
    renderWithClient(<CandidateCv candidateId="cand_1" />);
    expect(await screen.findByText(/Making a preview of the Word file/)).toBeTruthy();
    await waitFor(() => { expect(screen.getByText("text of cv.docx")).toBeTruthy(); });
  });

  it("asks once for the preview of a Word CV that has none", async () => {
    serve([doc("doc_w", WORD, null)]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(doc("doc_w", WORD, "queued")));
    renderWithClient(<CandidateCv candidateId="cand_1" />);
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/documents/{documentId}/pdf", { params: { path: { documentId: "doc_w" } } }); });
    await screen.findByText("text of cv.docx");
    expect(post).toHaveBeenCalledTimes(1);
  });

  it("shows nothing when there is no CV", async () => {
    serve([]);
    renderWithClient(<CandidateCv candidateId="cand_1" />);
    await waitFor(() => { expect(api.GET).toHaveBeenCalled(); });
    expect(document.querySelector(".candidate-cv")).toBeNull();
  });

  it("opened from a search, shows the text with the search's words marked", async () => {
    serve([doc("doc_p", "application/pdf", null, "cv.pdf")]);
    const get = api.GET as unknown as ReturnType<typeof vi.fn>;
    const served = get.getMockImplementation() as (path: string, init: unknown) => Promise<unknown>;
    get.mockImplementation(((path: string, init: unknown) => path === "/v1/documents/{documentId}/highlights"
      ? Promise.resolve(ok({ parts: [{ text: "Led ", match: false }, { text: "SAP", match: true }, { text: " upgrades", match: false }], matches: 1 }))
      : served(path, init)) as never);
    renderWithClient(<CandidateCv candidateId="cand_1" highlight="SAP NOT intern" />);
    expect((await screen.findByText("SAP")).tagName).toBe("MARK");
    expect(screen.getByRole("status").textContent).toContain("1 place for");
    expect(get).toHaveBeenCalledWith("/v1/documents/{documentId}/highlights", { params: { path: { documentId: "doc_p" }, query: { boolean: "SAP NOT intern" } } });
    fireEvent.click(screen.getByRole("button", { name: "Stop marking" }));
    expect(await screen.findByText("text of cv.pdf")).toBeTruthy();
  });
});
