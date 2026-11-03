import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { Assistant } from "./Assistant";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const on: Schemas["AssistantStatus"] = { enabled: true, usedTokens: 1500, monthlyTokens: 2_000_000 };
const answer = (over: Partial<Schemas["AssistantAnswer"]>): Schemas["AssistantAnswer"] => ({
  id: "ask_1", subjectType: "candidate", subjectId: "cand_1", kind: "summary", question: null, status: "done",
  answer: "An experienced accountant.", error: null, model: "deepseek-chat", createdAt: "2026-11-03T02:00:00Z", answeredAt: null, ...over,
});
const reads = (status: Schemas["AssistantStatus"], items: Schemas["AssistantAnswer"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/assistant") return Promise.resolve(ok(status));
    if (path === "/v1/candidates/{candidateId}/applications") {
      return Promise.resolve(ok({ items: [{ id: "appl_1", jobTitle: "Finance Manager", companyName: "Maju" }] }));
    }
    return Promise.resolve(ok({ items }));
  }) as never);
const USE = ["assistant:use", "pipeline:read"] as const;

describe("AI assistant", () => {
  it("is not shown, or asked anything, without the permission", () => {
    const get = reads(on, []);
    renderWithClient(<Assistant subjectType="candidate" subjectId="cand_1" permissions={["candidates:read"]} />);
    expect(screen.queryByRole("region", { name: "AI assistant" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("says when it is off", async () => {
    reads({ ...on, enabled: false }, []);
    renderWithClient(<Assistant subjectType="job" subjectId="job_1" permissions={[...USE]} />);
    expect(await screen.findByText(/The AI assistant is off/)).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Summary" })).toBeNull();
  });

  it("shows answers, asks a question, and says what it sees", async () => {
    reads(on, [answer({}), answer({ id: "ask_2", kind: "question", question: "Why did she leave?", status: "failed", answer: null, error: "the AI provider did not answer in time" })]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(answer({ id: "ask_3", status: "queued" })));
    renderWithClient(<Assistant subjectType="candidate" subjectId="cand_1" permissions={[...USE]} />);
    expect((await screen.findByRole("article", { name: "Summary" })).textContent).toContain("An experienced accountant.");
    expect(screen.getByRole("article", { name: "Why did she leave?" }).textContent).toContain("No answer: the AI provider did not answer in time");
    expect(screen.getByText(/without names, contact details or salary/).textContent).toContain("1,500 of 2,000,000 tokens");

    fireEvent.change(screen.getByLabelText("Ask a question"), { target: { value: "  What should I ask?  " } });
    fireEvent.click(screen.getByRole("button", { name: "Ask" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ subjectType: "candidate", subjectId: "cand_1", kind: "question", question: "What should I ask?" });
  });

  it("asks how well the candidate fits one of their jobs", async () => {
    reads(on, []);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(answer({ status: "queued" })));
    renderWithClient(<Assistant subjectType="candidate" subjectId="cand_1" permissions={[...USE]} />);
    expect(screen.queryByRole("button", { name: "Fit for this job" })).toBeNull();
    fireEvent.change(await screen.findByLabelText("About"), { target: { value: "appl_1" } });
    fireEvent.click(await screen.findByRole("button", { name: "Fit for this job" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ subjectType: "application", subjectId: "appl_1", kind: "fit" });
  });

  it("shows why it could not ask (this month's allowance)", async () => {
    reads(on, []);
    vi.spyOn(api, "POST").mockResolvedValue(failure("ASSISTANT_LIMIT_REACHED", "Your company has used this month's AI allowance.", undefined, 429));
    renderWithClient(<Assistant subjectType="job" subjectId="job_1" permissions={[...USE]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Summary" }));
    expect(await screen.findByText(/used this month's AI allowance/)).toBeTruthy();
  });
});
