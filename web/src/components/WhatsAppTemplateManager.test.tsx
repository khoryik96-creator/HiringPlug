import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { whatsAppTemplatesKey } from "../api/keys";
import { renderWithClient } from "../test-utils";
import { WhatsAppDialog } from "./WhatsApp";
import { allowDialogs, mockWhatsAppApi } from "./whatsAppTestApi";

beforeAll(allowDialogs);
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const all = ["emails:send", "notes:create", "pipeline:read", "emails.templates:create", "emails.templates:manage"];
const siti = { kind: "candidate" as const, id: "cand_1", name: "Siti Aminah" };
const templateNames = () => within(screen.getByLabelText("Template")).getAllByRole("option").map((o) => o.textContent);

async function openTemplates() {
  renderWithClient(<WhatsAppDialog subject={siti} onClose={vi.fn()} />);
  expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
  fireEvent.click(await screen.findByRole("button", { name: "Templates" }));
  return screen.findByRole("region", { name: "Templates" });
}

describe("WhatsApp window: Templates", () => {
  it("adds a template, with a field put in at the cursor, and the Template dropdown shows it", async () => {
    const { post } = mockWhatsAppApi(all);
    const pane = await openTemplates();
    fireEvent.click(within(pane).getByRole("button", { name: "Add a template" }));
    const form = within(pane).getByRole("group", { name: "New template" });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Follow up" } });
    const body = within(form).getByLabelText<HTMLTextAreaElement>("Message");
    fireEvent.change(body, { target: { value: "Hi , any news?" } });
    body.focus();
    body.setSelectionRange(3, 3);
    fireEvent.click(within(form).getByRole("button", { name: "First name" }));
    expect(body.value).toBe("Hi {{candidate.firstName}}, any news?");
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));
    expect(await within(pane).findByRole("group", { name: "Follow up" })).toBeTruthy();
    expect(post).toHaveBeenCalledWith("/v1/whatsapp-templates", { body: { name: "Follow up", body: "Hi {{candidate.firstName}}, any news?" } });

    fireEvent.click(within(pane).getByRole("button", { name: "Back to the message" }));
    expect(templateNames()).toEqual(["Blank message", "Introduce a role", "Follow up"]);
  });

  it("renames and changes a template", async () => {
    const { put } = mockWhatsAppApi(all);
    const pane = await openTemplates();
    const row = await within(pane).findByRole("group", { name: "Introduce a role" });
    fireEvent.click(within(row).getByRole("button", { name: "Change or rename" }));
    fireEvent.change(within(row).getByLabelText("Name"), { target: { value: "Introduce a job" } });
    fireEvent.change(within(row).getByLabelText("Message"), { target: { value: "Hello {{candidate.firstName}}" } });
    fireEvent.click(within(row).getByRole("button", { name: "Save" }));
    expect(await within(pane).findByRole("group", { name: "Introduce a job" })).toBeTruthy();
    expect(put).toHaveBeenCalledWith("/v1/whatsapp-templates/{templateId}", {
      params: { path: { templateId: "wtpl_1" } }, body: { version: 1, name: "Introduce a job", body: "Hello {{candidate.firstName}}" },
    });
    fireEvent.click(within(pane).getByRole("button", { name: "Back to the message" }));
    expect(templateNames()).toEqual(["Blank message", "Introduce a job"]);
  });

  it("saves a change with the version the form opened with, even when the list has loaded again since", async () => {
    const { put, store } = mockWhatsAppApi(all);
    const { client } = renderWithClient(<WhatsAppDialog subject={siti} onClose={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Templates" }));
    const row = await screen.findByRole("group", { name: "Introduce a role" });
    fireEvent.click(within(row).getByRole("button", { name: "Change or rename" }));
    fireEvent.change(within(row).getByLabelText("Message"), { target: { value: "Hello {{candidate.firstName}}" } });

    // Someone else changes it in Settings; coming back to the tab loads the list again.
    store.splice(0, 1, { id: "wtpl_1", version: 2, name: "Introduce a role", body: "Their change", createdAt: "", updatedAt: "" });
    store.push({ id: "wtpl_2", version: 1, name: "Theirs", body: "Hello", createdAt: "", updatedAt: "" });
    await act(() => client.invalidateQueries({ queryKey: whatsAppTemplatesKey }));
    expect(await screen.findByRole("group", { name: "Theirs" })).toBeTruthy(); // the list has loaded again

    fireEvent.click(within(row).getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(put.mock.calls[0]?.[1]).toMatchObject({ body: { version: 1 } }); // so the server says it changed meanwhile
  });

  it("goes back to the message on Escape, keeping it, and moves focus with each pane", async () => {
    mockWhatsAppApi(all);
    const onClose = vi.fn();
    renderWithClient(<WhatsAppDialog subject={siti} onClose={onClose} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    const escape = () => { fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true })); };
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Hi Siti, a long message" } });

    fireEvent.click(screen.getByRole("button", { name: "Save as template" }));
    expect(document.activeElement).toBe(await screen.findByLabelText("Name"));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Half named" } });
    escape();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("Hi Siti, a long message");
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Save as template" }));

    fireEvent.click(screen.getByRole("button", { name: "Templates" }));
    expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Templates" }));
    fireEvent.click(screen.getByRole("button", { name: "Back to the message" }));
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Templates" }));
    fireEvent.click(screen.getByRole("button", { name: "Templates" }));
    escape();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("Hi Siti, a long message");

    // At the message itself, Escape closes the window.
    escape();
    expect(onClose).toHaveBeenCalled();
  });

  it("asks before deleting, and deleting the chosen template sets the dropdown back to Blank message, keeping the message", async () => {
    const { del } = mockWhatsAppApi(all);
    renderWithClient(<WhatsAppDialog subject={siti} onClose={vi.fn()} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "wtpl_1" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Introduce a role"); });

    fireEvent.click(screen.getByRole("button", { name: "Templates" }));
    const row = await screen.findByRole("group", { name: "Introduce a role" });
    const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false);
    fireEvent.click(within(row).getByRole("button", { name: "Delete" }));
    expect(confirm).toHaveBeenCalledWith('Delete the "Introduce a role" template?');
    expect(del).not.toHaveBeenCalled();

    confirm.mockReturnValueOnce(true);
    fireEvent.click(within(row).getByRole("button", { name: "Delete" }));
    await waitFor(() => { expect(screen.queryByRole("group", { name: "Introduce a role" })).toBeNull(); });
    expect(del).toHaveBeenCalledWith("/v1/whatsapp-templates/{templateId}", { params: { path: { templateId: "wtpl_1" } } });

    fireEvent.click(screen.getByRole("button", { name: "Back to the message" }));
    expect(screen.getByLabelText<HTMLSelectElement>("Template").value).toBe("");
    expect(templateNames()).toEqual(["Blank message"]);
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Introduce a role");
  });

  it("shows no Change or Delete without emails.templates:manage, and no Add without emails.templates:create", async () => {
    mockWhatsAppApi(["emails:send", "notes:create", "emails.templates:create"]);
    const pane = await openTemplates();
    const row = await within(pane).findByRole("group", { name: "Introduce a role" });
    expect(within(row).queryByRole("button", { name: "Change or rename" })).toBeNull();
    expect(within(row).queryByRole("button", { name: "Delete" })).toBeNull();
    expect(within(pane).getByRole("button", { name: "Add a template" })).toBeTruthy();
    cleanup();

    mockWhatsAppApi(["emails:send", "notes:create", "emails.templates:manage"]);
    const managing = await openTemplates();
    expect(await within(managing).findByRole("button", { name: "Change or rename" })).toBeTruthy();
    expect(within(managing).queryByRole("button", { name: "Add a template" })).toBeNull();
  });
});
