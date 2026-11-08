import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Document } from "../api/client";
import { ok, renderWithClient, testCandidate } from "../test-utils";
import { Documents } from "./Documents";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const doc = (over: Partial<Document>): Document => ({
  id: "doc_1", candidateId: "cand_1", source: "original", fileName: "Siti CV.pdf", contentType: "application/pdf", sizeBytes: 1000,
  sha256: "0".repeat(64), status: "extracted", pageCount: 1, extractor: "pdf", errorMessage: null, extractionWarnings: [],
  parseStatus: "parsed", pdfStatus: null, createdAt: "2026-11-03T00:00:00Z", updatedAt: "2026-11-03T00:00:00Z", ...over,
});

describe("CVs and documents", () => {
  it("lists their CVs before the formatted ones, each with its own actions", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      doc({ id: "doc_f", source: "formatted", fileName: "Contoh CV - Siti.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdfStatus: "ready" }),
      doc({ id: "doc_q", fileName: "Siti new.pdf", status: "queued", parseStatus: null }),
      doc({}),
    ] }));
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["cvformats:create"]} />);
    const groups = await screen.findAllByRole("heading", { level: 4 });
    expect(groups.map((g) => g.textContent)).toEqual(["From the candidate", "Formatted"]);
    const theirs = screen.getByText("Siti CV.pdf").closest("li") as HTMLElement;
    expect(within(theirs).getByText(/Their CV · added 3 Nov 2026/)).toBeTruthy();
    expect(within(theirs).getByRole("button", { name: "Review details" })).toBeTruthy();
    expect(within(screen.getByText("Siti new.pdf").closest("li") as HTMLElement).getByText(/Reading the text…/)).toBeTruthy();
    const formatted = screen.getByText("Contoh CV - Siti.docx").closest("li") as HTMLElement;
    expect(within(formatted).queryByRole("button", { name: "Review details" })).toBeNull(); // our own output
    expect(within(formatted).getAllByRole("button").map((b) => b.textContent)).toContain("PDF");
    expect(screen.getByRole("link", { name: "Compare side by side" })).toBeTruthy();
  });

  it("uploads a file as the kind chosen", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 201 }));
    renderWithClient(<Documents candidate={testCandidate({ id: "cand_1" })} canUpload canEdit={false} />);
    expect(await screen.findByText(/No CV yet/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Upload" }));
    fireEvent.click(screen.getByLabelText("LinkedIn"));
    const file = new File(["%PDF"], "Siti.pdf", { type: "application/pdf" });
    fireEvent.change(screen.getByLabelText("Drop the file here"), { target: { files: [file] } });
    await waitFor(() => { expect(fetch).toHaveBeenCalled(); });
    const [url, init] = fetch.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("/v1/candidates/cand_1/documents");
    expect((init.body as FormData).get("source")).toBe("linkedin");
    await waitFor(() => { expect(screen.getByRole("button", { name: "Upload" })).toBeTruthy(); }); // closed once uploaded
  });
});
