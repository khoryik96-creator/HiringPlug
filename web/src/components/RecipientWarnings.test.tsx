import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { RecipientWarnings } from "./RecipientWarnings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("recipients that would not be reached", () => {
  it("are named before sending: who left, and whose mailbox bounces", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [
      { email: "ana@maju.test", contactId: "cont_ana", contactName: "Ana Lim", companyName: "Maju", problem: "left" },
      { email: "gone@maju.test", contactId: "cont_gone", contactName: "Gone", companyName: "Maju", problem: "bounces" },
    ] }));
    renderWithClient(<RecipientWarnings text="Ana@Maju.test, gone@maju.test; wei@maju.test not-an-address" />);
    await waitFor(() => { expect(screen.getByRole("status").textContent).toContain("Ana Lim (ana@maju.test) has left Maju: this email will not reach them."); });
    expect(screen.getByRole("status").textContent).toContain("Gone (gone@maju.test) will bounce");
    expect((post.mock.calls as unknown[][])[0]?.[1]).toEqual({ body: { emails: ["ana@maju.test", "gone@maju.test", "wei@maju.test"] } });
  });

  it("say nothing when everyone is fine, or there is no address yet", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ items: [] }));
    const { rerender } = renderWithClient(<RecipientWarnings text="" />);
    expect(post).not.toHaveBeenCalled();
    rerender(<RecipientWarnings text="wei@maju.test" />);
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(screen.queryByRole("status")).toBeNull();
  });
});
