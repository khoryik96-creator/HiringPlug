import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { fillExample, Invoicing } from "./Invoicing";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const settings: Schemas["InvoiceSettings"] = {
  issuer: "Hyppies", bankDetails: "", numberPrefix: "INV-", nextNumber: 1, taxLabel: "SST", taxPercent: "0",
  lineText: "Recruitment fee: {{candidate.name}}", footer: "", emailSubject: "Invoice {{invoice.number}}",
  emailBody: "Dear {{contact.firstName}},", sendHour: 9, version: 0,
  fields: [
    { key: "candidate.name", label: "Candidate", example: "Siti Nur Aisyah" },
    { key: "invoice.number", label: "Invoice number", example: "INV-0042" },
    { key: "contact.firstName", label: "Recipient's first name", example: "Aisyah" },
    { key: "invoice.total", label: "Total", example: "MYR 23,328.00" },
  ],
};

describe("the invoice template", () => {
  it("is edited with fields put where the cursor is, previewed, and saved", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(settings));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...settings, version: 1 }));
    renderWithClient(<Invoicing canManage />);
    const body = await screen.findByLabelText<HTMLTextAreaElement>(/Message/);
    expect(screen.getByText("Invoice INV-0001"), "the next number").toBeTruthy();
    fireEvent.change(screen.getByLabelText(/^Tax %/), { target: { value: "8" } });
    fireEvent.change(screen.getByLabelText(/^Next invoice number/), { target: { value: "42" } });
    expect(screen.getByText("The next invoice is INV-0042.")).toBeTruthy();
    body.setSelectionRange(body.value.length, body.value.length);
    fireEvent.blur(body);
    fireEvent.click(screen.getByRole("button", { name: "Total" }));
    expect(body.value).toBe("Dear {{contact.firstName}},{{invoice.total}}");
    expect(screen.getByText(/Dear Aisyah,MYR 23,328.00/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save invoice template" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ version: 0, taxPercent: "8", nextNumber: 42, emailBody: "Dear {{contact.firstName}},{{invoice.total}}" });
  });

  it("is read only for those who may not change settings", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(settings));
    renderWithClient(<Invoicing canManage={false} />);
    expect((await screen.findByLabelText<HTMLTextAreaElement>(/Message/)).disabled).toBe(true);
    expect(screen.queryByRole("button", { name: "Save invoice template" })).toBeNull();
  });

  it("fills examples", () => {
    expect(fillExample("{{candidate.name}} / {{ invoice.number }} / {{x.y}}", settings.fields)).toBe("Siti Nur Aisyah / INV-0042 / {{x.y}}");
  });
});
