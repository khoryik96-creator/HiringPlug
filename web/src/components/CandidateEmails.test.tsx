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
  id: "eml_1", candidateId: "cand_1", applicationId: null, to: ["siti@example.com"], cc: [], bcc: [], bodyHtml: null, attachments: [], salary: false, subject: "Your application",
  body: "Dear Siti,\n\nThanks.", status: "failed", error: "550 no such user", sentBy: "usr_1", sentByName: "Aina",
  createdAt: "2026-11-01T00:00:00Z", sentAt: null,
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
    expect(send.disabled).toBe(true);
    typeMessage("<p>Dear <b>Contoh</b> team</p>");
    expect(send.disabled).toBe(false);
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
