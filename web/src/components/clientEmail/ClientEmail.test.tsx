import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Permission, type Schemas } from "../../api/client";
import { ok, renderWithClient, sentBody, testMe } from "../../test-utils";
import { ClientEmail } from "./ClientEmail";
import { plainToHtml } from "./richText";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const fields: Schemas["EmailField"][] = [
  { token: "[Full name]", key: "candidate.fullName", label: "Full name", salary: false },
  { token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true },
];
const template: Schemas["EmailTemplate"] = {
  id: "etpl_1", version: 2, name: "Client submission", subject: "[Full name] for [Job title]", body: "x",
  bodyHtml: "<p>Dear [Client contact],</p><p>Expecting [Expected salary].</p>", createdAt: "x", updatedAt: "x",
};
const docs = [
  { id: "doc_1", source: "original", fileName: "Siti CV.pdf", sizeBytes: 120000, pdfStatus: null },
  { id: "doc_2", source: "formatted", fileName: "Siti (formatted).docx", sizeBytes: 40000, pdfStatus: "ready" },
];
const ALL: Permission[] = ["emails:send", "emails.templates:manage", "documents:read", "candidates.compensation:read"];

function setup(permissions: Permission[] = ALL) {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/me") return Promise.resolve(ok(testMe(permissions)));
    if (path === "/v1/email-templates") return Promise.resolve(ok({ items: [template], placeholders: [], fields }));
    if (path === "/v1/email-banners") return Promise.resolve(ok({ items: [] }));
    if (path === "/v1/candidates/{candidateId}/documents") return Promise.resolve(ok({ items: docs }));
    return Promise.resolve(ok({ items: [] }));
  }) as never);
  const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => {
    if (path === "/v1/email-previews") {
      return Promise.resolve(ok({ subject: "Siti Aisyah for Auditor", bodyHtml: "<p>Expecting [Expected salary].</p>", missing: ["[Expected salary]"], salary: false, clientEmail: "hr@client.test", fieldValues: { "[Full name]": "Siti Aisyah" } }));
    }
    if (path === "/v1/email-templates") return Promise.resolve(ok({ ...template, id: "etpl_2", name: "Mine" }));
    return Promise.resolve(ok({ id: "eml_1" }));
  }) as never);
  renderWithClient(<ClientEmail candidateId="cand_1" applicationId="appl_1" permissions={permissions} onClose={vi.fn()} />);
  return { post, panel: screen.getByRole("region", { name: "Email client" }) };
}

describe("emailing the client", () => {
  it("starts addressed to the job's hiring contact, from you", async () => {
    const { panel } = setup();
    await waitFor(() => { expect(within(panel).getByLabelText<HTMLInputElement>("To").value).toBe("hr@client.test"); });
    expect(await within(panel).findByText(/From: Demo \(Demo company\) · replies go to demo@example.com/)).toBeTruthy();
  });

  it("uses a template, inserts fields, and sends with cc, bcc and the attachments", async () => {
    const { post, panel } = setup();
    await within(panel).findByRole("option", { name: "Client submission" });
    fireEvent.change(within(panel).getByLabelText("Template"), { target: { value: "etpl_1" } });
    fireEvent.click(within(panel).getByRole("button", { name: "Use selected template" }));
    expect(within(panel).getByLabelText<HTMLInputElement>("Subject").value).toBe("[Full name] for [Job title]");
    const message = within(panel).getByRole("textbox", { name: "Message" });
    expect(message.innerHTML).toContain("Expecting [Expected salary].");
    await waitFor(() => { expect(within(panel).getByRole("option", { name: "Expected salary (salary) — no value" })).toBeTruthy(); });
    fireEvent.change(within(panel).getByLabelText("Insert field"), { target: { value: "[Full name]" } });
    // The value at once, marked with its field (editable; the record is not changed).
    expect(message.innerHTML).toContain('<span data-field="candidate.fullName">Siti Aisyah</span>');

    fireEvent.change(within(panel).getByLabelText("To"), { target: { value: "hr@client.test, boss@client.test" } });
    fireEvent.change(within(panel).getByLabelText("CC"), { target: { value: "lead@agency.test" } });
    fireEvent.change(within(panel).getByLabelText("BCC"), { target: { value: "me@agency.test" } });
    fireEvent.click(await within(panel).findByLabelText(/Original: Siti CV.pdf/));
    fireEvent.click(within(panel).getByLabelText("as PDF"));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "eat_1", fileName: "JD.pdf", contentType: "application/pdf", sizeBytes: 2048 }), { status: 201 })));
    fireEvent.change(within(panel).getByLabelText("Attach files"), { target: { files: [new File(["%PDF"], "JD.pdf")] } });
    expect(await within(panel).findByText("JD.pdf")).toBeTruthy();

    fireEvent.click(within(panel).getByRole("button", { name: "Send email" }));
    await waitFor(() => { expect(post.mock.calls.some((c) => c[0] === "/v1/emails")).toBe(true); });
    const call = post.mock.calls.findIndex((c) => c[0] === "/v1/emails");
    expect(sentBody(post, call)).toMatchObject({
      to: ["hr@client.test", "boss@client.test"], cc: ["lead@agency.test"], bcc: ["me@agency.test"],
      subject: "[Full name] for [Job title]", candidateId: "cand_1", applicationId: "appl_1",
      attachmentIds: ["eat_1"], cvs: [{ documentId: "doc_1", asPdf: false }, { documentId: "doc_2", asPdf: true }],
    });
    const sentHtml = (sentBody(post, call) as { bodyHtml: string }).bodyHtml;
    expect(sentHtml).toContain('<span data-field="candidate.fullName">Siti Aisyah</span>');
    expect(sentHtml).not.toContain("\u200b"); // the cursor mark is not sent
    expect(await screen.findByRole("status")).toBeTruthy();
  });

  it("previews the filled message and says which fields have no value", async () => {
    const { panel } = setup();
    fireEvent.click(within(panel).getByRole("button", { name: "Preview" }));
    const preview = await within(panel).findByLabelText("Preview");
    expect(preview.textContent).toContain("No value for this candidate: [Expected salary]");
    expect(preview.textContent).toContain("Siti Aisyah for Auditor");
  });

  it("saves templates as new or over the one picked, and deletes them; only for roles that manage templates", async () => {
    const { post, panel } = setup();
    fireEvent.change(within(panel).getByLabelText("Template name"), { target: { value: "Mine" } });
    fireEvent.change(within(panel).getByLabelText("Subject"), { target: { value: "Hello" } });
    fireEvent.click(within(panel).getByRole("button", { name: "Save as new" }));
    await waitFor(() => { expect(post.mock.calls.some((c) => c[0] === "/v1/email-templates")).toBe(true); });
    expect(sentBody(post, post.mock.calls.findIndex((c) => c[0] === "/v1/email-templates"))).toMatchObject({ name: "Mine", subject: "Hello" });

    await within(panel).findByRole("option", { name: "Client submission" });
    fireEvent.change(within(panel).getByLabelText("Template"), { target: { value: "etpl_1" } });
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(template));
    fireEvent.click(within(panel).getByRole("button", { name: "Save changes" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ version: 2, name: "Mine" });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const del = vi.spyOn(api, "DELETE").mockResolvedValue(ok(undefined));
    fireEvent.click(within(panel).getByRole("button", { name: "Delete template" }));
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    cleanup();

    const { panel: sourcer } = setup(["emails:send"]);
    expect(within(sourcer).queryByRole("button", { name: "Save as new" })).toBeNull();
    expect(within(sourcer).queryByText(/From the ATS/)).toBeNull(); // CVs need documents:read
    expect(within(sourcer).getByText(/Salary fields need a role that may see salaries/)).toBeTruthy();
  });
});

describe("plain templates in the editor", () => {
  it("keep their paragraphs and line breaks, escaped", () => {
    expect(plainToHtml("Dear <Siti>,\nHello\n\nRegards")).toBe("<p>Dear &lt;Siti&gt;,<br>Hello</p><p>Regards</p>");
  });
});
