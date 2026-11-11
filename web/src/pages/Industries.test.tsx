import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testIndustryCatalogue } from "../test-utils";
import { Industries } from "./Industries";
afterEach(() => { cleanup(); vi.restoreAllMocks(); });
it("adds a category with the catalogue version reviewed by the user", async () => {
 const post = vi.spyOn(api, "POST").mockResolvedValue(ok(testIndustryCatalogue()));
 renderWithClient(<Industries />);fireEvent.click(screen.getByRole("button", { name: "+ Add industry" }));
 fireEvent.change(screen.getByLabelText("Name"), { target: { value: "New sector" } });
 fireEvent.click(screen.getByRole("button", { name: "Save category" }));
 await waitFor(() => { expect(post).toHaveBeenCalled(); });
 expect(sentBody(post)).toMatchObject({ label: "New sector", catalogueVersion: 1, retired: false });
});
it("requires a replacement for used categories and preserves the retirement choice", async () => {
 const catalogue = testIndustryCatalogue();
 const parent = catalogue.items.find((i) => i.label === "Financial Services");
 if (!parent) throw new Error("missing test industry");
 parent.candidateCount = 5;
 const { client } = renderWithClient(<Industries />);client.setQueryData(["industries"], catalogue);
 await waitFor(() => { expect(screen.getByText(/5 profiles/)).toBeTruthy(); });
 const row = screen.getByText("Financial Services").closest("tr");expect(row).toBeTruthy();
 fireEvent.click(within(row as HTMLTableRowElement).getByRole("button", { name: "Delete" }));
 const dialog = await screen.findByRole("region", { name: "Delete category" });
 expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Reassign and delete" }).disabled).toBe(true);
 expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Retire" }).disabled).toBe(false);
 fireEvent.change(within(dialog).getByLabelText("Replacement"), { target: { value: "industry_education" } });
 expect(within(dialog).getByRole<HTMLButtonElement>("button", { name: "Reassign and delete" }).disabled).toBe(false);
});
