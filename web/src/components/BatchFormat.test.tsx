import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { BatchFormat } from "./BatchFormat";

afterEach(() => { cleanup(); vi.restoreAllMocks(); localStorage.removeItem("hiringplug.studio.summary"); });

const people = [{ candidateId: "cand_1", name: "Siti Aminah" }, { candidateId: "cand_2", name: "Wei Ming" }, { candidateId: "cand_3", name: "Nur Huda" }];
const run = (candidateId: string) => ({ id: `scv_${candidateId}`, candidateId, sourceDocumentId: "doc_1", status: "queued", blind: true, correctLanguage: true,
  alignment: "left", documentId: null, fileName: null, error: null, warning: null, payStatementsRemoved: 0, createdAt: "2026-11-23T00:00:00Z" });

describe("BatchFormat", () => {
  it("formats everyone chosen with the same options, then says who was left out and why", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [
      { candidateId: "cand_1", started: true, studioCv: run("cand_1") },
      { candidateId: "cand_2", started: true, studioCv: run("cand_2") },
      { candidateId: "cand_3", started: false, reason: "has no PDF or Word CV they sent" },
    ] }));
    renderWithClient(<BatchFormat people={people} onClose={vi.fn()} />);
    expect(screen.getByText("Siti Aminah, Wei Ming, Nur Huda")).toBeTruthy();
    fireEvent.click(screen.getByLabelText(/Blind CVs/));
    fireEvent.click(screen.getByLabelText(/instead of he or she/));
    fireEvent.change(screen.getByLabelText("Summary length"), { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: "Format 3 CVs" }));
    await screen.findByRole("heading", { name: "2 CVs are being formatted" });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/studio-cvs/batch");
    expect(sentBody(post)).toEqual({ candidateIds: ["cand_1", "cand_2", "cand_3"], blind: true, neutralGender: true, correctLanguage: true,
      alignment: "left", summary: "short", summaryFocus: "general" });
    const skipped = screen.getByRole("link", { name: "Nur Huda" });
    expect(skipped.getAttribute("href")).toBe("#/candidates/cand_3");
    expect(skipped.closest("li")?.textContent).toContain("has no PDF or Word CV they sent");
  });

  it("says when the AI is not set up, and never sends more than 20", async () => {
    vi.spyOn(api, "POST").mockResolvedValue(failure("STUDIO_CV_OFF", "CV Studio formatting needs the AI: set up an AI provider in Settings.", undefined, 409));
    renderWithClient(<BatchFormat people={people} onClose={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Format 3 CVs" }));
    expect(await screen.findByText(/needs the AI/)).toBeTruthy();
    cleanup();

    const many = Array.from({ length: 21 }, (_, i) => ({ candidateId: `cand_${String(i)}`, name: `Person ${String(i)}` }));
    renderWithClient(<BatchFormat people={many} onClose={vi.fn()} />);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Format 21 CVs" }).disabled).toBe(true);
    expect(screen.getByText("At most 20 at a time.")).toBeTruthy();
  });
});

describe("BatchFormat with nobody chosen", () => {
  it("cannot be sent", () => {
    const post = vi.spyOn(api, "POST");
    renderWithClient(<BatchFormat people={[]} onClose={vi.fn()} />);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Format 0 CVs" }).disabled).toBe(true);
    expect(screen.getByText("Choose the candidates again: none are chosen now.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });
});
