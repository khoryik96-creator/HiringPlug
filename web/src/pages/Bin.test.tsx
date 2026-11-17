import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { Bin } from "./Bin";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("Settings › Bin", () => {
  it("lists what was deleted, by whom, when it goes for good, and restores", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{
      id: "bin_1", kind: "company", recordId: "comp_1", label: "Contoh Berhad", contacts: 3, jobs: 1,
      binnedByName: "Aina", binnedAt: "2026-11-17T06:00:00Z", purgeAfter: "2026-12-17T06:00:00Z",
    }] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(null));
    renderWithClient(<Bin />);
    const row = (await screen.findByText("Contoh Berhad")).closest("tr") as HTMLElement;
    expect(row.textContent).toContain("with 3 contacts and 1 job");
    expect(row.textContent).toContain("Aina");
    expect(row.textContent).toContain("17 Dec 2026");
    fireEvent.click(screen.getByRole("button", { name: "Restore" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/bin/{entryId}/restore", { params: { path: { entryId: "bin_1" } } }); });
  });
});
