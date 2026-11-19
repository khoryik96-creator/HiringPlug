import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { ok, renderWithClient, testFieldSettings } from "../../test-utils";
import type { Fact } from "./claims";
import { ClaimsDashboard } from "./ClaimsDashboard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const fact = (over: Partial<Fact>): Fact => ({
  status: "submitted", receiptType: "food", claimType: "client_entertainment", linkKind: null, linkId: null, userId: "usr_a",
  month: "2026-11", currency: "MYR", count: 1, amount: "10.00", ...over,
});

it("adds up the team's claims into tiles and breakdowns", async () => {
  const get = vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/settings/fields" ? testFieldSettings() : ok({
      facts: [
        fact({ amount: "40.00", count: 2 }),
        fact({ status: "approved", receiptType: "grab_fare", claimType: "travel", amount: "60.00", userId: "usr_b", linkKind: "company", linkId: "comp_1" }),
        fact({ status: "paid", receiptType: "drinks", amount: "15.50" }),
      ],
      people: [{ id: "usr_a", name: "Aisyah" }, { id: "usr_b", name: "Ben" }],
      records: [{ kind: "company", id: "comp_1", name: "Acme Berhad", detail: null }],
    }),
  ));
  renderWithClient(<ClaimsDashboard canSeeTeam />);
  const waiting = await screen.findByText("Waiting for approval");
  expect(waiting.parentElement?.textContent).toContain("MYR 40.00");
  expect(waiting.parentElement?.textContent).toContain("2 claims");
  const byType = screen.getByRole("region", { name: "By receipt type" });
  expect(within(byType).getAllByRole("rowheader").map((r) => r.textContent)).toEqual(["Grab / e-hailing fare", "Food", "Drinks"]);
  const byPerson = screen.getByRole("region", { name: "By person" });
  expect(within(byPerson).getAllByRole("rowheader").map((r) => r.textContent)).toEqual(["Ben", "Aisyah"]);
  expect(within(screen.getByRole("region", { name: "By client or candidate" })).getByText("Acme Berhad")).toBeTruthy();
  expect(get).toHaveBeenCalledWith("/v1/claims/team/summary", expect.anything());

  fireEvent.click(screen.getByRole("button", { name: "Mine" }));
  await screen.findByText("Drafts");
  expect(get).toHaveBeenCalledWith("/v1/claims/summary", expect.anything());
  expect(screen.queryByRole("region", { name: "By person" })).toBeNull();
});
