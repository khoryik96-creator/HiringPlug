import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { ok, renderWithClient, sentBody, testMe } from "../../test-utils";
import { AI_WAIT_MS, AiEmailHelp } from "./AiEmailHelp";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const DRAFT = "aem_01jbz6w0m0k8q9x2r3s4t5v6w7";

function serve(enabled: boolean, written: { subject: string | null; body: string } = { subject: "Interview on Thursday", body: "Hi ⟦NAME⟧,\n\nThe role pays ⟦1⟧. Could we meet on Thursday?\n\n⟦ME⟧" }) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/assistant") return Promise.resolve(ok({ enabled, usedTokens: 0, monthlyTokens: 5000 }));
    if (path === "/v1/me") return Promise.resolve(ok({ ...testMe([]), user: { ...testMe([]).user, name: "Nurul Huda" } }));
    return Promise.resolve(ok({ id: DRAFT, status: "done", subject: written.subject, body: written.body, error: null, createdAt: "2026-11-16T00:00:00Z" }));
  }) as never);
  return vi.spyOn(api, "POST").mockResolvedValue(ok({ id: DRAFT, status: "queued", subject: null, body: null, error: null, createdAt: "2026-11-16T00:00:00Z" }));
}

describe("AI help with an email", () => {
  it("is not there while the assistant is off", async () => {
    serve(false);
    renderWithClient(<AiEmailHelp recipient="candidate" subject="" html="" names={{}} onApply={vi.fn()} />);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("button", { name: "✦ AI" })).toBeNull();
  });

  it("writes from a short ask without seeing pay or names, and puts them back", async () => {
    const post = serve(true);
    const onApply = vi.fn();
    renderWithClient(<AiEmailHelp recipient="candidate" subject="" html="" applicationId="appl_01jbz6w0m0k8q9x2r3s4t5v6w7"
      names={{ them: "Siti Aisyah" }} onApply={onApply} />);
    fireEvent.click(await screen.findByRole("button", { name: "✦ AI" }));
    expect(screen.getByRole("button", { name: "Write it for me" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.change(screen.getByLabelText("What should it say?"), { target: { value: "Invite Siti to interview Thursday; the role pays RM 9,500" } });
    await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button", { name: "Write" }).disabled).toBe(false); });
    fireEvent.click(screen.getByRole("button", { name: "Write" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    const sent = sentBody(post) as { instruction: string; mode: string; applicationId: string };
    expect(sent.mode).toBe("write");
    expect(sent.applicationId).toBe("appl_01jbz6w0m0k8q9x2r3s4t5v6w7");
    expect(sent.instruction).toBe("Invite ⟦NAME⟧ to interview Thursday; the role pays ⟦1⟧");

    const result = await screen.findByRole("region", { name: "What the AI wrote" });
    expect(result.textContent).toContain("Hi Siti,");
    expect(result.textContent).toContain("RM 9,500");
    expect(result.textContent).toContain("Nurul Huda");
    fireEvent.click(screen.getByRole("button", { name: "Use this" }));
    expect(onApply).toHaveBeenCalledWith("Interview on Thursday", expect.stringContaining("<p>Hi Siti,</p>"));
  });

  it("improves the draft in the style picked", async () => {
    const post = serve(true, { subject: null, body: "Dear ⟦NAME⟧,\n\nPlease find the CVs attached." });
    const onApply = vi.fn();
    renderWithClient(<AiEmailHelp recipient="client" subject="Shortlist" html="<p>hi Wei Tan here r the cvs</p>" names={{ them: "Wei Tan" }} onApply={onApply} />);
    fireEvent.click(await screen.findByRole("button", { name: "✦ AI" }));
    expect(screen.getByRole("button", { name: "Improve my draft" }).getAttribute("aria-pressed")).toBe("true");
    await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button", { name: "More formal" }).disabled).toBe(false); });
    fireEvent.click(screen.getByRole("button", { name: "More formal" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ mode: "improve", style: "formal", recipient: "client", subject: "Shortlist", draft: "hi ⟦1⟧ here r the cvs" }); // the whole name, back as written
    fireEvent.click(await screen.findByRole("button", { name: "Use this" }));
    expect(onApply).toHaveBeenCalledWith(null, "<p>Dear Wei,</p><p>Please find the CVs attached.</p>");
  });

  it("drafts a reply to the email pasted in", async () => {
    const post = serve(true);
    renderWithClient(<AiEmailHelp recipient="candidate" subject="" html="" names={{ them: "Siti Aisyah" }} onApply={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "✦ AI" }));
    fireEvent.click(screen.getByRole("button", { name: "Reply to their email" }));
    const draftReply = screen.getByRole<HTMLButtonElement>("button", { name: "Draft the reply" });
    expect(draftReply.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Their email"), { target: { value: "Hi, can we move it to 3pm? Call me on 012-345 6789. Siti" } });
    await waitFor(() => { expect(draftReply.disabled).toBe(false); });
    fireEvent.click(draftReply);
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ mode: "reply", theirMessage: "Hi, can we move it to 3pm? Call me on ⟦1⟧. ⟦NAME⟧" });
  });

  it("holds back the sender's name, and sends nothing until it is known", async () => {
    const post = serve(true);
    renderWithClient(<AiEmailHelp recipient="client" subject="" html="<p>Thanks, Nurul Huda</p>" names={{}} onApply={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "✦ AI" }));
    const grammar = screen.getByRole<HTMLButtonElement>("button", { name: "Check grammar" });
    await waitFor(() => { expect(grammar.disabled).toBe(false); }); // once /v1/me answered
    fireEvent.click(grammar);
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ draft: "Thanks, ⟦ME⟧" });
  });

  it("stops waiting when the AI takes too long, and says so", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      serve(true);
      const get = vi.mocked(api.GET);
      get.mockImplementation(((path: string) => {
        if (path === "/v1/assistant") return Promise.resolve(ok({ enabled: true, usedTokens: 0, monthlyTokens: 5000 }));
        if (path === "/v1/me") return Promise.resolve(ok(testMe([])));
        return Promise.resolve(ok({ id: DRAFT, status: "queued", subject: null, body: null, error: null, createdAt: "2026-11-16T00:00:00Z" }));
      }) as never);
      renderWithClient(<AiEmailHelp recipient="candidate" subject="" html="" names={{}} onApply={vi.fn()} />);
      fireEvent.click(await screen.findByRole("button", { name: "✦ AI" }));
      fireEvent.change(screen.getByLabelText("What should it say?"), { target: { value: "say hello" } });
      await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button", { name: "Write" }).disabled).toBe(false); });
      fireEvent.click(screen.getByRole("button", { name: "Write" }));
      expect(await screen.findByText("The AI is writing…")).toBeTruthy();
      await act(async () => { await vi.advanceTimersByTimeAsync(AI_WAIT_MS + 2000); });
      expect((await screen.findByRole("alert")).textContent).toContain("taking too long");
      const asked = get.mock.calls.filter((c) => (c as unknown[])[0] === "/v1/assistant/email-drafts/{draftId}").length;
      await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
      expect(get.mock.calls.filter((c) => (c as unknown[])[0] === "/v1/assistant/email-drafts/{draftId}").length, "no more asking").toBe(asked);
      expect(screen.getByRole<HTMLButtonElement>("button", { name: "Write" }).disabled, "it can be tried again").toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });
});
