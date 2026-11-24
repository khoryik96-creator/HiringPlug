import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { whatsAppTemplatesKey } from "../api/keys";
import { WhatsAppTemplates } from "../pages/WhatsAppTemplates";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { chatLink, WhatsAppButton, WhatsAppDialog } from "./WhatsApp";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
  if (typeof HTMLDialogElement.prototype.close !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value(this: HTMLDialogElement) { this.open = false; } });
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const me = { permissions: ["emails:send", "notes:create", "pipeline:read"] };
const templates = {
  items: [{ id: "wtpl_1", version: 1, name: "Introduce a role", body: "Hi {{candidate.firstName}}, {{job.title}}?", createdAt: "", updatedAt: "" }],
  placeholders: ["candidate.firstName", "job.title"],
  fields: [{ key: "candidate.firstName", label: "First name" }, { key: "job.title", label: "Job title" }],
};
const applications = { items: [{ id: "appl_1", jobId: "job_1", jobTitle: "Finance Manager", companyName: "Contoh Berhad" }], nextCursor: null };

function mockGets() {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/me" ? me : path === "/v1/whatsapp-templates" ? templates : applications,
  ))) as never);
}

describe("WhatsApp", () => {
  it("makes the chat link with the message typed in", () => {
    expect(chatLink("60123456789", "Hi Siti & co?")).toBe("https://wa.me/60123456789?text=Hi%20Siti%20%26%20co%3F");
    expect(chatLink("60123456789", "")).toBe("https://wa.me/60123456789");
  });

  it("fills a template for a candidate's job, opens WhatsApp and keeps the message as a note", async () => {
    mockGets();
    const opened = vi.spyOn(window, "open").mockReturnValue(null);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string, init: { body: Record<string, unknown> }) => Promise.resolve(ok(
      path === "/v1/notes" ? { id: "note_1" }
        : { text: init.body.templateId ? "Hi Siti, Finance Manager?" : "", missing: [], numbers: [{ label: "Phone", number: "+60123456789", chatNumber: "60123456789" }] },
    ))) as never);
    const onClose = vi.fn();
    renderWithClient(<WhatsAppDialog subject={{ kind: "candidate", id: "cand_1", name: "Siti Aminah" }} onClose={onClose} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    fireEvent.change(await screen.findByLabelText("Job"), { target: { value: "appl_1" } });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "wtpl_1" } });
    await waitFor(() => { expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toBe("Hi Siti, Finance Manager?"); });
    const drafted = post.mock.calls.map((c) => (c[1] as { body: Record<string, unknown> }).body).at(-1);
    expect(drafted).toMatchObject({ candidateId: "cand_1", templateId: "wtpl_1", applicationId: "appl_1" });

    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Hi Siti, call at 3?" } });
    fireEvent.click(screen.getByRole("button", { name: "Open in WhatsApp" }));
    expect(opened).toHaveBeenCalledWith("https://wa.me/60123456789?text=Hi%20Siti%2C%20call%20at%203%3F", "_blank", "noopener,noreferrer");
    await waitFor(() => { expect(onClose).toHaveBeenCalled(); });
    expect(sentBody(post, post.mock.calls.length - 1)).toEqual({ subjectType: "candidate", subjectId: "cand_1", kind: "whatsapp", body: "WhatsApp to +60123456789:\nHi Siti, call at 3?" });
  });

  it("is on every candidate's page: with no number saved it says what it needs", async () => {
    mockGets();
    renderWithClient(<WhatsAppButton subject={{ kind: "candidate", id: "cand_1", name: "Siti Aisyah" }} hasNumber={false} />);
    const button = await screen.findByRole<HTMLButtonElement>("button", { name: "WhatsApp" });
    expect(button.disabled).toBe(true);
    expect(button.title).toContain("Add their phone number");
  });

  it("opens the chat with no message when the message is left blank", async () => {
    mockGets();
    const opened = vi.spyOn(window, "open").mockReturnValue(null);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/notes" ? { id: "note_1" } : { text: "", missing: [], numbers: [{ label: "Phone", number: "+60123456789", chatNumber: "60123456789" }] },
    ))) as never);
    const onClose = vi.fn();
    renderWithClient(<WhatsAppDialog subject={{ kind: "candidate", id: "cand_1", name: "Siti Aminah" }} onClose={onClose} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    const send = screen.getByRole<HTMLButtonElement>("button", { name: "Open in WhatsApp" });
    await waitFor(() => { expect(send.disabled).toBe(false); });
    fireEvent.click(send);
    expect(opened).toHaveBeenCalledWith("https://wa.me/60123456789", "_blank", "noopener,noreferrer");
    await waitFor(() => { expect(onClose).toHaveBeenCalled(); });
    expect(sentBody(post, post.mock.calls.length - 1)).toMatchObject({ kind: "whatsapp", body: "WhatsApp chat opened with +60123456789 (no message typed)." });
  });

  it("opens the chat with nothing typed in when the message is only spaces, as the note says", async () => {
    mockGets();
    const opened = vi.spyOn(window, "open").mockReturnValue(null);
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/notes" ? { id: "note_1" } : { text: "", missing: [], numbers: [{ label: "Phone", number: "+60123456789", chatNumber: "60123456789" }] },
    ))) as never);
    const onClose = vi.fn();
    renderWithClient(<WhatsAppDialog subject={{ kind: "contact", id: "cont_1", name: "Aisyah Contoh" }} onClose={onClose} />);
    expect(await screen.findByText("To phone +60123456789")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "  \n\t " } });
    fireEvent.click(screen.getByRole("button", { name: "Open in WhatsApp" }));
    expect(opened).toHaveBeenCalledWith("https://wa.me/60123456789", "_blank", "noopener,noreferrer");
    await waitFor(() => { expect(onClose).toHaveBeenCalled(); });
    expect(sentBody(post, post.mock.calls.length - 1)).toMatchObject({ kind: "whatsapp", body: "WhatsApp chat opened with +60123456789 (no message typed)." });
  });

  it("cannot send without a valid number", async () => {
    mockGets();
    vi.spyOn(api, "POST").mockResolvedValue(ok({ text: "", missing: [], numbers: [{ label: "Mobile", number: "12" }] }));
    renderWithClient(<WhatsAppDialog subject={{ kind: "contact", id: "cont_1", name: "Aisyah" }} onClose={vi.fn()} />);
    expect(await screen.findByText(/No valid phone number/)).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Hello" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Open in WhatsApp" }).disabled).toBe(true);
  });

  it("adds a WhatsApp template, its field chips putting the field at the cursor", async () => {
    mockGets();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(templates.items[0]));
    renderWithClient(<WhatsAppTemplates canCreate canManage />);
    expect(await screen.findByText("Hi {{candidate.firstName}}, {{job.title}}?")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Add a template" }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Shortlisted" } });
    const message = screen.getByLabelText<HTMLTextAreaElement>("Message");
    fireEvent.change(message, { target: { value: "Hi , you are shortlisted." } });
    message.focus();
    message.setSelectionRange(3, 3);
    fireEvent.click(screen.getByRole("button", { name: "First name" }));
    expect(message.value).toBe("Hi {{candidate.firstName}}, you are shortlisted.");
    expect(document.activeElement).toBe(message);
    expect(message.selectionStart).toBe(26); // just after the field
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ name: "Shortlisted", body: "Hi {{candidate.firstName}}, you are shortlisted." });
  });

  it("renames a template from Change or rename in Settings", async () => {
    mockGets();
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...templates.items[0], name: "Introduce a job", version: 2 }));
    renderWithClient(<WhatsAppTemplates canCreate={false} canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Change or rename" }));
    const name = screen.getByLabelText<HTMLInputElement>("Name");
    expect(name.value).toBe("Introduce a role");
    fireEvent.change(name, { target: { value: "Introduce a job" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ version: 1, name: "Introduce a job", body: "Hi {{candidate.firstName}}, {{job.title}}?" });
    expect(screen.queryByRole("button", { name: "Add a template" })).toBeNull();
  });

  it("in Settings, a chip clicked before clicking in the message puts the field at the end", async () => {
    mockGets();
    renderWithClient(<WhatsAppTemplates canCreate={false} canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Change or rename" }));
    const message = screen.getByLabelText<HTMLTextAreaElement>("Message");
    message.setSelectionRange(0, 0); // what a browser reports for a box never in focus
    fireEvent.click(screen.getByRole("button", { name: "First name" }));
    expect(message.value).toBe("Hi {{candidate.firstName}}, {{job.title}}?{{candidate.firstName}}");
  });

  it("in Settings, saves with the version the form opened with, after the list loads again", async () => {
    const get = mockGets();
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...templates.items[0], version: 3 }));
    const { client } = renderWithClient(<WhatsAppTemplates canCreate={false} canManage />);
    fireEvent.click(await screen.findByRole("button", { name: "Change or rename" }));
    get.mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/whatsapp-templates"
      ? { ...templates, items: [{ ...templates.items[0], version: 2, body: "Their change" }, { ...templates.items[0], id: "wtpl_2", name: "Theirs" }] } : me))) as never);
    await act(() => client.invalidateQueries({ queryKey: whatsAppTemplatesKey }));
    expect(await screen.findByRole("heading", { name: "Theirs" })).toBeTruthy(); // the list has loaded again
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ version: 1, body: "Hi {{candidate.firstName}}, {{job.title}}?" });
  });
});
