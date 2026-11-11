import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { IndustryPicker, type IndustryChoices } from "./IndustryPicker";
afterEach(() => { cleanup(); vi.restoreAllMocks(); });
function Picker({ candidate }: { candidate?: { id: string; version: number; currentCompany: string } }) {
  const [value, set] = useState<IndustryChoices>({ catalogueVersion: 1, ids: ["industry_education"], primaryId: "industry_education" });
  return <><IndustryPicker value={value} legacy="Education" candidate={candidate} onChange={(v) => { set(v); }} /><output aria-label="Selection">{JSON.stringify(value)}</output></>;
}
describe("industry hierarchy", () => {
  it("supports broad-only selections, subindustries and search aliases while preserving earlier tags", () => {
    renderWithClient(<Picker />);
    fireEvent.click(screen.getByRole("button", { name: /Financial Services/ }));
    fireEvent.click(screen.getByLabelText("FSI – Banking"));
    expect(screen.getByLabelText("Selection").textContent).toContain("industry_banking");
    expect(screen.getByLabelText("Selection").textContent).toContain("industry_education");
    fireEvent.change(screen.getByLabelText("Search industries"), { target: { value: "SAP" } });
    expect(screen.getByLabelText("Tech – ERP/CRM/HCM/Application")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Tech – ERP/CRM/HCM/Application"));
    expect(screen.getByLabelText("Selection").textContent).toContain("industry_erp");
  });
  it("allows manual choices when recognition fails", async () => {
    vi.spyOn(api, "GET").mockRejectedValue(new Error("Recognition unavailable"));
    renderWithClient(<Picker candidate={{ id: "candidate", version: 3, currentCompany: "Unknown" }} />);
    expect(await screen.findByText("Recognition unavailable")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Financial Services/ }));
    fireEvent.click(screen.getByLabelText("FSI – Banking"));
    expect(screen.getByLabelText("Selection").textContent).toContain("industry_banking");
  });
  it("only offers a suggestion matching the open candidate and catalogue versions", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ catalogueVersion: 1, candidateVersion: 4, status: "ready", categoryId: "industry_banking", reason: "Verified employer", employers: [] }));
    renderWithClient(<Picker candidate={{ id: "candidate", version: 3, currentCompany: "Bank" }} />);
    await waitFor(() => { expect(screen.getByText("Verified employer")).toBeTruthy(); });
    expect(screen.queryByRole("button", { name: "Add as current" })).toBeNull();
  });
});
