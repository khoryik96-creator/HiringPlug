import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
import { QuickIndustries, QuickSpecialization, QuickWords } from "./ProfileQuickEdit";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function server() {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields"
    ? ok(testFieldSettings()) : ok({ favourites: { itSkill: ["SAP"] } }))) as never);
  // As the server does: the change saved, one version on.
  return vi.spyOn(api, "PATCH").mockImplementation(((_: string, init: { body: { version: number } }) =>
    Promise.resolve(ok(testCandidate({ ...init.body, version: init.body.version + 1 })))));
}
const open = async (name: string) => {
  const b = await screen.findByRole<HTMLButtonElement>("button", { name });
  await waitFor(() => { expect(b.disabled).toBe(false); });
  fireEvent.click(b);
};

describe("editing a profile's lists in place", () => {
  it("changes the specialization at once, and says it is saved", async () => {
    const patch = server();
    renderWithClient(<QuickSpecialization candidate={testCandidate({ version: 4, specialization: null })} canEdit />);
    await open("Specialization: not set. Change");
    fireEvent.click(await screen.findByRole("button", { name: "Cybersecurity" }));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ version: 4, specialization: "Cybersecurity" }); });
    expect(await screen.findByText("✓ Saved")).toBeTruthy();
  });

  it("adds an IT skill from the list (favourites first) and removes one", async () => {
    const patch = server();
    const { rerender } = renderWithClient(<QuickWords candidate={testCandidate({ version: 4, skills: ["Java"] })} canEdit field="skills" />);
    await open("Add an IT skill");
    expect(await screen.findByRole("list", { name: "Your favourites" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Java" }), "one already there is not offered").toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "SAP" }));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ version: 4, skills: ["Java", "SAP"] }); });
    rerender(<QuickWords candidate={testCandidate({ version: 5, skills: ["Java", "SAP"] })} canEdit field="skills" />);
    fireEvent.click(screen.getByRole("button", { name: "Remove Java" }));
    await waitFor(() => { expect(sentBody(patch, 1)).toEqual({ version: 5, skills: ["SAP"] }); });
  });

  it("adds an industry found by typing, the first one becoming the current one", async () => {
    const patch = server();
    renderWithClient(<QuickIndustries candidate={testCandidate({ version: 4, industrySelections: { catalogueVersion: 1, ids: [] } })} canEdit />);
    await open("Add an industry");
    fireEvent.change(screen.getByLabelText("Find industries"), { target: { value: "bank" } });
    fireEvent.click(screen.getByRole("button", { name: "Financial Services / FSI – Banking" }));
    await waitFor(() => {
      expect(sentBody(patch)).toEqual({ version: 4, industrySelections: { catalogueVersion: 1, ids: ["industry_banking"], primaryId: "industry_banking" }, industry: "Financial Services" });
    });
  });

  it("only shows them to someone who may not change the profile", () => {
    server();
    renderWithClient(<QuickWords candidate={testCandidate({ certifications: ["ACCA"] })} canEdit={false} field="certifications" />);
    expect(screen.getByText("ACCA")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});

describe("two quick changes in a row", () => {
  it("sends the second from the version the first made, not a stale one", async () => {
    const patch = server();
    const c = testCandidate({ id: "cand_01jbz6w0m0k8q9x2r3s4t5v6q1", version: 4, specialization: null, skills: [] });
    renderWithClient(<><QuickSpecialization candidate={c} canEdit /><QuickWords candidate={c} canEdit field="skills" /></>);
    await open("Specialization: not set. Change");
    fireEvent.click(await screen.findByRole("button", { name: "Cybersecurity" }));
    await open("Add an IT skill");
    fireEvent.click(screen.getByRole("button", { name: "Python" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 0)).toEqual({ version: 4, specialization: "Cybersecurity" });
    expect(sentBody(patch, 1), "both made before the page showed version 5").toEqual({ version: 5, skills: ["Python"] });
  });
});
