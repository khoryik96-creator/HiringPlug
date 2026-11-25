import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Document } from "../api/client";
import { ok, renderWithClient, testCandidate } from "../test-utils";
import { Documents } from "./Documents";
import { pollEvery, SLOW_POLL, STALLED_AFTER } from "./ReadDetails";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const NOW = Date.parse("2026-11-25T10:00:00Z");
const doc = (over: Partial<Document>): Document => ({
  id: "doc_1", candidateId: "cand_1", source: "original", fileName: "Siti CV.pdf", contentType: "application/pdf", sizeBytes: 1000,
  sha256: "0".repeat(64), status: "extracted", pageCount: 1, extractor: "pdf", errorMessage: null, extractionWarnings: [],
  parseStatus: "parsed", parseError: null, pdfStatus: null, createdAt: "2026-11-03T00:00:00Z", updatedAt: "2026-11-03T00:00:00Z", ...over,
});

function serve(d: Document) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/candidates/{candidateId}/documents" ? { items: [d] } : d,
  ))) as never);
}

// BUG-004: a CV whose reading had stopped was checked every 2 seconds for
// ever, "Reading details… 413s", with no way to read it again.
describe("reading a CV's details", () => {
  it("checks every 2 seconds while a CV is read, for a while only", () => {
    const started = new Date(NOW - 30_000).toISOString();
    const reading = doc({ parseStatus: "queued", createdAt: "2026-11-01T00:00:00Z", updatedAt: started });
    expect(pollEvery([reading], NOW)).toBe(2000);
    expect(pollEvery([reading], NOW + STALLED_AFTER * 1000)).toBe(false);
    expect(pollEvery([doc({})], NOW)).toBe(false);
    expect(pollEvery([doc({ pdfStatus: "queued", updatedAt: started })], NOW)).toBe(2000);
    expect(pollEvery([doc({ pdfStatus: "queued", updatedAt: "2026-11-01T00:00:00Z" })], NOW)).toBe(false);
  });

  it("offers reading again once a reading has run far too long", async () => {
    const stuck = doc({ id: "doc_stuck", contentType: "text/plain", extractedText: "Stuck text", parseStatus: "queued",
      createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-24T00:00:00Z" });
    serve(stuck);
    const post = vi.spyOn(api, "POST").mockImplementation((() => Promise.resolve(ok(stuck))) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Stuck text");
    expect(screen.queryByRole("button", { name: /^Reading details…/ })).toBeNull();
    fireEvent.click(await screen.findByRole("button", { name: "Read details again" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
  });

  it("says why a CV's text could not be read, and reads it again from the file", async () => {
    const failed = doc({ id: "doc_gone", status: "failed", parseStatus: null, extractedText: null,
      errorMessage: "The file was not found on the server, so its text could not be read. Upload it again." });
    serve(failed);
    const post = vi.spyOn(api, "POST").mockImplementation((() => Promise.resolve(ok({ ...failed, status: "queued" }))) as never);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    expect((await screen.findByText(/The file was not found on the server/)).textContent).toContain("Upload it again.");
    fireEvent.click(screen.getByRole("button", { name: "Read the CV again" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/documents/{documentId}/parse", { params: { path: { documentId: "doc_gone" } } }); });
  });

  it("keeps the details read before when reading them again failed, saying so", async () => {
    serve(doc({ id: "doc_kept", contentType: "text/plain", extractedText: "Kept text",
      parseError: "The CV could not be read again, so the details read before are kept. Read it again later." }));
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    expect(await screen.findByText(/the details read before are kept/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Review details" })).toBeTruthy();
  });

  // Review of 1.47.0: after 10 minutes a reading was taken for stopped even
  // while its job still waited (a long queue, held after a restore), and
  // reading again was offered: the server knows whether a job is left.
  it("checks a long reading less often while its job is left, and stops once none is", () => {
    const started = new Date(NOW - 20 * 60_000).toISOString();
    const long = doc({ parseStatus: "queued", createdAt: "2026-11-01T00:00:00Z", updatedAt: started });
    expect(pollEvery([{ ...long, readingJob: "waiting" }], NOW)).toBe(SLOW_POLL);
    expect(pollEvery([{ ...long, readingJob: "held" }], NOW)).toBe(SLOW_POLL);
    expect(pollEvery([{ ...long, readingJob: null }], NOW)).toBe(false);
    expect(pollEvery([long], NOW)).toBe(false); // the server did not say: as before
  });

  it("says a long reading still waits instead of offering to read it again", async () => {
    const waiting = doc({ id: "doc_wait", contentType: "text/plain", extractedText: "Waiting text", parseStatus: "queued", readingJob: "held",
      createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-24T00:00:00Z" });
    serve(waiting);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Waiting text");
    expect((await screen.findAllByText(/Held after a restore of the server/)).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Read details again" })).toBeNull();
  });

  it("offers reading again at once when no job is left for it", async () => {
    const stopped = doc({ id: "doc_stop", contentType: "text/plain", extractedText: "Stopped text", parseStatus: "queued", readingJob: null,
      createdAt: "2026-11-01T00:00:00Z", updatedAt: new Date().toISOString() });
    serve(stopped);
    renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["documents:read"]} />);
    await screen.findByText("Stopped text");
    expect(await screen.findByRole("button", { name: "Read details again" })).toBeTruthy();
  });
});
