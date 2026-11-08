import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { EmailTemplates } from "./EmailTemplates";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const template = {
  id: "etpl_1", version: 2, name: "Submit to client", subject: "Candidate for you", body: "Hi\nSalary: [Expected salary]",
  bodyHtml: "<p>Hi <b>there</b></p><p>Salary: [Expected salary]</p>", createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};
const fields = [
  { token: "[Candidate name]", key: "candidate.fullName", label: "Candidate name", salary: false },
  { token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true },
];
const reads = () => vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
  path === "/v1/email-templates" ? { items: [template], fields, placeholders: [] } : { items: [] },
))) as never);

describe("email templates", () => {
  it("show their formatting, and are edited with CV Studio's toolbar and a field picker", async () => {
    reads();
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(template));
    renderWithClient(<EmailTemplates canManage />);
    const shown = await screen.findByRole("region", { name: "Submit to client" });
    expect(within(shown).getByText("there").tagName).toBe("B");
    fireEvent.click(within(shown).getByRole("button", { name: "Change" }));

    const toolbar = screen.getByRole("toolbar", { name: "Formatting" });
    for (const name of ["Bold", "Italic", "Underline", "Indent", "Outdent", "Bullets"]) {
      expect(within(toolbar).getByRole("button", { name })).toBeTruthy();
    }
    const fieldsInBody = within(toolbar).getByLabelText<HTMLSelectElement>("Insert field");
    expect(Array.from(fieldsInBody.options).map((o) => o.text)).toEqual(["Insert field…", "Candidate name", "Expected salary (salary)"]);
    fireEvent.change(fieldsInBody, { target: { value: "[Candidate name]" } });
    expect(screen.getByRole("textbox", { name: "Message" }).textContent).toContain("[Candidate name]");

    fireEvent.change(screen.getByLabelText("Insert field in subject"), { target: { value: "[Candidate name]" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    const sent = sentBody(put) as { subject: string; bodyHtml: string; version: number };
    expect(sent.version).toBe(2);
    expect(sent.subject).toBe("Candidate for you[Candidate name]");
    expect(sent.bodyHtml).toContain("<b>there</b>");
    expect(sent.bodyHtml).toContain("[Candidate name]");
  });

  it("are only read by those who may not change them", async () => {
    reads();
    renderWithClient(<EmailTemplates canManage={false} />);
    await screen.findByRole("region", { name: "Submit to client" });
    expect(screen.queryByRole("button", { name: "Change" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add template" })).toBeNull();
  });
});
