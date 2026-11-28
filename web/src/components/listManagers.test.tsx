import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate, testFieldSettings, testIndustryCatalogue } from "../test-utils";
import { QuickIndustries, QuickWords } from "./ProfileQuickEdit";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

/** The API for someone who may (or may not) manage Settings, saving what is sent. */
function server(manage: boolean) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/settings/fields" ? ok(testFieldSettings())
      : path === "/v1/me" ? ok({ permissions: manage ? ["settings:manage", "candidates:update"] : ["candidates:update"] })
        : path === "/v1/industries" ? ok(testIndustryCatalogue())
          : ok({ favourites: {} }))) as never);
  const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { options: { label: string }[] } }) =>
    Promise.resolve(ok({ name: "it_skill", label: "IT skills", kind: "open", hints: false, colors: false,
      options: init.body.options.map((o) => ({ key: o.label, label: o.label, hint: null, color: null, hidden: false, builtIn: false })) }))) as never);
  const patch = vi.spyOn(api, "PATCH").mockImplementation(((_: string, init: { body: { version?: number } }) =>
    Promise.resolve(ok(init.body.version === undefined ? testIndustryCatalogue() : testCandidate({ ...init.body, version: init.body.version + 1 })))) as never);
  const post = vi.spyOn(api, "POST").mockImplementation((() => Promise.resolve(ok(testIndustryCatalogue()))) as never);
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
});
