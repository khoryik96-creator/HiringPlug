import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { candidateKey } from "../api/keys";
import { api, type Candidate, type Document, type Permission } from "../api/client";
import { latestCv } from "../components/CvPane";
import { failure, ok, RECRUITER, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
import { ScreeningCall } from "./ScreeningCall";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
});

const doc = (id: string, source: Document["source"], createdAt: string): Document => ({
  id, candidateId: "cand_1", source, fileName: `${id}.docx`, contentType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
  sizeBytes: 10, sha256: "0".repeat(64), status: "extracted", pageCount: 1, extractor: "x", errorMessage: null, extractionWarnings: [],
  parseStatus: null, extractedText: `text of ${id}`, parsedProfile: null, pdfStatus: null, createdAt, updatedAt: createdAt,
});
const docs = [doc("doc_old", "original", "2026-10-01T00:00:00Z"), doc("doc_new", "linkedin", "2026-11-01T00:00:00Z"), doc("doc_fmt", "formatted", "2026-11-02T00:00:00Z")];
const CALLER: Permission[] = [...RECRUITER, "notes:create", "notes.sensitive:read"];

function reads(c: Candidate) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { path?: { documentId?: string } } }) => {
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    if (path === "/v1/candidates/{candidateId}") return Promise.resolve(ok(c));
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: docs }));
    if (path === "/v1/documents/{documentId}") return Promise.resolve(ok(docs.find((d) => d.id === init?.params?.path?.documentId)));
    return Promise.resolve(ok({ enabled: false, usedTokens: 0, monthlyTokens: 0 }));
  }) as never);
}

const now = new Date();
const today = `${String(now.getFullYear())}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;

describe("screening call", () => {
  it("picks the newest CV the candidate sent, not one the app formatted", () => {
    expect(latestCv(docs)?.id).toBe("doc_new");
    expect(latestCv([docs[2] as Document])?.id).toBe("doc_fmt");
    expect(latestCv([])).toBeUndefined();
  });

  it("shows the CV beside the call, and saves what was asked in one step", async () => {
    const c = testCandidate({ source: null, nationality: "MY", noticePeriod: null, compensation: null });
    window.location.hash = `#/candidates/${c.id}/screening`;
    reads(c);
    const saved = testCandidate({ ...c, version: 4, source: "referral" });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ candidate: saved, noteId: "note_1", sourceKept: false, call: { id: "scl_1", calledOn: "2026-11-09", calledBy: "usr_1", calledByName: "Aina", noteId: null, salaryRecorded: false, createdAt: "2026-11-09T00:00:00Z" }, salaryApplied: true }));
    renderWithClient(<ScreeningCall candidateId={c.id} permissions={CALLER} />);

    expect(await screen.findByText("text of doc_new")).toBeTruthy(); // the CV, readable during the call
    const form = screen.getByRole("form", { name: "Screening call" });
    fireEvent.change(within(form).getByLabelText("RFL"), { target: { value: "Better prospects " } });
    fireEvent.change(within(form).getByLabelText("LF"), { target: { value: "Audit roles in SG" } });
    expect(within(form).queryByLabelText("ES")).toBeNull(); // salary and notice have their own fields
    fireEvent.change(within(form).getByLabelText("Other notes"), { target: { value: "  Wants hybrid; open to SG.  " } });
    fireEvent.click(within(form).getByLabelText(/Sensitive note/));
    fireEvent.change(within(form).getByLabelText("Notice period"), { target: { value: "2_week" } });
    fireEvent.change(within(form).getByLabelText("Nationality"), { target: { value: "SG" } });
    fireEvent.change(within(form).getByLabelText("Work authorization"), { target: { value: "requires_visa" } });
    fireEvent.click(within(form).getByRole("button", { name: /Referral/ }));
    fireEvent.change(within(form).getAllByLabelText("From (or the single figure)")[0] as HTMLElement, { target: { value: "9,500" } });
    fireEvent.change(within(form).getAllByLabelText("Currency")[1] as HTMLElement, { target: { value: "sgd" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save the call" }));
    expect(window.location.hash).toBe(`#/candidates/${c.id}/screening`);

    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/candidates/{candidateId}/screening-calls");
    expect(sentBody(post)).toEqual({
      version: 3, notes: "RFL: Better prospects\nLF: Audit roles in SG\n\nWants hybrid; open to SG.", sensitive: true, calledOn: today,
      nationality: "SG", workAuthorization: { status: "requires_visa", basis: null, note: null }, noticePeriod: { value: 2, unit: "week" },
      source: "referral",
      compensation: {
        expectedVersion: 0, currentSalary: null,
        expectedSalary: { min: "9500", max: null, currency: "SGD", period: "month" },
      },
    });
    expect((await screen.findByRole("status")).textContent).toContain("with the notes");
    await waitFor(() => { expect(window.location.hash).toBe(`#/candidates/${c.id}`); });
    expect(within(form).getByLabelText<HTMLTextAreaElement>("Other notes").value).toBe("");
    expect(within(form).getByLabelText<HTMLTextAreaElement>("RFL").value).toBe("");
    expect(within(form).getByLabelText<HTMLInputElement>(/Sensitive note/).checked).toBe(false); // each note is marked on its own
  });

  it("keeps an existing source and leaves salary alone when it was not touched or may not be changed", async () => {
    const c = testCandidate({ source: "jobstreet" });
    reads(c);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ candidate: c, noteId: null, sourceKept: false, call: { id: "scl_1", calledOn: "2026-11-09", calledBy: "usr_1", calledByName: "Aina", noteId: null, salaryRecorded: false, createdAt: "2026-11-09T00:00:00Z" }, salaryApplied: true }));
    renderWithClient(<ScreeningCall candidateId={c.id} permissions={CALLER} />);
    const form = await screen.findByRole("form", { name: "Screening call" });
    expect(within(form).getByText(/JobStreet \(kept/)).toBeTruthy();
    fireEvent.click(within(form).getByRole("button", { name: "Save the call" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    // Only what changed is sent: nothing here, so nothing a colleague changed meanwhile is sent back.
    expect(sentBody(post)).toEqual({ version: 3, notes: "", sensitive: false, calledOn: today });
    cleanup();

    reads(c);
    renderWithClient(<ScreeningCall candidateId={c.id} permissions={["candidates:read", "candidates:update", "documents:read", "notes:create"]} />);
    const sourcerForm = await screen.findByRole("form", { name: "Screening call" });
    expect(within(sourcerForm).queryByText("Expected salary")).toBeNull(); // salary needs its own permission
    expect(within(sourcerForm).queryByLabelText(/Sensitive note/)).toBeNull();
  });

  it("when someone saved first, starts again from their version and keeps the notes typed", async () => {
    const c = testCandidate({ source: "jobstreet" });
    window.location.hash = `#/candidates/${c.id}/screening`;
    const get = reads(c);
    const post = vi.spyOn(api, "POST").mockResolvedValueOnce(failure("CONFLICT_STALE_VERSION", "Someone else saved first.", undefined, 409));
    renderWithClient(<ScreeningCall candidateId={c.id} permissions={CALLER} />);
    const form = await screen.findByRole("form", { name: "Screening call" });
    fireEvent.change(within(form).getByLabelText("Other notes"), { target: { value: "Twenty minutes of notes" } });
    fireEvent.change(within(form).getByLabelText("Salary notes"), { target: { value: "Keep my pay discussion" } });
    fireEvent.change(within(form).getByLabelText("Nationality"), { target: { value: "SG" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save the call" }));
    const reload = await within(form).findByRole("button", { name: "Reload their version (your notes are kept; other changes here are lost)" });
    expect(window.location.hash).toBe(`#/candidates/${c.id}/screening`);

    get.mockRestore();
    reads(testCandidate({ ...c, version: 5, nationality: "ID" }));
    fireEvent.click(reload);
    await waitFor(() => { expect(within(form).getByLabelText<HTMLSelectElement>("Nationality").value).toBe("ID"); });
    expect(within(form).getByLabelText<HTMLTextAreaElement>("Other notes").value).toBe("Twenty minutes of notes");
    expect(within(form).getByLabelText<HTMLTextAreaElement>("Salary notes").value).toBe("Keep my pay discussion");

    post.mockResolvedValueOnce(ok({ candidate: testCandidate({ ...c, version: 5 }), noteId: "note_2", sourceKept: false, call: { id: "scl_1", calledOn: "2026-11-09", calledBy: "usr_1", calledByName: "Aina", noteId: null, salaryRecorded: false, createdAt: "2026-11-09T00:00:00Z" }, salaryApplied: true }));
    fireEvent.click(within(form).getByRole("button", { name: "Save the call" }));
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toMatchObject({ version: 5, notes: "Twenty minutes of notes", compensation: {salaryNotes:"Keep my pay discussion"} });
  });
});


it("clears a salary draft when switching to another cached candidate", async () => {
  const first = testCandidate({ compensation: null });
  reads(first);
  const { client, rerender } = renderWithClient(<ScreeningCall candidateId={first.id} permissions={CALLER} />);
  const input = await screen.findByLabelText("Current salary breakdown");
  fireEvent.change(input, { target: { value: "16k + AWS" } });
  const other = testCandidate({ id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", fullName: "Other Example", compensation: null });
  client.setQueryDefaults(candidateKey(other.id), { staleTime: Infinity });
  client.setQueryData(candidateKey(other.id), other);
  rerender(<ScreeningCall candidateId={other.id} permissions={CALLER} />);
  await waitFor(() => { expect(screen.getByLabelText<HTMLTextAreaElement>("Current salary breakdown").value).toBe(""); });
});

it("returns to the profile with a notice when a later call kept its salary", async () => {
  const c = testCandidate({ compensation: null });
  reads(c);
  vi.spyOn(api, "POST").mockResolvedValue(ok({ candidate: c, noteId: null, sourceKept: false,
    call: { id: "scl_older", calledOn: "2026-11-01", calledBy: "usr_1", calledByName: "Aina", noteId: null, salaryRecorded: true, createdAt: "2026-11-09T00:00:00Z" }, salaryApplied: false }));
  renderWithClient(<ScreeningCall candidateId={c.id} permissions={CALLER} />);
  const form = await screen.findByRole("form", { name: "Screening call" });
  fireEvent.change(within(form).getByLabelText("Amount"), { target: { value: "9000" } });
  fireEvent.click(within(form).getByRole("button", { name: "Save the call" }));
  await waitFor(() => { expect(window.location.hash).toBe(`#/candidates/${c.id}?screeningSalary=kept`); });
});

it("saves the freshly applied salary when save is clicked immediately", async () => {
  const c = testCandidate({ compensation: null });
  reads(c);
  const post = vi.spyOn(api,"POST").mockImplementation(((path: string) => Promise.resolve(path === "/v1/salary-calculations"
    ? ok({base:"9000",allowances:"400",bonus:"0",monthly:"9400",annualMonths:12,hasCurrent:true,reviewRequired:false,expectedMin:"10340",warnings:[],excluded:[]})
    : ok({candidate:c,noteId:null,sourceKept:false,call:{id:"scl_new",calledOn:today,calledBy:"usr_1",calledByName:"Aina",noteId:null,salaryRecorded:true,createdAt:"2026-11-09T00:00:00Z"},salaryApplied:true}))) as never);
  renderWithClient(<ScreeningCall candidateId={c.id} permissions={CALLER} />);
  fireEvent.change(await screen.findByLabelText("Current salary breakdown"),{target:{value:"9k + 400 allowance"}});
  fireEvent.change(screen.getByLabelText("Expected salary"),{target:{value:"10%"}});
  await waitFor(() => { expect(screen.getByRole<HTMLButtonElement>("button",{name:"Calculate"}).disabled).toBe(false); });
  fireEvent.click(screen.getByRole("button",{name:"Calculate"}));
  fireEvent.click(await screen.findByRole("button",{name:"Use these figures"}));
  fireEvent.click(screen.getByRole("button",{name:"Save the call"}));
  await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
  expect(sentBody(post,1)).toMatchObject({compensation:{currentSalary:{amount:"9400"},expectedSalary:{min:"10340"}}});
});
