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
  parseStatus: "parsed", parseError: null, pdfStatus: null, createdAt: "2026-11-03T00:00:00Z", updatedAt: "2026-11-03T00:00:00Z", ...over,
});

/** The server: the candidate's documents, as a list and one by one (the viewer reads the one shown). */
function serveDocs(items: Document[]) {
  vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { path?: { documentId?: string } } }) => Promise.resolve(ok(
    path === "/v1/documents/{documentId}" ? items.find((d) => d.id === init?.params?.path?.documentId) ?? items[0] : { items },
  ))) as never);
}

describe("CVs and documents", () => {
  it("shares one selected file between the library, preview and file actions", async () => {
    const first = doc({id: "doc_first", fileName: "Original CV.txt", contentType: "text/plain", extractedText: "Original text"});
    const formatted = doc({id: "doc_formatted", fileName: "Formatted CV.txt", source: "formatted", contentType: "text/plain", extractedText: "Formatted text"});
    vi.spyOn(api, "GET").mockImplementation(((path: string, init: {params: {path: {documentId: string}}}) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? {items: [first, formatted]} : init.params.path.documentId === first.id ? first : formatted
    ))) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read", "documents:delete"]}/>);
    const library = await screen.findByRole("navigation", {name: "Candidate files"});
    expect(await screen.findByText("Original text")).toBeTruthy();
    expect(screen.queryByRole("combobox", {name: "CV version"})).toBeNull();
    // No Download button beside the viewer: a file the viewer cannot download comes from the menu.
    expect(screen.queryByRole("button", {name: "Download"})).toBeNull();
    fireEvent.click(screen.getByLabelText("More for Original CV.txt"));
    expect(screen.getByRole("menuitem", {name: "Download the file"})).toBeTruthy();
    fireEvent.click(within(library).getByRole("button", {name: /Formatted CV.txt/}));
    expect(await screen.findByText("Formatted text")).toBeTruthy();
    expect(screen.queryByText("Original text")).toBeNull();
    expect(screen.queryByRole("button", {name: "Review details"})).toBeNull();
    fireEvent.click(screen.getByLabelText("More for Formatted CV.txt"));
    expect(screen.getByRole("menuitem", {name: "Delete this formatted CV"})).toBeTruthy();
  });

  it("has nothing to download beside a PDF: the viewer has its own download", async () => {
    const pdf = doc({ id: "doc_pdf", extractedText: "PDF text" });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [pdf] } : pdf
    ))) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByRole("button", { name: "Review details" });
    expect(screen.queryByRole("button", { name: "Download" })).toBeNull();
    expect(screen.queryByLabelText("More for Siti CV.pdf")).toBeNull(); // an empty menu is not shown
  });

  it("opens the review beside the CV, and reads it again from there, offering the new details", async () => {
    let parseStatus: Document["parseStatus"] = "parsed";
    let name = "Noelle Khoo";
    const cv = () => doc({ id: "doc_cv", fileName: "Noelle CV.txt", contentType: "text/plain", extractedText: "CV text", parseStatus,
      updatedAt: new Date().toISOString(),
      parsedProfile: { parser: "rules@1", fullName: { value: name, quote: name.toUpperCase() }, skills: [], experience: [], education: [], dropped: 0 } as never });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [cv()] } : cv()
    ))) as never);
    vi.spyOn(api, "POST").mockImplementation((() => { parseStatus = "queued"; return Promise.resolve(ok(cv())); }) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("CV text");
    fireEvent.click(screen.getByRole("button", { name: "Review details" }));
    const review = await screen.findByRole("region", { name: "Details from this CV" });
    // Beside the CV, in place of the list of files.
    const workspace = document.querySelector(".cv-workspace.reviewing") as HTMLElement;
    expect(workspace.contains(review)).toBe(true);
    expect(within(workspace).getByText("CV text")).toBeTruthy();
    expect(screen.queryByRole("navigation", { name: "Candidate files" })).toBeNull();
    expect(screen.getByRole("button", { name: "Close review" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Read this CV again" }));
    expect((await screen.findByRole("status", { name: "Reading the CV" })).textContent).toContain("Reading the details…");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: /^Reading details…/ }).disabled).toBe(true);
    expect(screen.queryByRole("region", { name: "Details from this CV" })).toBeNull();
    expect(within(review).getByText("Noelle Khoo")).toBeTruthy();
    name = "Noelle Khoo Jia Yi"; // the reader did better this time
    parseStatus = "parsed";
    // Read: the review asked for opens by itself, with the new details (not the copy cached before).
    const again = await screen.findByRole("region", { name: "Details from this CV" }, { timeout: 4000 });
    expect(await within(again).findByText("Noelle Khoo Jia Yi")).toBeTruthy();
    expect(within(again).queryByText("Noelle Khoo")).toBeNull();
  });

  it("reads a CV never read with the same button, then shows it ready", async () => {
    let parseStatus: Document["parseStatus"] = null;
    const cv = () => doc({ id: "doc_old", fileName: "Old CV.txt", contentType: "text/plain", extractedText: "Old text", parseStatus,
      parsedProfile: { parser: "rules@1", fullName: { value: "Ali", quote: "ALI" }, skills: [], experience: [], education: [], dropped: 0 } as never });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [cv()] } : cv()
    ))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation((() => { parseStatus = "queued"; return Promise.resolve(ok(cv())); }) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Old text");
    expect(screen.queryByRole("button", { name: "Review details" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Read details" }));
    await screen.findByRole("button", { name: /^Reading details…/ });
    expect(post).toHaveBeenCalledTimes(1);
    parseStatus = "parsed";
    expect(await screen.findByRole("region", { name: "Details from this CV" }, { timeout: 4000 })).toBeTruthy();
  });

  it("says so when the details asked for could not be read, until dismissed", async () => {
    let parseStatus: Document["parseStatus"] = null;
    const cv = () => doc({ id: "doc_bad", fileName: "Bad CV.txt", contentType: "text/plain", extractedText: "Bad text", parseStatus,
      parseError: parseStatus === "failed" ? "The details could not be read from this CV (IndexError at experience.py:120)." : null });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [cv()] } : cv()
    ))) as never);
    vi.spyOn(api, "POST").mockImplementation((() => { parseStatus = "queued"; return Promise.resolve(ok(cv())); }) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Bad text");
    fireEvent.click(screen.getByRole("button", { name: "Read details" }));
    await screen.findByRole("button", { name: /^Reading details…/ });
    parseStatus = "failed";
    const bar = await screen.findByRole("status", { name: "Reading the CV" }, { timeout: 4000 });
    await waitFor(() => { expect(bar.textContent).toContain("The details could not be read"); }, { timeout: 4000 });
    expect(screen.getByRole("alert").textContent).toContain("(IndexError at experience.py:120)"); // where it broke, to be fixed
    expect(screen.queryByRole("region", { name: "Details from this CV" })).toBeNull();
    fireEvent.click(within(bar).getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByRole("status", { name: "Reading the CV" })).toBeNull();
    expect(screen.getByRole("button", { name: "Read details again" })).toBeTruthy();
  });

  it("reads a formatted CV again only for comparing: from the menu, never offering its review", async () => {
    let parseStatus: Document["parseStatus"] = "parsed";
    const f = () => doc({ id: "doc_f", source: "formatted", fileName: "Formatted CV.txt", contentType: "text/plain", extractedText: "Formatted text", parseStatus,
      parsedProfile: { parser: "rules@1", fullName: { value: "Siti", quote: "SITI" }, skills: [], experience: [], education: [], dropped: 0 } as never });
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [f()] } : f()
    ))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation((() => { parseStatus = "queued"; return Promise.resolve(ok(f())); }) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Formatted text");
    expect(screen.queryByRole("button", { name: /details/ })).toBeNull();
    fireEvent.click(screen.getByLabelText("More for Formatted CV.txt"));
    fireEvent.click(screen.getByRole("menuitem", { name: "Read details again (for comparing)" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
    parseStatus = "parsed";
    const bar = await screen.findByRole("status", { name: "Reading the CV" });
    await waitFor(() => { expect(bar.textContent).toContain("Details read from Formatted CV.txt"); }, { timeout: 4000 });
    expect(within(bar).queryByRole("button", { name: "Review details" })).toBeNull(); // our own output
    expect(screen.queryByRole("region", { name: "Details from this CV" })).toBeNull();
  });

  it("compares two CVs side by side right in the tab, and goes back to one", async () => {
    const a = doc({ id: "doc_a", fileName: "Their CV.txt", contentType: "text/plain", extractedText: "Their text" });
    const b = doc({ id: "doc_b", fileName: "Formatted CV.txt", source: "formatted", contentType: "text/plain", extractedText: "Formatted text" });
    vi.spyOn(api, "GET").mockImplementation(((path: string, init: { params: { path: { documentId: string } } }) => Promise.resolve(ok(
      path === "/v1/candidates/{candidateId}/documents" ? { items: [a, b] } : init.params.path.documentId === a.id ? a : b
    ))) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
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

  it("lists their CVs before the formatted ones in the library, each with what it is", async () => {
    serveDocs([
      doc({ id: "doc_f", source: "formatted", fileName: "Contoh CV - Siti.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdfStatus: "failed" }),
      doc({ id: "doc_q", fileName: "Siti new.txt", contentType: "text/plain", status: "queued", parseStatus: null, createdAt: "2026-11-04T00:00:00Z" }),
      doc({ fileName: "Siti CV.txt", contentType: "text/plain" }),
    ]);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["cvformats:create"]} />);
    const library = await screen.findByRole("navigation", { name: "Candidate files" });
    expect(within(library).getAllByRole("heading", { level: 4 }).map((g) => g.textContent)).toEqual(["From the candidate", "Formatted"]);
    expect(within(library).getByRole("button", { name: /Siti CV\.txt/ }).textContent).toContain("Their CV · 3 Nov 2026 · Text read · Details read");
    expect(within(library).getByRole("button", { name: /Siti new\.txt/ }).textContent).toContain("Reading the text…");
    fireEvent.click(within(library).getByRole("button", { name: /Siti CV\.txt/ }));
    expect(await screen.findByRole("button", { name: "Review details" })).toBeTruthy();
    fireEvent.click(within(library).getByRole("button", { name: /Contoh CV - Siti\.docx/ }));
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Review details" })).toBeNull(); }); // our own output
    expect(screen.getByRole("button", { name: "Compare side by side" })).toBeTruthy();
  });

  it("says a file is too large when the server in front answers with a page, not the API's error", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [] }));
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("<html>413 Request Entity Too Large</html>", { status: 413, headers: { "Content-Type": "text/html" } }));
    renderWithClient(<Documents candidate={testCandidate({ id: "cand_1" })} canUpload canEdit={false} />);
    expect(await screen.findByText(/No CV yet/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Upload" }));
    fireEvent.change(screen.getByLabelText("Drop the file here"), { target: { files: [new File(["%PDF"], "Big.pdf", { type: "application/pdf" })] } });
    expect(await screen.findByText("The file is too large to upload.")).toBeTruthy();
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
    // Their own CV a PDF (the viewer's own download, so no menu); the formatted one a Word file whose preview failed.
    serveDocs([doc({ id: "doc_f", source: "formatted", fileName: "Contoh CV - Siti.docx", contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdfStatus: "failed" }), doc({})]);
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(undefined));
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:delete"]} />);
    const library = await screen.findByRole("navigation", { name: "Candidate files" });
    // Their own PDF: nothing to delete (and the viewer downloads it), so no menu at all.
    await screen.findByRole("button", { name: "Review details" });
    expect(screen.queryByLabelText("More for Siti CV.pdf")).toBeNull();
    fireEvent.click(within(library).getByRole("button", { name: /Contoh CV - Siti\.docx/ }));
    fireEvent.click(await screen.findByLabelText("More for Contoh CV - Siti.docx"));
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete this formatted CV" }));
    await waitFor(() => { expect(del).toHaveBeenCalledWith("/v1/documents/{documentId}", { params: { path: { documentId: "doc_f" } } }); });
  });
});
