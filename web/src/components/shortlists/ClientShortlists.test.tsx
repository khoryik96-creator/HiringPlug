import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Document, type Schemas } from "../../api/client";
import { ok, renderWithClient, sentBody } from "../../test-utils";
import { ClientLinkForm, clientCvs } from "./ClientLinkForm";
import { noteFields, withDetail } from "./NoteFields";
import { answeredText, ClientShortlists } from "./ClientShortlists";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const JOB = "jo_01jbz6w0m0k8q9x2r3s4t5v6w7";
const doc = (id: string, fileName: string, source: Document["source"], createdAt: string) => ({ id, fileName, source, createdAt }) as Document;
const person = (n: string, over: Partial<Schemas["ClientShortlistCandidate"]> = {}): Schemas["ClientShortlistCandidate"] => ({
  id: `cslc_01jbz6w0m0k8q9x2r3s4t5v6w${n}`, applicationId: `appl_01jbz6w0m0k8q9x2r3s4t5v6w${n}`, candidateId: `cand_01jbz6w0m0k8q9x2r3s4t5v6w${n}`,
  candidateName: `Person ${n}`, label: `Person ${n}`, blind: false, documentId: "doc_x", note: null, decision: null, comment: null, answeredAt: null, ...over,
});
const link = (over: Partial<Schemas["ClientShortlist"]> = {}): Schemas["ClientShortlist"] => ({
  id: "csl_01jbz6w0m0k8q9x2r3s4t5v6w7", jobId: JOB, status: "open", message: null, expiresAt: "2026-12-06T02:00:00Z", revokedAt: null,
  openedAt: null, lastOpenedAt: null, emailedTo: null, createdByName: "Dewi", createdAt: "2026-11-22T02:00:00Z", candidates: [person("1")], ...over,
});

describe("client links", () => {
  it("offers the formatted CVs, blind first, newest first", () => {
    const docs = [
      doc("d1", "CV - Siti.pdf", "original", "2026-11-01"), doc("d2", "Agency CV - Siti Tan.docx", "formatted", "2026-11-03"),
      doc("d3", "Agency CV - Candidate C-1 (Blinded).docx", "formatted", "2026-11-02"),
    ];
    expect(clientCvs(docs).map((d) => d.id)).toEqual(["d3", "d2"]);
  });

  it("counts the client's answers", () => {
    expect(answeredText(link())).toBe("No answers yet");
    expect(answeredText(link({ candidates: [person("1", { decision: "approved" }), person("2", { decision: "rejected" }), person("3")] })))
      .toBe("1 approved, 1 rejected, 1 waiting");
  });

  it("shows whether the client opened each link and their answers", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [link({
      openedAt: "2026-11-22T05:00:00Z", emailedTo: "aisyah@maju.test",
      candidates: [person("1", { decision: "approved", comment: "Let's meet." }), person("2", { blind: true, label: "Candidate C-2" })],
    }), link({ id: "csl_01jbz6w0m0k8q9x2r3s4t5v6w8", status: "revoked", revokedAt: "2026-11-21T02:00:00Z" })] }));
    renderWithClient(<ClientShortlists jobId={JOB} contactName="Aisyah Rahman" canEmail />);
    const [first] = await screen.findAllByRole("article", { name: "Link of 22 Nov 2026" });
    if (!first) throw new Error("no link");
    expect(within(first).getByText(/Opened/)).toBeTruthy();
    expect(within(first).getByText(/emailed to aisyah@maju.test/)).toBeTruthy();
    expect(within(first).getByText("“Let's meet.”")).toBeTruthy();
    expect(within(first).getByText("(blind: shown as Candidate C-2)")).toBeTruthy();
    expect(within(first).getByText("Waiting")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Revoke" })).toHaveLength(1);
    expect(screen.getByText("Revoked")).toBeTruthy();
  });

  it("makes a link with the CV, note, days and email chosen, and shows it once", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [
      doc("doc_01jbz6w0m0k8q9x2r3s4t5v6w1", "Agency CV - Siti Tan.docx", "formatted", "2026-11-03"),
      doc("doc_01jbz6w0m0k8q9x2r3s4t5v6w2", "Agency CV - Candidate C-1 (Blinded).docx", "formatted", "2026-11-02"),
    ] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ url: "https://ats.test/shortlist/abc", shortlist: link({ emailedTo: "aisyah@maju.test" }) }));
    const done = vi.fn();
    renderWithClient(<ClientLinkForm jobId={JOB} contactName="Aisyah Rahman" canEmail onClose={done}
      people={[{ applicationId: "appl_1", candidateId: "cand_1", name: "Siti Tan", stage: "submitted" }]} />);
    const cv = await screen.findByRole("combobox", { name: "CV for Siti Tan" });
    expect((cv as HTMLSelectElement).value).toBe("doc_01jbz6w0m0k8q9x2r3s4t5v6w2");
    fireEvent.change(screen.getByRole("textbox", { name: "Note for Siti Tan" }), { target: { value: "Strong on close." } });
    fireEvent.change(screen.getByRole("combobox", { name: "Link works for" }), { target: { value: "30" } });
    fireEvent.click(screen.getByRole("checkbox", { name: /Also email it to Aisyah Rahman/ }));
    fireEvent.click(screen.getByRole("button", { name: "Make the link and email it" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({
      candidates: [{ applicationId: "appl_1", documentId: "doc_01jbz6w0m0k8q9x2r3s4t5v6w2", note: "Strong on close." }],
      expiresInDays: 30, emailHiringContact: true,
    });
    expect(await screen.findByDisplayValue("https://ats.test/shortlist/abc")).toBeTruthy();
    expect(screen.getByText("Also emailed to aisyah@maju.test.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(done).toHaveBeenCalled();
  });

  it("asks for a formatted CV first", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [doc("d1", "CV - Siti.pdf", "original", "2026-11-01")] }));
    renderWithClient(<ClientLinkForm jobId={JOB} canEmail={false} onClose={() => undefined}
      people={[{ applicationId: "appl_1", candidateId: "cand_1", name: "Siti Tan", stage: "submitted" }]} />);
    expect(await screen.findByText(/No formatted CV yet/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Make the link" })).toHaveProperty("disabled", true);
    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("adds the candidate's details to the note: notice period, salary (asked for only then), never their name", async () => {
    const fields = [
      { token: "[Full name]", key: "candidate.fullName", label: "Full name", salary: false },
      { token: "[Notice period]", key: "candidate.noticePeriod", label: "Notice period", salary: false },
      { token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true },
      { token: "[Job title]", key: "job.title", label: "Job title", salary: false },
    ];
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/email-templates"
      ? ok({ items: [], fields, placeholders: [] })
      : ok({ items: [doc("doc_01jbz6w0m0k8q9x2r3s4t5v6w1", "Agency CV - Siti Tan.docx", "formatted", "2026-11-03")] }))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string, init: { body: { includeSalary?: boolean } }) => Promise.resolve(
      path === "/v1/email-previews"
        ? ok({ subject: "", bodyHtml: "", missing: [], salary: false, clientEmail: null,
          fieldValues: { "[Notice period]": "1 month", ...(init.body.includeSalary && { "[Expected salary]": "RM 15,000" }) } })
        : ok({ url: "https://ats.test/shortlist/abc", shortlist: link() }))) as never);
    renderWithClient(<ClientLinkForm jobId={JOB} canEmail={false} onClose={() => undefined}
      people={[{ applicationId: "appl_1", candidateId: "cand_1", name: "Siti Tan", stage: "submitted" }]} />);
    const add = await screen.findByRole("combobox", { name: "Add a detail of Siti Tan" });
    await waitFor(() => { expect(within(add).getAllByRole("option").map((o) => o.textContent)).toEqual(["+ Add a detail…", "Notice period", "Expected salary"]); });
    const note = screen.getByRole("textbox", { name: "Note for Siti Tan" });
    fireEvent.change(note, { target: { value: "Strong on close." } });
    fireEvent.change(add, { target: { value: "[Notice period]" } });
    await waitFor(() => { expect((note as HTMLTextAreaElement).value).toBe("Strong on close.\nNotice period: 1 month"); });
    const previews = () => (post.mock.calls as unknown[][]).filter((c) => c[0] === "/v1/email-previews").map((c) => (c[1] as { body: object }).body);
    expect(previews()[0]).toMatchObject({ candidateId: "cand_1", applicationId: "appl_1" });
    expect(previews()[0]).not.toHaveProperty("includeSalary"); // a salary is not asked for to add the notice period
    fireEvent.change(add, { target: { value: "[Expected salary]" } });
    await waitFor(() => { expect((note as HTMLTextAreaElement).value).toBe("Strong on close.\nNotice period: 1 month\nExpected salary: RM 15,000"); });
    expect(previews()[1]).toMatchObject({ includeSalary: true });
  });

  it("puts each detail on its own line", () => {
    expect(withDetail("", "Notice period", " 1  month ")).toBe("Notice period: 1 month");
    expect(withDetail("Strong.  ", "Lives in", "Penang")).toBe("Strong.\nLives in: Penang");
    expect(noteFields([{ token: "[Last name]", key: "candidate.lastName", label: "Last name", salary: false }])).toEqual([]);
  });
});
