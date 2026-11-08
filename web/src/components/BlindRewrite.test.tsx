import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { failure, ok, renderWithClient, sentBody } from "../test-utils";
import { BlindRewrite } from "./BlindRewrite";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const run = (over: Partial<Schemas["BlindRewrite"]>): Schemas["BlindRewrite"] => ({
  id: "jbr_1", adId: "ad_1", status: "done", tone: "professional", strength: "medium", error: null, warnings: [],
  retried: false, model: "deepseek-chat", tokens: 1600, createdAt: "2026-11-08T01:00:00Z", finishedAt: "2026-11-08T01:01:00Z", ...over,
});
const lists = (items: Schemas["BlindRewrite"][]) => vi.spyOn(api, "GET").mockResolvedValue(ok({ items }));

describe("rewriting a blind ad with the AI", () => {
  it("sends the tone, rewording and what else to take out", async () => {
    lists([]);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(run({ status: "queued" })));
    renderWithClient(<BlindRewrite adId="ad_1" jobId="jo_1" canEdit />);
    fireEvent.click(screen.getByRole("button", { name: "Rewrite with AI" }));
    fireEvent.click(screen.getByLabelText("Concise"));
    fireEvent.click(screen.getByLabelText("Strong"));
    fireEvent.change(screen.getByLabelText("Also take out (optional)"), { target: { value: "Project Falcon" } });
    fireEvent.click(screen.getByRole("button", { name: "Rewrite" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ tone: "concise", strength: "high", removeAlso: "Project Falcon", whyJoinUs: null });
  });

  it("shows what to check, why one failed, and that one is being made", async () => {
    lists([run({ warnings: ["Check the tech stack: Kafka from the ad may be missing."] })]);
    renderWithClient(<BlindRewrite adId="ad_1" jobId="jo_1" canEdit={false} />);
    expect((await screen.findByRole("status")).textContent).toContain("Kafka from the ad may be missing");
    expect(screen.queryByRole("button", { name: "Rewrite with AI" })).toBeNull();
    cleanup();
    lists([run({ status: "failed", error: "The AI's version still names the client, so it was not used." })]);
    renderWithClient(<BlindRewrite adId="ad_1" jobId="jo_1" canEdit />);
    expect((await screen.findByRole("alert")).textContent).toContain("still names the client");
    cleanup();
    lists([run({ status: "queued" })]);
    renderWithClient(<BlindRewrite adId="ad_1" jobId="jo_1" canEdit />);
    expect((await screen.findByRole("status")).textContent).toContain("Rewriting the blind ad");
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Rewrite with AI" }).disabled).toBe(true);
  });

  it("says when the AI is off", async () => {
    lists([]);
    vi.spyOn(api, "POST").mockResolvedValue(failure("JOB_AD_BLIND_AI_OFF", "Rewriting needs the AI: set up an AI provider in Settings.", undefined, 409));
    renderWithClient(<BlindRewrite adId="ad_1" jobId="jo_1" canEdit />);
    fireEvent.click(screen.getByRole("button", { name: "Rewrite with AI" }));
    fireEvent.click(screen.getByRole("button", { name: "Rewrite" }));
    expect(await screen.findByText(/set up an AI provider/)).toBeTruthy();
  });
});
