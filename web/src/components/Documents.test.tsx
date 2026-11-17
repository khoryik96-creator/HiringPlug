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
  it("shares one selected file between the library, preview and file actions", async () => {
    const first = doc({id: "doc_first", fileName: "Original CV.txt", contentType: "text/plain", extractedText: "Original text"});
    const formatted = doc({id: "doc_formatted", fileName: "Formatted CV.txt", source: "formatted", contentType: "text/plain", extractedText: "Formatted text"});
    vi.spyOn(api, "GET").mockImplementation(((path: string, init: {params: {path: {documentId: string}}}) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? {items: [first, formatted]} : init.params.path.documentId === first.id ? first : formatted
    ))) as never);
    renderWithClient(<Documents workspace candidate={testCandidate()} canUpload canEdit permissions={["documents:read", "documents:delete"]}/>);
    const library = await screen.findByRole("navigation", {name: "Candidate files"});
    expect(await screen.findByText("Original text")).toBeTruthy();
    expect(screen.queryByRole("combobox", {name: "CV version"})).toBeNull();
    expect(screen.getAllByRole("button", {name: "Download"})).toHaveLength(1);
    fireEvent.click(within(library).getByRole("button", {name: /Formatted CV.txt/}));
    expect(await screen.findByText("Formatted text")).toBeTruthy();
    expect(screen.queryByText("Original text")).toBeNull();
    expect(screen.queryByRole("button", {name: "Review details"})).toBeNull();
    fireEvent.click(screen.getByLabelText("More for Formatted CV.txt"));
    expect(screen.getByRole("menuitem", {name: "Delete this formatted CV"})).toBeTruthy();
  });

  it("opens Review details above the CV, and says what Read details again is doing", async () => {
    let parseStatus: Document["parseStatus"] = "parsed";
    const cv = () => doc({ id: "doc_cv", fileName: "Noelle CV.txt", contentType: "text/plain", extractedText: "CV text", parseStatus,
      parsedProfile: { parser: "rules@1", fullName: { value: "Noelle Khoo", quote: "NOELLE KHOO" }, skills: [], experience: [], education: [], dropped: 0 } as never });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [cv()] } : cv()
    ))) as never);
    vi.spyOn(api, "POST").mockImplementation((() => { parseStatus = "queued"; return Promise.resolve(ok(cv())); }) as never);
    renderWithClient(<Documents workspace candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("CV text");
    fireEvent.click(screen.getByRole("button", { name: "Review details" }));
    const review = await screen.findByRole("region", { name: "Details from this CV" });
    const viewer = document.querySelector(".cv-workspace") as HTMLElement;
    // Before the tall viewer, where the button is: it used to open below it, out of sight.
    expect(review.compareDocumentPosition(viewer) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    fireEvent.click(screen.getByLabelText("More for Noelle CV.txt"));
    fireEvent.click(screen.getByRole("menuitem", { name: "Read details again" }));
    expect((await screen.findByRole("status")).textContent).toContain("Reading the details from Noelle CV.txt…");
    expect(screen.queryByRole("region", { name: "Details from this CV" })).toBeNull();
    parseStatus = "parsed";
    await waitFor(() => { expect(screen.getByRole("status").textContent).toContain("Details read from Noelle CV.txt."); }, { timeout: 4000 });
    fireEvent.click(within(screen.getByRole("status")).getByRole("button", { name: "Review details" }));
    expect(await screen.findByRole("region", { name: "Details from this CV" })).toBeTruthy();
  });

  it("compares two CVs side by side right in the tab, and goes back to one", async () => {
    const a = doc({ id: "doc_a", fileName: "Their CV.txt", contentType: "text/plain", extractedText: "Their text" });
    const b = doc({ id: "doc_b", fileName: "Formatted CV.txt", source: "formatted", contentType: "text/plain", extractedText: "Formatted text" });
    vi.spyOn(api, "GET").mockImplementation(((path: string, init: { params: { path: { documentId: string } } }) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [a, b] } : init.params.path.documentId === a.id ? a : b
    ))) as never);
    renderWithClient(<Documents workspace candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Their text");
    // A button in the tab, not a link to another page.
    expect(screen.queryByRole("link", { name: "Compare side by side" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Compare side by side" }));
    const both = await screen.findByRole("region", { name: "CVs side by side" });
    expect(await within(both).findByText("Their text")).toBeTruthy();
    expect(await within(both).findByText("Formatted text")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Back to one CV" }));
    expect(screen.queryByRole("region", { name: "CVs side by side" })).toBeNull();
    expect(screen.getByRole("navigation", { name: "Candidate files" })).toBeTruthy();
  });

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

  it("skips exactly a CV they have, and asks about one with the same name", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    const existing = { documentId: "doc_1", candidateId: "cand_1", candidateName: "Siti", fileName: "Siti.pdf", createdAt: "2026-11-03T02:00:00Z" };
    const post = vi.spyOn(api, "POST")
      .mockResolvedValueOnce(ok({ items: [{ fileName: "copy.pdf", sha256: "a", sameContent: existing, sameName: [] }] }))
      .mockResolvedValueOnce(ok({ items: [{ fileName: "Siti.pdf", sha256: "b", sameContent: null, sameName: [existing] }] }));
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 201 }));
    renderWithClient(<Documents candidate={testCandidate({ id: "cand_1" })} canUpload canEdit={false} />);
    await screen.findByText(/No CV yet/);
    fireEvent.click(screen.getByRole("button", { name: "Upload" }));

    fireEvent.change(screen.getByLabelText("Drop the file here"), { target: { files: [new File(["%PDF one"], "copy.pdf")] } });
    expect((await screen.findByRole("status")).textContent).toContain("Skipped: copy.pdf is exactly Siti's CV (Siti.pdf, added 3 Nov 2026)");
    expect(post.mock.calls[0]?.[1]).toMatchObject({ body: { candidateId: "cand_1" } });
    expect(fetch).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText("Drop the file here"), { target: { files: [new File(["%PDF two"], "Siti.pdf")] } });
    expect((await screen.findByRole("alert")).textContent).toContain("already has a CV with this name");
    expect(fetch).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Upload anyway" }));
    await waitFor(() => { expect(fetch).toHaveBeenCalledTimes(1); });
  });

  it("deletes a formatted CV after asking, never the candidate's own", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [doc({ id: "doc_f", source: "formatted", fileName: "Contoh CV - Siti.docx" }), doc({})] }));
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(undefined));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:delete"]} />);
    const own = (await screen.findByText("Siti CV.pdf")).closest("li") as HTMLElement;
    fireEvent.click(within(own).getByLabelText(/More for/));
    expect(within(own).queryByRole("menuitem", { name: /Delete/ })).toBeNull();
    const formatted = screen.getByText("Contoh CV - Siti.docx").closest("li") as HTMLElement;
    fireEvent.click(within(formatted).getByLabelText(/More for/));
    fireEvent.click(within(formatted).getByRole("menuitem", { name: "Delete this formatted CV" }));
    await waitFor(() => { expect(del).toHaveBeenCalledWith("/v1/documents/{documentId}", { params: { path: { documentId: "doc_f" } } }); });
  });
});
