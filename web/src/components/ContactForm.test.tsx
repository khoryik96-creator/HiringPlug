import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Contact } from "../api/client";
import { ContactDetail } from "../pages/ContactDetail";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { ContactForm } from "./ContactForm";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const contact: Contact = {
  id: "cont_01jbz6w0m0k8q9x2r3s4t5v6w7", version: 1, companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank",
  firstName: "Aisyah", lastName: "Rahman", fullName: "Aisyah Rahman", title: "HR Manager", email: "hr@maybank.com",
  emailCheck: { status: "risky", reasons: ["a shared mailbox (hr@), not a person"], checkedAt: "2026-11-01T00:00:00Z" },
  phone: null, mobile: "+60 12-345 6789", linkedinUrl: null, status: "active", role: "hr", notes: null, employment: null,
  createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
};

/** GET answers: the field settings, else `data`. */
const reads = (data: unknown) =>
  vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(path === "/v1/settings/fields" ? ok(testFieldSettings()) : ok(data))) as never);

const change = (label: string, value: string) => {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
};

describe("contact form", () => {
  it("asks for the company and finds it by name", async () => {
    reads({ items: [{ id: contact.companyId, name: "Maybank" }], nextCursor: null });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(contact));
    renderWithClient(<ContactForm contact={null} onDone={vi.fn()} />);
    change("First name", "Aisyah");
    fireEvent.click(screen.getByRole("button", { name: "Add contact" }));
    expect(await screen.findByText("Choose the company they work at.")).toBeTruthy();
    expect(post).not.toHaveBeenCalled();

    change("Company", "may");
    fireEvent.click(await screen.findByRole("button", { name: "Maybank" }));
    change("Email", "josé@maybank.com"); // accepted: the API decides
    await screen.findByRole("option", { name: "Director" });
    change("Role", "director");
    fireEvent.click(screen.getByRole("button", { name: "Add contact" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toMatchObject({ companyId: contact.companyId, firstName: "Aisyah", email: "josé@maybank.com", status: "active", role: "director" });
  });

  it("starts at the company it was opened from", async () => {
    reads({ items: [], nextCursor: null });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(contact));
    renderWithClient(<ContactForm contact={null} company={{ id: contact.companyId, name: "Maybank" }} onDone={vi.fn()} />);
    expect(screen.getByText("Maybank")).toBeTruthy();
    change("First name", "Wei");
    fireEvent.click(screen.getByRole("button", { name: "Add contact" }));
    await waitFor(() => { expect(sentBody(post)).toMatchObject({ companyId: contact.companyId, role: null }); });
  });
});

describe("contact page", () => {
  it("explains the email check and lets editors check again", async () => {
    reads(contact);
    const recheck = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...contact, emailCheck: { ...contact.emailCheck, checkedAt: "2026-11-02T00:00:00Z" } }));
    renderWithClient(<ContactDetail id={contact.id} permissions={["contacts:read", "contacts:update"]} />);
    await screen.findByRole("heading", { name: "Aisyah Rahman" });
    const details = within(screen.getByRole("region", { name: "Contact details" }));
    expect(details.getByText("⚠ Risky")).toBeTruthy();
    expect(details.getByText(/a shared mailbox \(hr@\), not a person/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Check again" }));
    await waitFor(() => { expect(recheck).toHaveBeenCalled(); });
  });

  it("gives viewers no edit or check buttons", async () => {
    reads(contact);
    renderWithClient(<ContactDetail id={contact.id} permissions={["contacts:read"]} />);
    await screen.findByRole("heading", { name: "Aisyah Rahman" });
    expect(screen.queryByRole("button", { name: "Edit contact" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Check again" })).toBeNull();
    expect(screen.queryByRole("group", { name: "Role" })).toBeNull();
    expect(screen.queryByLabelText("Still works there?")).toBeNull();
  });

  it("shows the role down the card in its colour, and saves a new role or status straight away", async () => {
    reads(contact);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...contact, version: 2, role: "hiring_manager" }));
    renderWithClient(<ContactDetail id={contact.id} permissions={["contacts:read", "contacts:update"]} />);
    await screen.findByRole("heading", { name: "Aisyah Rahman" });
    const card = screen.getByRole("region", { name: "Contact" });
    await waitFor(() => { expect(card.querySelector<HTMLElement>(".banner-column")?.style.background).toBe("rgb(219, 39, 119)"); });
    expect(card.querySelector(".banner-column")?.textContent).toBe("Role: HR");
    expect(screen.getByRole("button", { name: "HR" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "Hiring manager" }));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ version: 1, role: "hiring_manager" }); });
    await waitFor(() => { expect(card.querySelector(".banner-column")?.textContent).toBe("Role: Hiring manager"); });

    fireEvent.change(screen.getByLabelText("Still works there?"), { target: { value: "left" } });
    await waitFor(() => { expect(sentBody(patch, 1)).toEqual({ version: 2, status: "left" }); });
  });

  it("clears the role when its pill is clicked again", async () => {
    reads(contact);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...contact, version: 2, role: null }));
    renderWithClient(<ContactDetail id={contact.id} permissions={["contacts:read", "contacts:update"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "HR" }));
    await waitFor(() => { expect(sentBody(patch)).toEqual({ version: 1, role: null }); });
    await waitFor(() => { expect(screen.getByRole("region", { name: "Contact" }).querySelector(".banner-column")?.textContent).toBe("Role: Role not set"); });
  });
});
