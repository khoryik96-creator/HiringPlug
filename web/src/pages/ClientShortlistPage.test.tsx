import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ClientShortlistPage, fetchCv, shortlistRoute, showsPdfInPage } from "./ClientShortlistPage";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const TOKEN = "b".repeat(43);
const item = (over: Record<string, unknown> = {}) => ({
  id: "cslc_01jbz6w0m0k8q9x2r3s4t5v6w7", label: "Candidate C-4F2K9Q", blind: true, note: "Strong on month-end close.",
  cvFileName: "Candidate C-4F2K9Q CV.pdf", decision: null, comment: null, answeredAt: null, ...over,
});
const open = {
  agencyName: "Hyppies", jobTitle: "Finance Manager", recruiterName: "Dewi", message: "Two for the finance role.",
  status: "open", expiresAt: "2026-12-06T02:00:00Z",
  candidates: [item(), item({ id: "cslc_01jbz6w0m0k8q9x2r3s4t5v6w8", label: "Wei Tan", blind: false, note: null, cvFileName: "Wei Tan CV.pdf" })],
} as const;

describe("client shortlist page", () => {
  it("is found at /shortlist/<token> and #/shortlist/<token> only", () => {
    expect(shortlistRoute({ pathname: `/shortlist/${TOKEN}`, hash: "" })).toBe(TOKEN);
    expect(shortlistRoute({ pathname: "/", hash: `#/shortlist/${TOKEN}` })).toBe(TOKEN);
    expect(shortlistRoute({ pathname: "/shortlist/short", hash: "" })).toBeNull();
    expect(shortlistRoute({ pathname: `/book/${TOKEN}`, hash: "" })).toBeNull();
  });

  it("shows the candidates and sends the client's answer with the comment", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(open));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({
      ...open, candidates: [item({ decision: "approved", comment: "Let's meet.", answeredAt: "2026-11-23T02:00:00Z" }), open.candidates[1]],
    }));
    renderWithClient(<ClientShortlistPage token={TOKEN} />);
    expect(await screen.findByText("Shortlist: Finance Manager")).toBeTruthy();
    expect(screen.getByText("Two for the finance role.")).toBeTruthy();
    expect(screen.getByText(/0 of 2 answered/)).toBeTruthy();
    const first = screen.getByRole("listitem", { name: "Candidate C-4F2K9Q" });
    expect(within(first).getByText("Blind CV")).toBeTruthy();
    expect(within(first).getByText("Strong on month-end close.")).toBeTruthy();
    fireEvent.change(within(first).getByRole("textbox"), { target: { value: "Let's meet." } });
    fireEvent.click(within(first).getByRole("button", { name: "Approve for interview" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/public/client-shortlists/{shortlistToken}/candidates/{shortlistCandidateId}/answer");
    expect(sentBody(post)).toEqual({ decision: "approved", comment: "Let's meet." });
    expect(await screen.findByText("✓ Approved")).toBeTruthy();
    expect(screen.getByText("“Let's meet.”")).toBeTruthy();
    expect(screen.getByText(/1 of 2 answered/)).toBeTruthy();
  });

  it("says when the link no longer works", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...open, status: "revoked", message: null, candidates: [] }));
    renderWithClient(<ClientShortlistPage token={TOKEN} />);
    expect(await screen.findByText(/This link was withdrawn. Ask Dewi/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Approve/ })).toBeNull();
  });

  it("fetches the CV under the label's name", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(new Uint8Array([37, 80, 68, 70]), {
      status: 200, headers: { "Content-Disposition": "attachment; filename=\"Candidate C-4F2K9Q CV.pdf\"" },
    }));
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => "blob:cv" }));
    const copy = await fetchCv(TOKEN, item());
    expect(fetchMock).toHaveBeenCalledWith(`/v1/public/client-shortlists/${TOKEN}/candidates/${item().id}/cv`);
    expect(copy).toEqual({ url: "blob:cv", fileName: "Candidate C-4F2K9Q CV.pdf", pdf: true });
    fetchMock.mockResolvedValue(new Response(null, { status: 409 }));
    await expect(fetchCv(TOKEN, item())).rejects.toThrow("This link no longer works.");
  });

  /** A phone (touch screen, no PDF viewer in the page) or a computer. */
  const asDevice = (phone: boolean) => {
    vi.stubGlobal("matchMedia", (q: string) => ({ matches: phone && q === "(pointer: coarse)", media: q, addEventListener: () => undefined, removeEventListener: () => undefined }));
    Object.defineProperty(navigator, "pdfViewerEnabled", { configurable: true, value: !phone });
  };
  const cvResponse = () => new Response(new Uint8Array([37, 80, 68, 70]), {
    status: 200, headers: { "Content-Disposition": "attachment; filename=\"Candidate C-4F2K9Q CV.pdf\"" },
  });

  it("knows a phone shows no PDF in the page", () => {
    asDevice(true);
    expect(showsPdfInPage()).toBe(false);
    asDevice(false);
    expect(showsPdfInPage()).toBe(true);
  });

  it("on a phone opens the CV in a tab of its own, not in the page (where phones block it)", async () => {
    asDevice(true);
    vi.spyOn(api, "GET").mockResolvedValue(ok(open));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(cvResponse()));
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => "blob:cv", revokeObjectURL: () => undefined }));
    const tab = { opener: {} as unknown, location: { href: "" }, close: vi.fn() };
    const openTab = vi.spyOn(window, "open").mockReturnValue(tab as unknown as Window);
    renderWithClient(<ClientShortlistPage token={TOKEN} />);
    const first = await screen.findByRole("listitem", { name: "Candidate C-4F2K9Q" });
    fireEvent.click(within(first).getByRole("button", { name: "View CV" }));
    expect(openTab).toHaveBeenCalledWith("", "_blank"); // at once, while it is the click's: no pop-up blocker
    expect(tab.opener).toBeNull();
    await waitFor(() => { expect(tab.location.href).toBe("blob:cv"); });
    expect(document.querySelector("iframe")).toBeNull();
  });

  it("on a computer shows the CV in the page", async () => {
    asDevice(false);
    vi.spyOn(api, "GET").mockResolvedValue(ok(open));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(cvResponse()));
    vi.stubGlobal("URL", Object.assign(URL, { createObjectURL: () => "blob:cv", revokeObjectURL: () => undefined }));
    const openTab = vi.spyOn(window, "open");
    renderWithClient(<ClientShortlistPage token={TOKEN} />);
    const first = await screen.findByRole("listitem", { name: "Candidate C-4F2K9Q" });
    fireEvent.click(within(first).getByRole("button", { name: "View CV" }));
    expect(await screen.findByTitle("CV of Candidate C-4F2K9Q")).toBeTruthy();
    expect(openTab).not.toHaveBeenCalled();
  });
});
