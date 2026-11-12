import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Permission } from "../../api/client";
import { ok, renderWithClient, sentBody, testMe } from "../../test-utils";
import { ClientEmail } from "./ClientEmail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  try { window.localStorage.clear(); } catch { /* not there */ }
});

const ALL: Permission[] = ["emails:send", "documents:read", "candidates.compensation:read"];
const values: Record<string, Record<string, string>> = {
  appl_f: { "[Full name]": "Farid Hassan", "[Current title]": "Hub Supervisor", "[Expected salary]": "RM 9,000 per month", "[Notice period]": "2 months" },
  appl_n: { "[Full name]": "Nurul Huda", "[Expected salary]": "RM 11,000 per month", "[Notice period]": "1 month" },
};
const docs: Record<string, unknown[]> = {
  cand_f: [{ id: "doc_f", source: "original", fileName: "Farid CV.pdf", createdAt: "2026-11-01T00:00:00Z", pdfStatus: null }],
  cand_n: [
    { id: "doc_n1", source: "original", fileName: "Nurul CV.pdf", createdAt: "2026-11-01T00:00:00Z", pdfStatus: null },
    { id: "doc_n2", source: "formatted", fileName: "Hyppies CV - Nurul Huda.docx", createdAt: "2026-11-05T00:00:00Z", pdfStatus: "ready" },
  ],
};

function setup() {
  vi.spyOn(api, "GET").mockImplementation(((path: string, init: { params?: { path?: { candidateId?: string } } }) => {
    if (path === "/v1/me") return Promise.resolve(ok(testMe(ALL)));
    if (path === "/v1/email-templates") return Promise.resolve(ok({ items: [], placeholders: [], fields: [] }));
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: docs[init.params?.path?.candidateId ?? ""] ?? [] }));
    if (path === "/v1/jobs/{jobId}/applications") {
      return Promise.resolve(ok({ items: [{ id: "appl_a", candidateId: "cand_a", candidateName: "Aisyah Rahman", candidateTitle: "Senior Accountant", stage: "applied" }] }));
    }
    return Promise.resolve(ok({ items: [] }));
  }) as never);
  const post = vi.spyOn(api, "POST").mockImplementation(((path: string, init: { body: { applicationId?: string; candidates?: unknown[] } }) => {
    if (path === "/v1/email-previews") {
      const own = init.body.applicationId ? values[init.body.applicationId] ?? {} : { "[Job title]": "Hub Operations Manager" };
      return Promise.resolve(ok({ subject: "", bodyHtml: "", missing: [], salary: false, clientEmail: "hr@client.test", fieldValues: own }));
    }
    return Promise.resolve(ok({ id: "eml_1" }));
  }) as never);
  renderWithClient(<ClientEmail jobId="jo_1" permissions={ALL} onClose={vi.fn()} shortlist={[
    { candidateId: "cand_f", applicationId: "appl_f", name: "Farid Hassan", title: "Hub Supervisor", stage: "offer" },
    { candidateId: "cand_n", applicationId: "appl_n", name: "Nurul Huda", title: "Hub Supervisor", stage: "placed" },
  ]} />);
  fireEvent.click(screen.getByText("Candidates and CVs (2)"));
  return { post, panel: screen.getByRole("region", { name: "Email client" }) };
}

describe("emailing the client about several candidates", () => {
  it("attaches each one's profile, puts their own details in the message, and sends them in order", async () => {
    const { post, panel } = setup();
    expect(within(panel).getByRole("heading", { name: "Email client about 2 candidates" })).toBeTruthy();
    // Nurul's newest formatted profile, as PDF, unless another is chosen.
    const nurul = await within(panel).findByLabelText<HTMLSelectElement>("Profile to attach for Nurul Huda");
    await waitFor(() => { expect(nurul.selectedOptions[0]?.textContent).toContain("📎 Hyppies CV - Nurul Huda (PDF)"); });
    fireEvent.change(within(panel).getByLabelText("Profile to attach for Farid Hassan"), { target: { value: "" } }); // no attachment

    fireEvent.click(within(panel).getByRole("button", { name: "Move Nurul Huda up" }));
    await waitFor(() => { expect(within(panel).getByRole<HTMLButtonElement>("button", { name: "Insert candidates into the message" }).disabled).toBe(false); });
    fireEvent.click(within(panel).getByRole("button", { name: "Insert candidates into the message" }));
    const message = within(panel).getByRole("textbox", { name: "Message" });
    await waitFor(() => { expect(message.textContent).toContain("1. Nurul HudaCurrent salary: —Expected salary: RM 11,000 per month"); });
    expect(message.textContent).toContain("2. Farid HassanCurrent salary: —");
    expect(within(panel).getByText(/Not on record \(shown as —\): Nurul Huda: Current salary, Location; Farid Hassan: Current salary, Location/)).toBeTruthy();

    fireEvent.blur(message);
    fireEvent.change(within(panel).getByLabelText("Subject"), { target: { value: "Candidates for Hub Operations Manager" } });
    fireEvent.click(within(panel).getByRole("button", { name: "Send email" }));
    const paths = () => post.mock.calls.map((c) => (c as unknown[])[0]);
    await waitFor(() => { expect(paths()).toContain("/v1/emails"); });
    const send = paths().indexOf("/v1/emails");
    const body = sentBody(post, send) as { candidates: unknown; cvs: unknown; bodyHtml: string; candidateId?: string };
    expect(body.candidates).toEqual([{ candidateId: "cand_n", applicationId: "appl_n" }, { candidateId: "cand_f", applicationId: "appl_f" }]);
    expect(body.cvs).toEqual([{ documentId: "doc_n2", asPdf: true }]);
    expect(body.candidateId).toBeUndefined();
    expect(body.bodyHtml).toContain(`<span data-field="candidate.expectedSalary">RM 11,000 per month</span>`);
    expect(await within(panel).findByText(/on each candidate's emails/)).toBeTruthy();
  });

  it("adds more from the job and writes them as one table", async () => {
    const { panel } = setup();
    fireEvent.change(await within(panel).findByLabelText("Add a candidate from this job"), { target: { value: "appl_a" } });
    expect(within(panel).getByRole("heading", { name: "Email client about 3 candidates" })).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "One table" }));
    fireEvent.click(within(panel).getByRole("checkbox", { name: "Location" })); // not wanted this time
    await waitFor(() => { expect(within(panel).getByRole<HTMLButtonElement>("button", { name: "Insert candidates into the message" }).disabled).toBe(false); });
    fireEvent.click(within(panel).getByRole("button", { name: "Insert candidates into the message" }));
    const table = await waitFor(() => {
      const t = within(panel).getByRole("textbox", { name: "Message" }).querySelector("table");
      if (!t) throw new Error("no table yet");
      return t;
    });
    expect([...table.querySelectorAll("th")].map((th) => th.textContent)).toEqual(["Candidate", "Current salary", "Expected salary", "Notice period"]);
    expect(table.querySelectorAll("tbody tr")).toHaveLength(3);
  });
});
