import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Document, type Schemas } from "../api/client";
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
const run = (over: Partial<Schemas["StudioCv"]>): Schemas["StudioCv"] => ({
  id: "scv_1", candidateId: "cand_1", sourceDocumentId: "doc_1", status: "queued", blind: false, correctLanguage: true,
  alignment: "left", documentId: null, fileName: null, error: null, warning: null, payStatementsRemoved: 0,
  createdAt: new Date(Date.now() - 12_000).toISOString(), ...over,
});

/** The server: the candidate's documents and formatting runs, as they are now. */
function serve(state: { docs: Document[]; runs: Schemas["StudioCv"][] }) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/candidates/{candidateId}/studio-cvs" ? { items: state.runs } : { items: state.docs },
  ))) as never);
}

const show = () => renderWithClient(<Documents candidate={testCandidate()} canUpload canEdit permissions={["cvformats:create"]} />);

describe("formatting a CV", () => {
  it("runs once: orange with the seconds while it runs, green Done once saved, then back to Format a CV", async () => {
    const state = { docs: [doc({})], runs: [run({})] };
    serve(state);
    show();
    const running = await screen.findByRole("button", { name: /^Formatting… \d+s$/ });
    expect(running.hasAttribute("disabled")).toBe(true); // never a second run
    expect(running.className).toContain("format-running");
    const bar = screen.getByRole("status", { name: "CV formatting" });
    expect(bar.textContent).toMatch(/Formatting….*1[2-4]s/);
    expect(screen.queryByText("Formatting history")).toBeNull();

    state.runs = [run({ status: "done", documentId: "doc_f", fileName: "Siti - CV.docx", warning: "a role may be missing" })];
    state.docs = [doc({}), doc({ id: "doc_f", source: "formatted", fileName: "Siti - CV.docx" })];
    const done = await screen.findByRole("button", { name: "Done" }, { timeout: 4000 });
    expect(done.className).toContain("format-done");
    expect(screen.getByRole("status", { name: "CV formatting" }).textContent).toContain("Saved Siti - CV.docx");
    expect(screen.getByText(/Compare with the CV before sending: a role may be missing/)).toBeTruthy();

    fireEvent.click(done);
    expect(await screen.findByRole("button", { name: "Format a CV" })).toBeTruthy();
    expect(screen.queryByRole("status", { name: "CV formatting" })).toBeNull();
  });

  it("asks before formatting again when a formatted CV is saved", async () => {
    serve({ docs: [doc({}), doc({ id: "doc_f", source: "formatted", fileName: "Siti - CV.docx" })], runs: [] });
    show();
    await screen.findByText("Siti - CV.docx");
    fireEvent.click(screen.getByRole("button", { name: "Format a CV" }));
    expect(screen.getByRole("alert").textContent).toContain("A formatted CV is saved already: Siti - CV.docx. Format again?");
    expect(screen.queryByRole("form", { name: "Format their CV" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("alert")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Format a CV" }));
    fireEvent.click(screen.getByRole("button", { name: "Format again" }));
    await waitFor(() => { expect(screen.getByRole("form", { name: "Format their CV" })).toBeTruthy(); });
  });

  it("opens the form at once when no formatted CV is saved", async () => {
    serve({ docs: [doc({})], runs: [] });
    show();
    await screen.findByText("Siti CV.pdf");
    fireEvent.click(screen.getByRole("button", { name: "Format a CV" }));
    expect(screen.getByRole("form", { name: "Format their CV" })).toBeTruthy();
  });

  it("says why a run failed", async () => {
    const state = { docs: [doc({})], runs: [run({})] };
    serve(state);
    show();
    await screen.findByRole("button", { name: /^Formatting…/ });
    state.runs = [run({ status: "failed", error: "It could not be formatted: the AI provider refused the API key" })];
    expect((await screen.findByRole("alert", {}, { timeout: 4000 })).textContent).toContain("refused the API key");
    expect(screen.getByRole("button", { name: "Format a CV" })).toBeTruthy();
  });
});
