import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { DeleteRecord } from "./DeleteRecord";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const check = (over: object) => ({ label: "Contoh Berhad", contacts: 0, jobs: 0, applications: 0, blocked: null, ...over });

function serve(c: object) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/bin/check"
    ? c
    : { items: [{ id: "comp_2", name: "Contoh Group", city: "Kuala Lumpur" }] }))) as never);
}

describe("deleting to the Bin", () => {
  it("says it goes to the Bin, then deletes", async () => {
    serve(check({ label: "Siti Aisyah", applications: 2 }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
    renderWithClient(<DeleteRecord kind="candidate" id="cand_1" />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    await screen.findByText("Delete Siti Aisyah?");
    const dialog = screen.getByRole("dialog", { name: "Delete candidate" });
    expect(dialog.textContent).toContain("Bin for 30 days");
    expect(dialog.textContent).toContain("2 pipeline entries are hidden with it");
    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[1] as HTMLElement);
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ kind: "candidate", id: "cand_1" });
  });

  it("moves a company's contacts and jobs to the company chosen, or deletes them with it", async () => {
    serve(check({ contacts: 3, jobs: 2 }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({}));
    renderWithClient(<DeleteRecord kind="company" id="comp_1" />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect((await screen.findByRole("group", { name: "Its 3 contacts and 2 jobs" })).textContent).toContain("Move them to another company");
    const go = screen.getByRole("button", { name: "Move them and delete" });
    expect(go.hasAttribute("disabled")).toBe(true); // choose the company first
    fireEvent.change(screen.getByPlaceholderText("Search companies"), { target: { value: "Contoh" } });
    fireEvent.click(await screen.findByText("Contoh Group"));
    fireEvent.click(screen.getByRole("button", { name: "Move them and delete" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ kind: "company", id: "comp_1", moveTo: "comp_2" });

    fireEvent.click(screen.getByLabelText(/Delete them with it/));
    fireEvent.click(screen.getAllByRole("button", { name: "Delete" })[1] as HTMLElement);
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toEqual({ kind: "company", id: "comp_1" });
  });

  it("a company whose jobs have a placement can still go once they move", async () => {
    serve(check({ contacts: 1, jobs: 2, blocked: "It has a placement: placements and their invoices are kept, so it cannot be deleted." }));
    renderWithClient(<DeleteRecord kind="company" id="comp_1" />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(await screen.findByText(/Its jobs have a placement, so they move to another company/)).toBeTruthy();
    expect(screen.queryByLabelText(/Delete them with it/)).toBeNull();
    expect(screen.getByRole("button", { name: "Move them and delete" })).toBeTruthy();
  });

  it("says why a record with a placement cannot be deleted", async () => {
    serve(check({ blocked: "It has a placement: placements and their invoices are kept, so it cannot be deleted." }));
    renderWithClient(<DeleteRecord kind="candidate" id="cand_1" />);
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect((await screen.findByRole("alert")).textContent).toContain("It has a placement");
    expect(screen.getAllByRole("button", { name: "Delete" })).toHaveLength(1);
  });
});
