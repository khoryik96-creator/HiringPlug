import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { CandidateEmails } from "./CandidateEmails";
import { addresses } from "./EmailComposer";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const template = (name: string, id: string): Schemas["EmailTemplate"] => ({
  id, version: 1, name, subject: "s", body: "b", bodyHtml: null, createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
});
const sent: Schemas["Email"] = {
  id: "eml_1", candidateId: "cand_1", contactId: null, applicationId: null, to: ["siti@example.com"], cc: [], bcc: [], bodyHtml: null, attachments: [], salary: false, subject: "Your application",
  body: "Dear Siti,\n\nThanks.", status: "failed", error: "550 no such user", sentBy: "usr_1", sentByName: "Aina",
  createdAt: "2026-11-01T00:00:00Z", sentAt: null, bouncedTo: [], bouncedAt: null,
};
const reads = (emails: Schemas["Email"][]) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/email-templates") {
      return Promise.resolve(ok({ items: [template("Interview invitation", "etpl_1"), template("Submit candidate to client", "etpl_2")], placeholders: [], fields: [] }));
    }
    if (path === "/v1/candidates/{candidateId}/applications") {
      return Promise.resolve(ok({ items: [{ id: "appl_1", jobTitle: "Accountant", companyName: "Contoh" }] }));
    }
    return Promise.resolve(ok({ items: emails }));
  }) as never);

const preview = { subject: "", bodyHtml: "", missing: [], salary: false, clientEmail: null, fieldValues: {} };
/** The body sent to path (the composer also asks for a preview of the fields' values). */
const bodyTo = (post: { mock: { calls: unknown[][] } }, path: string) =>
  (post.mock.calls.find((c) => c[0] === path)?.[1] as { body: unknown } | undefined)?.body;
/** Types a message into the formatted message box. */
const typeMessage = (html: string) => {
  const box = screen.getByRole("textbox", { name: "Message" });
  box.innerHTML = html;
  fireEvent.input(box);
  return box;
};

describe("candidate emails", () => {
  it("lists what was sent, with why one was not", async () => {
    reads([sent]);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read"]} />);
    expect(await screen.findByText("Your application")).toBeTruthy();
    expect(screen.getByText("Not sent")).toBeTruthy();
    expect(screen.getByText("Why: 550 no such user")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Write email" })).toBeNull(); // reading is not sending
    expect(screen.queryByText("Bounced")).toBeNull();
  });

  it("shows which recipients a sent email bounced back from", async () => {
    reads([{ ...sent, status: "sent", error: null, deliveryMode: "live", to: ["siti@example.com", "nur@example.com"],
      bouncedTo: ["nur@example.com"], bouncedAt: "2026-11-01T00:20:00Z" }]);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read"]} />);
    expect(await screen.findByText("Bounced")).toBeTruthy();
    expect(screen.getByText("Bounced back from nur@example.com: the mailbox does not exist.")).toBeTruthy();
  });

  it("fills a template for a job, waits for missing values, then sends", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => {
      if (path === "/v1/email-drafts") {
        return Promise.resolve(ok({ to: ["hr@contoh.test"], subject: "Candidate for Accountant", body: "Dear [company.name] team", bodyHtml: null, missing: ["company.name"] }));
      }
      if (path === "/v1/email-previews") return Promise.resolve(ok(preview));
      return Promise.resolve(ok(sent));
    }) as never);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read", "emails:send", "pipeline:read"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Write email" }));
    expect(screen.getByRole("toolbar", { name: "Formatting" }), "bold, italic, underline, indent…").toBeTruthy();
    expect(screen.getByLabelText("Insert field")).toBeTruthy();
    fireEvent.change(await screen.findByLabelText("About"), { target: { value: "appl_1" } });
    fireEvent.change(await screen.findByLabelText("Template"), { target: { value: "etpl_2" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("To").value).toBe("hr@contoh.test"); });
    expect(bodyTo(post, "/v1/email-drafts")).toMatchObject({ templateId: "etpl_2", candidateId: "cand_1", applicationId: "appl_1" });
    expect(screen.getByRole("textbox", { name: "Message" }).textContent).toBe("Dear [company.name] team");
    expect(screen.getByRole("status").textContent).toContain("[company.name]");
    const send = screen.getByRole<HTMLButtonElement>("button", { name: "Send" });
    fireEvent.click(send); // [company.name] is still to fill in
    expect(bodyTo(post, "/v1/emails")).toBeUndefined();
    expect(screen.getByRole("textbox", { name: "Message" }).getAttribute("aria-invalid")).toBe("true");
    typeMessage("<p>Dear <b>Contoh</b> team</p>");
    expect(screen.getByRole("textbox", { name: "Message" }).getAttribute("aria-invalid")).toBeNull();
    expect(screen.queryByRole("textbox", { name: "Cc" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Cc" }));
    fireEvent.change(screen.getByLabelText("Cc"), { target: { value: "me@agency.test; boss@agency.test" } });
    fireEvent.click(send);
    await waitFor(() => { expect(bodyTo(post, "/v1/emails")).toBeTruthy(); });
    expect(bodyTo(post, "/v1/emails")).toEqual({
      to: ["hr@contoh.test"], cc: ["me@agency.test", "boss@agency.test"], subject: "Candidate for Accountant",
      bodyHtml: "<p>Dear <b>Contoh</b> team</p>", candidateId: "cand_1", applicationId: "appl_1", timeZone: expect.any(String) as string,
    });
  });

  it("shows a formatted template formatted", async () => {
    reads([]);
    vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-drafts"
      ? { to: ["siti@example.com"], subject: "Interview", body: "Dear Siti", bodyHtml: "<p>Dear <b>Siti</b>,</p><ul><li>Bring your IC</li></ul>", missing: [] }
      : preview))) as never);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read", "emails:send"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Write email" }));
    await screen.findByRole("option", { name: "Interview invitation" });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "etpl_1" } });
    await waitFor(() => { expect(screen.getByRole("textbox", { name: "Message" }).innerHTML).toContain("<b>Siti</b>"); });
    expect(screen.getByRole("textbox", { name: "Message" }).innerHTML).toContain("<li>Bring your IC</li>");
  });

  it("fills a template's fields with the candidate's values, salary too, in the subject and the message", async () => {
    reads([]);
    const fieldValues = { "[First name]": "Siti", "[Expected salary]": "RM 12,000 per month" };
    vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-drafts"
      ? { to: ["siti@example.com"], subject: "[First name] - [Expected salary]", body: "x", bodyHtml: "<p>Hi [First name], you expect [Expected salary].</p>", missing: [] }
      : { ...preview, fieldValues }))) as never);
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-templates"
      ? { items: [template("Interview invitation", "etpl_1")], placeholders: [], fields: [
        { token: "[First name]", key: "candidate.firstName", label: "First name", salary: false },
        { token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true }] }
      : { items: [] }))) as never);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read", "emails:send"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Write email" }));
    await screen.findByRole("option", { name: "Interview invitation" });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "etpl_1" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Subject").value).toBe("Siti - RM 12,000 per month"); });
    expect(screen.getByRole("textbox", { name: "Message" }).textContent.replaceAll("\u200b", "")).toBe("Hi Siti, you expect RM 12,000 per month.");
  });

  it("saves the email as a new template, the candidate's details back as fields", async () => {
    reads([]);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-previews"
      ? { ...preview, fieldValues: { "[First name]": "Siti", "[Full name]": "Siti Aisyah" } }
      : template("Follow-up", "etpl_9")))) as never);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read", "emails:send", "emails.templates:create"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Write email" }));
    expect(screen.queryByRole("link", { name: "Edit templates" }), "changing them is for admins").toBeNull();
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/email-previews", expect.anything()); });
    fireEvent.change(screen.getByLabelText("Subject"), { target: { value: "Siti Aisyah, Siti: next steps" } });
    typeMessage('<p>Hi <span data-field="candidate.firstName">Siti</span></p>');
    fireEvent.click(screen.getByRole("button", { name: "Save as a new template" }));
    fireEvent.change(screen.getByLabelText("New template name"), { target: { value: "Follow-up" } });
    fireEvent.click(screen.getByRole("button", { name: "Save template" }));
    await waitFor(() => { expect(bodyTo(post, "/v1/email-templates")).toBeTruthy(); });
    expect(bodyTo(post, "/v1/email-templates")).toEqual({
      name: "Follow-up", subject: "[Full name], [First name]: next steps", bodyHtml: '<p>Hi <span data-field="candidate.firstName">Siti</span></p>',
    });
    expect(await screen.findByText(/Saved as/)).toBeTruthy();
  });

  it("reads addresses however they are separated", () => {
    expect(addresses(" a@x.test, b@y.test;c@z.test  d@w.test ")).toEqual(["a@x.test", "b@y.test", "c@z.test", "d@w.test"]);
  });

  it("puts a booking link in the message", async () => {
    reads([]);
    vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-previews"
      ? preview : { id: "bkl_1", url: "https://ats.example/book/abc", durationMinutes: 30 }))) as never);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read", "emails:send", "scheduling:use"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Write email" }));
    typeMessage("<p>Hi Siti,</p>");
    fireEvent.click(screen.getByRole("button", { name: "+ Booking link" }));
    fireEvent.click(screen.getByRole("button", { name: "Insert link" }));
    await waitFor(() => {
      expect(screen.getByRole("textbox", { name: "Message" }).textContent)
        .toBe("Hi Siti,Please pick a time that suits you (30 minutes): https://ats.example/book/abc");
    });
  });
});

it("labels captured test mail without implying external delivery",async()=>{
 reads([{...sent,status:"sent",error:null,deliveryMode:"test"}]);
 renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read"]}/>);
 expect(await screen.findByText("Captured in test inbox")).toBeTruthy();expect(screen.getByText(/not delivered to a real mailbox/)).toBeTruthy();
});
