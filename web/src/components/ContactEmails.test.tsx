import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { ContactEmails } from "./ContactEmails";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const fields = [
  { token: "[First name]", key: "candidate.firstName", label: "First name", salary: false },
  { token: "[Client company]", key: "company.name", label: "Client company", salary: false },
];

describe("emailing a client's contact", () => {
  it("writes to them from their page, fields filled from the contact, and lists what was sent", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-templates"
      ? { items: [], placeholders: [], fields } : { items: [] }))) as never);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/email-previews"
      ? { subject: "", bodyHtml: "", missing: [], salary: false, clientEmail: null, fieldValues: { "[First name]": "Aisyah", "[Client company]": "Contoh Berhad" } }
      : { id: "eml_1" }))) as never);
    renderWithClient(<ContactEmails contactId="cont_1" email="aisyah@contoh.test" permissions={["emails:read", "emails:send"]} />);
    expect(await screen.findByText("No emails sent from HiringPlug yet.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Write email" }));
    expect(screen.getByLabelText<HTMLInputElement>("To").value).toBe("aisyah@contoh.test");
    expect(screen.queryByRole("button", { name: "+ Booking link" }), "booking links are for candidates").toBeNull();
    await waitFor(() => {
      expect(post).toHaveBeenCalledWith("/v1/email-previews", { body: expect.objectContaining({ contactId: "cont_1" }) as unknown });
    });
    fireEvent.change(screen.getByLabelText("Subject"), { target: { value: "[Client company]: catching up" } });
    expect(screen.getByLabelText<HTMLInputElement>("Subject").value).toBe("Contoh Berhad: catching up");
    const box = screen.getByRole("textbox", { name: "Message" });
    box.innerHTML = "<p>Dear Aisyah,</p>";
    fireEvent.input(box);
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => { expect(post).toHaveBeenCalledWith("/v1/emails", expect.anything()); });
    const calls = (post as unknown as { mock: { calls: unknown[][] } }).mock.calls;
    const sent = calls.find((c) => c[0] === "/v1/emails")?.[1] as { body: Record<string, unknown> };
    expect(sent.body).toMatchObject({ to: ["aisyah@contoh.test"], subject: "Contoh Berhad: catching up", contactId: "cont_1" });
    expect(sent.body.candidateId).toBeUndefined();
  });
});
