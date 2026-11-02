import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Contact } from "../api/client";
import { ContactDetail } from "../pages/ContactDetail";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ContactForm } from "./ContactForm";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const contact: Contact = {
  id: "cont_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank",
  firstName: "Aisyah", lastName: "Rahman", fullName: "Aisyah Rahman", title: "HR Manager", email: "hr@maybank.com",
  emailCheck: { status: "risky", reasons: ["a shared mailbox (hr@), not a person"], checkedAt: "2026-11-01T00:00:00Z" },
  phone: null, mobile: "+60 12-345 6789", linkedinUrl: null, status: "active", notes: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

const change = (label: string, value: string) => {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
};

describe("contact form", () => {
  it("asks for the company and finds it by name", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ id: contact.companyId, name: "Maybank" }], nextCursor: null }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(contact));
    renderWithClient(<ContactForm contact={null} onDone={vi.fn()} />);
    change("First name", "Aisyah");
    fireEvent.click(screen.getByRole("button", { name: "Add contact" }));
    expect(await screen.findByText("Choose the company they work at.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();

    change("Company", "may");
    fireEvent.click(await screen.findByRole("button", { name: "Maybank" }));
    change("Email", "josé@maybank.com"); // accepted: the API decides
    fireEvent.click(screen.getByRole("button", { name: "Add contact" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ companyId: contact.companyId, firstName: "Aisyah", email: "josé@maybank.com", status: "active" });
  });

  it("starts at the company it was opened from", async () => {
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(contact));
    renderWithClient(<ContactForm contact={null} company={{ id: contact.companyId, name: "Maybank" }} onDone={vi.fn()} />);
    expect(screen.getByText("Maybank")).toBeTruthy();
    change("First name", "Wei");
    fireEvent.click(screen.getByRole("button", { name: "Add contact" }));
    await waitFor(() => { expect(sentBody(post)).toMatchObject({ companyId: contact.companyId }); });
  });
});

describe("contact page", () => {
  it("explains the email check and lets editors check again", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(contact));
    const recheck = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...contact, emailCheck: { ...contact.emailCheck, checkedAt: "2026-11-02T00:00:00Z" } }));
    renderWithClient(<ContactDetail id={contact.id} permissions={["contacts:read", "contacts:update"]} />);
    await screen.findByRole("heading", { name: "Aisyah Rahman" });
    expect(screen.getByText("⚠ Risky")).toBeTruthy();
    expect(screen.getByText(/a shared mailbox \(hr@\), not a person/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Check again" }));
    await waitFor(() => { expect(recheck).toHaveBeenCalled(); });
  });

  it("gives viewers no edit or check buttons", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(contact));
    renderWithClient(<ContactDetail id={contact.id} permissions={["contacts:read"]} />);
    await screen.findByRole("heading", { name: "Aisyah Rahman" });
    expect(screen.queryByRole("button", { name: "Edit contact" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Check again" })).toBeNull();
  });
});
