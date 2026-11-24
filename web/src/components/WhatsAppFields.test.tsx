import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { renderWithClient, sentBody } from "../test-utils";
import { WhatsAppDialog } from "./WhatsApp";
import { allowDialogs, mockWhatsAppApi } from "./whatsAppTestApi";

beforeAll(allowDialogs);
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const all = ["emails:send", "notes:create", "pipeline:read", "emails.templates:create", "emails.templates:manage"];
const siti = { kind: "candidate" as const, id: "cand_1", name: "Siti Aminah" };

async function openWindow() {
  renderWithClient(<WhatsAppDialog subject={siti} onClose={vi.fn()} />);
  expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
  return screen.getByLabelText<HTMLTextAreaElement>("Message");
}

describe("WhatsApp window: Insert field", () => {
  it("puts the person's value at the cursor, not at the end, and keeps typing there", async () => {
    mockWhatsAppApi(all);
    const message = await openWindow();
    fireEvent.change(message, { target: { value: "Hi , the role is yours." } });
    message.focus(); // they click in the message
    message.setSelectionRange(3, 3);
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "candidate.firstName" } });
    expect(message.value).toBe("Hi Siti, the role is yours.");
    expect(document.activeElement).toBe(message);
    expect(message.selectionStart).toBe(7);
    expect(message.selectionEnd).toBe(7);

    // Over a selection: the selected text is replaced.
    message.setSelectionRange(21, 26); // "yours"
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "job.title" } });
    expect(message.value).toBe("Hi Siti, the role is Finance Manager.");
  });

  it("puts a field at the end of a message they have not clicked in yet, not at its start", async () => {
    mockWhatsAppApi(all);
    const message = await openWindow();
    fireEvent.change(message, { target: { value: "Hi there" } });
    message.setSelectionRange(0, 0); // what a browser reports for a box never in focus
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "candidate.firstName" } });
    expect(message.value).toBe("Hi thereSiti");

    // Shown again after Templates: still not clicked in, so the end again.
    fireEvent.click(screen.getByRole("button", { name: "Templates" }));
    fireEvent.click(await screen.findByRole("button", { name: "Back to the message" }));
    const again = screen.getByLabelText<HTMLTextAreaElement>("Message");
    again.setSelectionRange(0, 0);
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "job.title" } });
    expect(again.value).toBe("Hi thereSitiFinance Manager");
  });

  it("with the keyboard, moves through the fields without putting them in, and puts one in on Enter", async () => {
    mockWhatsAppApi(all);
    const message = await openWindow();
    fireEvent.change(message, { target: { value: "Hi " } });
    const dropdown = screen.getByLabelText<HTMLSelectElement>("Insert field");
    dropdown.focus();
    // Some browsers change a closed dropdown straight from the arrow keys.
    fireEvent.keyDown(dropdown, { key: "ArrowDown" });
    fireEvent.change(dropdown, { target: { value: "candidate.firstName" } });
    fireEvent.keyDown(dropdown, { key: "ArrowDown" });
    fireEvent.change(dropdown, { target: { value: "candidate.fullName" } });
    expect(message.value).toBe("Hi ");
    expect(dropdown.value).toBe("candidate.fullName");
    expect(document.activeElement).toBe(dropdown);

    fireEvent.keyDown(dropdown, { key: "Enter" });
    expect(message.value).toBe("Hi Siti Aminah");
    expect(document.activeElement).toBe(message);
    expect(dropdown.value).toBe("");
  });

  it("lists the fields by label and shows those without a value disabled as (empty)", async () => {
    mockWhatsAppApi(all);
    await openWindow();
    const dropdown = screen.getByLabelText("Insert field");
    const options = within(dropdown).getAllByRole<HTMLOptionElement>("option");
    expect(options.map((o) => o.textContent)).toEqual(["Insert field…", "First name", "Full name", "Nationality (empty)", "Job title", "Client company", "Your name"]);
    expect(options.find((o) => o.textContent === "Nationality (empty)")?.disabled).toBe(true);
    expect(options.find((o) => o.textContent === "First name")?.disabled).toBe(false);
  });

  it("counts an inserted field as typed, so choosing a template still fills the message again", async () => {
    mockWhatsAppApi(all);
    const message = await openWindow();
    fireEvent.change(screen.getByLabelText("Insert field"), { target: { value: "candidate.fullName" } });
    expect(message.value).toBe("Siti Aminah");
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "wtpl_1" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Introduce a role"); });
  });
});

describe("WhatsApp window: Save as template", () => {
  it("turns the person's values back into fields, whole words only, and saves it with a name", async () => {
    const { post } = mockWhatsAppApi(all, { takenName: "Introduce a role" });
    const message = await openWindow();
    fireEvent.change(message, { target: { value: "Hi Siti Aminah! Siti, the Finance Manager role at Contoh Berhad near Sitiawan. Nur Rekrut" } });
    fireEvent.click(screen.getByRole("button", { name: "Save as template" }));

    const text = await screen.findByLabelText<HTMLTextAreaElement>("Template text");
    expect(text.value).toBe("Hi {{candidate.fullName}}! {{candidate.firstName}}, the {{job.title}} role at {{company.name}} near Sitiawan. {{sender.name}}");
    fireEvent.change(text, { target: { value: `${text.value} Thanks!` } });

    // A name already used: the server says so, and the form stays.
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Introduce a role" } });
    fireEvent.click(screen.getByRole("button", { name: "Save template" }));
    expect((await screen.findByRole("alert")).textContent).toContain("already exists");

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Role near home" } });
    fireEvent.click(screen.getByRole("button", { name: "Save template" }));
    expect(await screen.findByText(/Saved as the .Role near home. template/)).toBeTruthy();
    const saved = post.mock.calls.filter((c) => c[0] === "/v1/whatsapp-templates").map((c) => sentBody({ mock: { calls: [c] } }));
    expect(saved.at(-1)).toEqual({
      name: "Role near home",
      body: "Hi {{candidate.fullName}}! {{candidate.firstName}}, the {{job.title}} role at {{company.name}} near Sitiawan. {{sender.name}} Thanks!",
    });
    // Back at the message, as it was; the new template is in the dropdown.
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toContain("near Sitiawan");
    await waitFor(() => { expect(within(screen.getByLabelText("Template")).getByRole("option", { name: "Role near home" })).toBeTruthy(); });
  });

  it("turns a field that had no value ([job.title]) back into a field", async () => {
    mockWhatsAppApi(all);
    const message = await openWindow();
    fireEvent.change(message, { target: { value: "Hi Siti, I have a [job.title] role at [company.name]." } });
    fireEvent.click(screen.getByRole("button", { name: "Save as template" }));
    const text = await screen.findByLabelText<HTMLTextAreaElement>("Template text");
    expect(text.value).toBe("Hi {{candidate.firstName}}, I have a {{job.title}} role at {{company.name}}.");
  });

  it("has no Save as template or Templates buttons without the template permissions", async () => {
    mockWhatsAppApi(["emails:send", "notes:create", "pipeline:read"]);
    await openWindow();
    await screen.findByLabelText("Job"); // permissions are loaded
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Hi Siti" } });
    expect(screen.queryByRole("button", { name: "Save as template" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Templates" })).toBeNull();
    expect(screen.getByLabelText("Insert field")).toBeTruthy();
  });

  it("offers Templates but not Save as template to someone who may only manage", async () => {
    mockWhatsAppApi(["emails:send", "notes:create", "pipeline:read", "emails.templates:manage"]);
    await openWindow();
    expect(await screen.findByRole("button", { name: "Templates" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Save as template" })).toBeNull();
  });
});
