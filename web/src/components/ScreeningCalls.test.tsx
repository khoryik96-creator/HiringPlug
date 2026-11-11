import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { ScreeningCalls } from "./ScreeningCalls";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const call = (id: string, calledOn: string, calledByName: string, noteId: string | null, salaryRecorded: boolean) =>
  ({ id, calledOn, calledBy: "usr_1", calledByName, noteId, salaryRecorded, createdAt: `${calledOn}T08:00:00Z` });
const note = (id: string, body: string) =>
  ({ id, subjectType: "candidate", subjectId: "cand_1", kind: "screening", body, sensitive: false, authorId: "usr_1", authorName: "Aina", createdAt: "2026-11-08T08:00:00Z" });

describe("a candidate's screening calls", () => {
  it("lists them latest first, the latest open, with who, when and whether it set the salary", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/notes"
      ? { items: [note("note_2", "RFL: wants regional scope"), note("note_1", "RFL: restructuring")], hiddenSensitive: 0 }
      : { items: [call("scl_2", "2026-11-08", "Aina", "note_2", true), call("scl_1", "2026-10-21", "Hafiz", "note_1", false), call("scl_0", "2026-10-01", "Hafiz", null, false)] }))) as never);
    renderWithClient(<ScreeningCalls candidateId="cand_1" permissions={["candidates:read", "notes:read"]} />);
    expect(await screen.findByText("RFL: wants regional scope")).toBeTruthy();
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(3);
    expect(items[0]?.textContent).toContain("8 Nov 2026");
    expect(items[0]?.textContent).toContain("Latest");
    expect(items[0]?.textContent).toContain("Salary updated");
    expect(items[1]?.textContent).toContain("Hafiz");
    expect(items[1]?.textContent).not.toContain("Salary updated");
    expect(items[0]?.querySelector("details")?.open).toBe(true);
    expect(items[1]?.querySelector("details")?.open).toBe(false);
    expect(items[2]?.textContent).toContain("No notes were written on this call.");
  });
});

it("keeps a call's salary wording and notes separate from the current profile and hides them without salary access", async () => {
  vi.spyOn(api,"GET").mockResolvedValue(ok({items:[{...call("scl_old","2026-10-21","Hafiz",null,true),salarySnapshot:{currentSalary:{amount:"9500",currency:"MYR",period:"month",description:"9k + 500 fixed allowance"},expectedSalary:{min:"11000",max:"12000",currency:"MYR",period:"month",description:"Negotiable"},salaryNotes:"Old discussion retained"}}]}));
  const view=renderWithClient(<ScreeningCalls candidateId="cand_1" permissions={["candidates:read","candidates.compensation:read"]}/>);
  expect(await screen.findByText("9k + 500 fixed allowance")).toBeTruthy();
  expect(screen.getByText("Old discussion retained")).toBeTruthy();
  expect(screen.getByText("MYR 11000 – 12000 / month")).toBeTruthy();
  view.rerender(<ScreeningCalls candidateId="cand_1" permissions={["candidates:read"]}/>);
  expect(screen.queryByText("Old discussion retained")).toBeNull();
  expect(screen.queryByText("9k + 500 fixed allowance")).toBeNull();
});
