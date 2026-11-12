import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type ContactSummary } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { ContactTable } from "./ContactTable";
import { SeparateEmail } from "./SeparateEmail";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const people = [
  { candidateId: "cand_1", name: "Aisyah Rahman", email: "aisyah@example.com" },
  { candidateId: "cand_2", name: "Rajesh Kumar", email: "rajesh@example.com" },
  { candidateId: "cand_3", name: "Farid Hassan", email: null },
];
const row = (candidateId: string, name: string, to: string | null, problem: string | null, emailId: string | null = null) =>
  ({ candidateId, contactId: null, name, to, emailId, problem });

function setup() {
  vi.spyOn(api, "GET").mockImplementation(((path: string) =>
    Promise.resolve(ok(path === "/v1/mailbox" ? { storageReady: true, connected: false, serverMode: "test", microsoftReady: false, googleReady: false, version: 0 } : path === "/v1/email-templates" ? { items: [], placeholders: [], fields: [] } : { items: [] }))) as never);
  const post = vi.spyOn(api, "POST").mockImplementation(((path: string, init: { body: { checkOnly?: boolean; candidateId?: string } }) => {
    if (path === "/v1/email-previews") {
      return Promise.resolve(ok({ subject: "Hi Rajesh", bodyHtml: "<p>Hi Rajesh,</p>", missing: [], salary: false, clientEmail: null, fieldValues: {} }));
    }
    const sending = !init.body.checkOnly;
    return Promise.resolve(ok({
      sent: sending ? 1 : 0,
      recipients: [
        row("cand_1", "Aisyah Rahman", "aisyah@example.com", null, sending ? "eml_1" : null),
        row("cand_3", "Farid Hassan", null, "has no email address"),
      ],
    }));
  }) as never);
  renderWithClient(<SeparateEmail people={people} onClose={vi.fn()} />);
  return { post, panel: screen.getByRole("region", { name: "Email each separately" }) };
}

describe("one email to each person separately", () => {
  it("checks who gets it, previews it as one of them, and sends each their own", async () => {
    const { post, panel } = setup();
    expect(within(panel).getByRole("heading", { name: "Email 3 people separately" })).toBeTruthy();
    expect(within(panel).getByText(/no email/)).toBeTruthy();
    expect(await within(panel).findByText(/Local test inbox only/)).toBeTruthy();
    fireEvent.click(within(panel).getByRole("button", { name: "Leave out Rajesh Kumar" }));
    fireEvent.change(within(panel).getByLabelText("Subject"), { target: { value: "A role for you, [First name]" } });

    fireEvent.click(within(panel).getByRole("button", { name: "Check who gets it" }));
    const table = await within(panel).findByRole("table", { name: "Who gets it" });
    expect(within(table).getByText("✓ gets it")).toBeTruthy();
    expect(within(table).getByText("✗ has no email address")).toBeTruthy();
    expect(sentBody(post)).toMatchObject({
      subject: "A role for you, [First name]", checkOnly: true,
      recipients: [{ candidateId: "cand_1" }, { candidateId: "cand_3" }], // Rajesh was left out
    });

    fireEvent.change(within(panel).getByLabelText("Preview as"), { target: { value: "cand_1" } });
    fireEvent.click(within(panel).getByRole("button", { name: "Preview" }));
    await within(panel).findByText("Hi Rajesh");
    expect(sentBody(post, 1)).toMatchObject({ candidateId: "cand_1", subject: "A role for you, [First name]" });

    fireEvent.click(within(panel).getByRole("button", { name: "Send to each separately" }));
    expect(await screen.findByRole("heading", { name: "Queued for 1 person, each their own copy" })).toBeTruthy();
    expect(within(screen.getByRole("table", { name: "Not sent to" })).getByText("Farid Hassan")).toBeTruthy();
    expect(sentBody(post, 2)).toMatchObject({ checkOnly: false });
  });
});

describe("ticking contacts", () => {
  const contact = (id: string, fullName: string): ContactSummary => ({
    id, fullName, title: null, companyId: "comp_1", companyName: "Maju", email: `${id}@example.com`, phone: null, mobile: null,
    status: "active", emailCheckStatus: null,
  } as unknown as ContactSummary);
  const list = [contact("cont_1", "Mei Tan"), contact("cont_2", "Ali Hassan")];

  it("one row at a time by clicking it, or all shown; the name still opens the contact", () => {
    const onPick = vi.fn();
    renderWithClient(<ContactTable contacts={list} empty="none" picked={new Set(["cont_1"])} onPick={onPick} />);
    expect(screen.queryByRole("checkbox")).toBeNull();
    expect(screen.getByRole("row", { name: "Mei Tan (chosen)" }).className).toContain("chosen");
    fireEvent.click(screen.getByRole("row", { name: "Ali Hassan" }).querySelector("td:nth-child(2)") as Element);
    expect(onPick).toHaveBeenLastCalledWith([list[1]], true);
    fireEvent.click(screen.getByRole("row", { name: "Mei Tan (chosen)" }).querySelector("td:nth-child(2)") as Element);
    expect(onPick).toHaveBeenLastCalledWith([list[0]], false);
    onPick.mockClear();
    fireEvent.click(screen.getByRole("link", { name: "Ali Hassan" }));
    expect(onPick).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Select all shown" }));
    expect(onPick).toHaveBeenLastCalledWith(list, true);
  });

  it("does not choose rows for those who may not send email", async () => {
    renderWithClient(<ContactTable contacts={list} empty="none" />);
    await waitFor(() => { expect(screen.queryByRole("button", { name: "Select all shown" })).toBeNull(); });
    expect(screen.queryByRole("row", { name: "Mei Tan" })).toBeNull();
  });
});
