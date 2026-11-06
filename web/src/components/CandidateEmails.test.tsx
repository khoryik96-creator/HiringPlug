import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
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
        return Promise.resolve(ok({ to: ["hr@contoh.test"], subject: "Candidate for Accountant", body: "Dear [company.name] team", missing: ["company.name"] }));
      }
      return Promise.resolve(ok(sent));
    }) as never);
    renderWithClient(<CandidateEmails candidateId="cand_1" permissions={["emails:read", "emails:send", "pipeline:read"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Write email" }));
    fireEvent.change(await screen.findByLabelText("About"), { target: { value: "appl_1" } });
    fireEvent.change(await screen.findByLabelText("Template"), { target: { value: "etpl_2" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("To").value).toBe("hr@contoh.test"); });
    expect(sentBody(post)).toMatchObject({ templateId: "etpl_2", candidateId: "cand_1", applicationId: "appl_1" });
    expect(screen.getByRole("status").textContent).toContain("[company.name]");
    const send = screen.getByRole<HTMLButtonElement>("button", { name: "Send" });
    expect(send.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Dear Contoh team" } });
    expect(send.disabled).toBe(false);
    fireEvent.change(screen.getByLabelText("Cc"), { target: { value: "me@agency.test; boss@agency.test" } });
    fireEvent.click(send);
    await waitFor(() => { expect(post).toHaveBeenCalledTimes(2); });
    expect(sentBody(post, 1)).toEqual({
      to: ["hr@contoh.test"], cc: ["me@agency.test", "boss@agency.test"], subject: "Candidate for Accountant",
      body: "Dear Contoh team", candidateId: "cand_1", applicationId: "appl_1",
    });
  });

  it("reads addresses however they are separated", () => {
    expect(addresses(" a@x.test, b@y.test;c@z.test  d@w.test ")).toEqual(["a@x.test", "b@y.test", "c@z.test", "d@w.test"]);
  });
});
