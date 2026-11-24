import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { whatsAppTemplatesKey } from "../api/keys";
import { renderWithClient } from "../test-utils";
import { WhatsAppDialog } from "./WhatsApp";
import { allowDialogs, hold, mockWhatsAppApi } from "./whatsAppTestApi";

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

  it("closes only the form a change belonged to, keeping what is typed in another", async () => {
    const { post, store } = mockWhatsAppApi(all);
    store.push({ id: "wtpl_2", version: 1, name: "Old one", body: "Hello", createdAt: "", updatedAt: "" });
    const pane = await openTemplates();
    const row = await within(pane).findByRole("group", { name: "Introduce a role" });
    fireEvent.click(within(row).getByRole("button", { name: "Change or rename" }));
    fireEvent.change(within(row).getByLabelText("Message"), { target: { value: "Half typed" } });

    // Deleting another template leaves this form open.
    vi.spyOn(window, "confirm").mockReturnValue(true);
    fireEvent.click(within(within(pane).getByRole("group", { name: "Old one" })).getByRole("button", { name: "Delete" }));
    await waitFor(() => { expect(within(pane).queryByRole("group", { name: "Old one" })).toBeNull(); });
    expect(within(row).getByLabelText<HTMLTextAreaElement>("Message").value).toBe("Half typed");

    // A new template saved while this form was opened again leaves it open too.
    fireEvent.click(within(row).getByRole("button", { name: "Cancel" }));
    const release = hold(post, "/v1/whatsapp-templates");
    fireEvent.click(within(pane).getByRole("button", { name: "Add a template" }));
    const form = within(pane).getByRole("group", { name: "New template" });
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Follow up" } });
    fireEvent.change(within(form).getByLabelText("Message"), { target: { value: "Any news?" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save" }));
    fireEvent.click(within(row).getByRole("button", { name: "Change or rename" }));
    fireEvent.change(within(row).getByLabelText("Message"), { target: { value: "Typed again" } });
    release();
    expect(await within(pane).findByRole("group", { name: "Follow up" })).toBeTruthy();
    expect(within(row).getByLabelText<HTMLTextAreaElement>("Message").value).toBe("Typed again");
  });

  it("sets the dropdown back to Blank message when the chosen template is deleted after leaving the pane, or by someone else", async () => {
    const { post, del, store } = mockWhatsAppApi(all);
    const { client } = renderWithClient(<WhatsAppDialog subject={siti} onClose={vi.fn()} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    const lastDraft = () => post.mock.calls.filter((c) => c[0] === "/v1/whatsapp-drafts").map((c) => (c[1] as { body: Record<string, unknown> }).body).at(-1);
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "wtpl_1" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Introduce a role"); });

    fireEvent.click(screen.getByRole("button", { name: "Templates" }));
    const release = hold(del, "/v1/whatsapp-templates/{templateId}");
    vi.spyOn(window, "confirm").mockReturnValue(true);
    fireEvent.click(within(await screen.findByRole("group", { name: "Introduce a role" })).getByRole("button", { name: "Delete" }));
    fireEvent.click(screen.getByRole("button", { name: "Back to the message" })); // before the delete is done
    release();
    await waitFor(() => { expect(templateNames()).toEqual(["Blank message"]); });
    await waitFor(() => { expect(lastDraft()?.templateId).toBeUndefined(); }); // no later draft asks for the deleted template
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Introduce a role");

    // Someone else deletes the chosen template; the list loads again.
    store.push({ id: "wtpl_9", version: 1, name: "Theirs", body: "Hello", createdAt: "", updatedAt: "" });
    await act(() => client.invalidateQueries({ queryKey: whatsAppTemplatesKey }));
    await waitFor(() => { expect(templateNames()).toEqual(["Blank message", "Theirs"]); });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "wtpl_9" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Theirs"); });
    store.splice(0, store.length);
    await act(() => client.invalidateQueries({ queryKey: whatsAppTemplatesKey }));
    await waitFor(() => { expect(lastDraft()?.templateId).toBeUndefined(); });
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("From Theirs");
  });

  it("still says a template was saved when the pane was left while it was saving", async () => {
    const { post } = mockWhatsAppApi(all);
    renderWithClient(<WhatsAppDialog subject={siti} onClose={vi.fn()} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Hi Siti, any news?" } });
    fireEvent.click(screen.getByRole("button", { name: "Save as template" }));
    fireEvent.change(await screen.findByLabelText("Name"), { target: { value: "Any news" } });
    const release = hold(post, "/v1/whatsapp-templates");
    fireEvent.click(screen.getByRole("button", { name: "Save template" }));
    fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true })); // Escape, back to the message
    release();
    expect(await screen.findByText(/Saved as the .Any news. template/)).toBeTruthy();
    expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("Hi Siti, any news?");
  });
});
