import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate, testFieldSettings, testIndustryCatalogue } from "../test-utils";
import { CandidateForm } from "./CandidateForm";
import { QuickIndustries, QuickSpecialization, QuickWords } from "./ProfileQuickEdit";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

/** The API for someone who may (or may not) manage Settings, saving what is sent. */
function server(manage: boolean, posted: unknown = testIndustryCatalogue()) {
  let catalogue: unknown = testIndustryCatalogue(); // as saved last
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/settings/fields" ? ok(testFieldSettings())
      : path === "/v1/me" ? ok({ permissions: manage ? ["settings:manage", "candidates:update"] : ["candidates:update"] })
        : path === "/v1/industries" ? ok(catalogue)
          : path === "/v1/candidates/used-values" ? ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] })
          : ok({ favourites: {} }))) as never);
  const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { options: { label: string }[] } }) =>
    Promise.resolve(ok({ name: "it_skill", label: "IT skills", kind: "open", hints: false, colors: false,
      options: init.body.options.map((o) => ({ key: o.label, label: o.label, hint: null, color: null, hidden: false, builtIn: false })) }))) as never);
  const patch = vi.spyOn(api, "PATCH").mockImplementation(((_: string, init: { body: { version?: number } }) =>
    Promise.resolve(ok(init.body.version === undefined ? testIndustryCatalogue() : testCandidate({ ...init.body, version: init.body.version + 1 })))) as never);
  const post = vi.spyOn(api, "POST").mockImplementation((() => { catalogue = posted; return Promise.resolve(ok(posted)); }) as never);
  return { put, patch, post };
}
const open = async (name: string) => {
  const b = await screen.findByRole<HTMLButtonElement>("button", { name });
  await waitFor(() => { expect(b.disabled).toBe(false); });
  fireEvent.click(b);
};
const skillsList = [{ key: "Java", label: "Java", hidden: false }, { key: "Python", label: "Python", hidden: false }, { key: "SAP", label: "SAP", hidden: false }];

describe("managing a list from its dropdown", () => {
  it("lets an admin add a word typed, and gives it to the candidate", async () => {
    const { put, patch } = server(true);
    renderWithClient(<QuickWords candidate={testCandidate({ version: 4, skills: ["Java"] })} canEdit field="skills" />);
    await open("Add an IT skill");
    fireEvent.change(screen.getByLabelText("Find IT skills"), { target: { value: "  Rust " } });
    fireEvent.click(await screen.findByRole("button", { name: "+ Add “Rust” to the list" }));
    await waitFor(() => { expect(sentBody(put)).toEqual({ options: [...skillsList, { label: "Rust", hidden: false }] }); });
    await waitFor(() => { expect(sentBody(patch)).toEqual({ version: 4, skills: ["Java", "Rust"] }); });
  });

  it("lets an admin rename a word for everyone (sent under its old word)", async () => {
    const { put } = server(true);
    renderWithClient(<QuickWords candidate={testCandidate({ skills: [] })} canEdit field="skills" />);
    await open("Add an IT skill");
    fireEvent.click(await screen.findByRole("button", { name: "Rename SAP" }));
    const input = screen.getByLabelText("New name for SAP");
    fireEvent.change(input, { target: { value: "SAP S/4HANA" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(sentBody(put)).toEqual({ options: [skillsList[0], skillsList[1], { key: "SAP", label: "SAP S/4HANA", hidden: false }] });
    });
  });

  it("asks before an admin removes a word, saying candidates keep it", async () => {
    const { put } = server(true);
    renderWithClient(<QuickWords candidate={testCandidate({ skills: [] })} canEdit field="skills" />);
    await open("Add an IT skill");
    fireEvent.click(await screen.findByRole("button", { name: "Remove SAP from the list" }));
    expect(screen.getByText("Candidates who have it keep it.")).toBeTruthy();
    expect(put).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(sentBody(put)).toEqual({ options: [skillsList[0], skillsList[1]] }); });
  });

  it("offers none of it to someone who may not manage Settings", async () => {
    server(false);
    renderWithClient(<QuickWords candidate={testCandidate({ skills: [] })} canEdit field="skills" />);
    await open("Add an IT skill");
    await screen.findByRole("button", { name: "SAP" });
    expect(screen.queryByRole("button", { name: "Rename SAP" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Remove SAP from the list" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Find IT skills"), { target: { value: "Rust" } });
    expect(screen.queryByRole("button", { name: /Add “Rust”/ })).toBeNull();
    expect(screen.getByText("None match.")).toBeTruthy();
  });

  it("lets an admin rename a subindustry by its own name, and retire an industry", async () => {
    const { patch, post } = server(true);
    renderWithClient(<QuickIndustries candidate={testCandidate({ industrySelections: { catalogueVersion: 1, ids: [] } })} canEdit />);
    await open("Add an industry");
    fireEvent.click(await screen.findByRole("button", { name: "Rename Financial Services / FSI – Banking" }));
    const input = screen.getByLabelText<HTMLInputElement>("New name for Financial Services / FSI – Banking");
    expect(input.value, "its own name, not its industry's").toBe("FSI – Banking");
    fireEvent.change(input, { target: { value: "FSI – Retail Banking" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => {
      expect(sentBody(patch)).toEqual({ catalogueVersion: 1, label: "FSI – Retail Banking", parentId: "industry_bank", aliases: ["banking"], retired: false });
    });
    fireEvent.click(screen.getByRole("button", { name: "Remove Education from the list" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => { expect(sentBody(post)).toEqual({ catalogueVersion: 1, action: "retire" }); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/industries/{industryId}/remove");
  });

  it("renames with Enter without submitting a form the dropdown sits in", async () => {
    const { put } = server(true);
    const submitted = vi.fn((e: { preventDefault: () => void }) => { e.preventDefault(); });
    renderWithClient(<form onSubmit={submitted}><QuickWords candidate={testCandidate({ skills: [] })} canEdit field="skills" /></form>);
    await open("Add an IT skill");
    fireEvent.click(await screen.findByRole("button", { name: "Rename SAP" }));
    const input = screen.getByLabelText("New name for SAP");
    fireEvent.change(input, { target: { value: "SAP S/4HANA" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(submitted).not.toHaveBeenCalled();
  });

  it("offers only adding in Edit profile, where a rename would leave the open form behind", async () => {
    server(true);
    renderWithClient(<CandidateForm candidate={testCandidate({ skills: [] })} onDone={vi.fn()} />);
    await open("IT skills");
    await screen.findByRole("button", { name: "SAP" });
    expect(screen.queryByRole("button", { name: "Rename SAP" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Remove SAP from the list" })).toBeNull();
    fireEvent.change(screen.getByLabelText("Find IT skills"), { target: { value: "Rust" } });
    expect(await screen.findByRole("button", { name: "+ Add “Rust” to the list" })).toBeTruthy();
  });

  it("does not offer to rename a word that is only on this candidate", async () => {
    server(true);
    renderWithClient(<QuickSpecialization candidate={testCandidate({ specialization: "Underwater Basketry" })} canEdit />);
    await open("Specialization: Underwater Basketry. Change");
    await screen.findByRole("button", { name: "Cybersecurity" });
    expect(screen.getByRole("button", { name: "Rename Cybersecurity" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Rename Underwater Basketry" }), "not in the list").toBeNull();
  });

  it("chooses an industry just added with the catalogue as it is now", async () => {
    const catalogue = testIndustryCatalogue();
    const added = { ...catalogue, version: 2, items: [...catalogue.items, { id: "industry_aviation", label: "Aviation", aliases: [], retired: false, candidateCount: 0, companyCount: 0 }] };
    const { patch } = server(true, added);
    renderWithClient(<QuickIndustries candidate={testCandidate({ version: 4, industrySelections: { catalogueVersion: 1, ids: [] } })} canEdit />);
    await open("Add an industry");
    fireEvent.change(screen.getByLabelText("Find industries"), { target: { value: "Aviation" } });
    fireEvent.click(await screen.findByRole("button", { name: "+ Add “Aviation” to the list" }));
    await waitFor(() => {
      expect(sentBody(patch)).toEqual({ version: 4, industrySelections: { catalogueVersion: 2, ids: ["industry_aviation"], primaryId: "industry_aviation" }, industry: "Aviation" });
    });
  });

  it("does not offer to add an industry or subindustry there already, by any of its names", async () => {
    server(true);
    renderWithClient(<QuickIndustries candidate={testCandidate({ industrySelections: { catalogueVersion: 1, ids: [] } })} canEdit />);
    await open("Add an industry");
    for (const name of ["FSI – Banking", "banking", "education"]) {
      fireEvent.change(screen.getByLabelText("Find industries"), { target: { value: name } });
      expect(screen.queryByRole("button", { name: /^\+ Add/ }), name).toBeNull();
    }
  });
});
