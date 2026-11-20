import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { ok, renderWithClient, sentBody } from "../../test-utils";
import { zonedInput } from "../calendar/dates";
import { OutlookDraftEditor } from "./OutlookDraftEditor";
import { OutlookScheduledList } from "./OutlookScheduledList";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

const schedule: Schemas["OutlookScheduledSend"] = {
  id: "oms_test", messageId: "omg_test", subject: "Tuesday interview", status: "scheduled",
  version: 2, sendAt: "2026-11-20T01:00:00Z", updatedAt: "2026-11-19T00:00:00Z", error: null,
};

it("submits the revised draft and new time in the same request", async () => {
  const draft: Schemas["OutlookDraft"] = {
    messageId: "omg_test", version: "opened", subject: "Interview", body: "Tuesday is available.",
    bodyHtml: "<p><strong>Tuesday</strong><br/> is available.</p>",
    to: ["nadia@example.test"], cc: [], bcc: [], hasAttachments: true, bodyTruncated: false,
    schedule, scheduleNeedsReview: false, webUrl: null,
  };
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(path === "/v1/outlook/messages/{messageId}/attachments"
    ? ok({ items: [], more: false }) : ok(draft)));
  const patch = vi.spyOn(api, "PATCH");
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(schedule));
  const onClose = vi.fn();
  renderWithClient(<OutlookDraftEditor id="omg_test" onClose={onClose}/>);
  fireEvent.blur(await screen.findByRole("textbox", { name: "Message" }));
  fireEvent.change(await screen.findByLabelText("Subject"), { target: { value: "Tuesday interview" } });
  fireEvent.click(screen.getByRole("button", { name: "Change time" }));
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const future = new Date(Math.ceil(Date.now() / 60000) * 60000 + 7200000);
  fireEvent.change(screen.getByLabelText(/Send at/), { target: { value: zonedInput(future, zone) } });
  fireEvent.click(screen.getByRole("button", { name: "Update schedule" }));
  await waitFor(() => { expect(onClose).toHaveBeenCalledWith("schedule"); });
  expect(patch).not.toHaveBeenCalled();
  expect(sentBody(put)).toEqual({ expectedVersion: schedule.version, draftVersion: "opened", sendAt: future.toISOString(),
    changes: { subject: "Tuesday interview" } });
});

it("cancels only a scheduled send, using its displayed version", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [schedule, { ...schedule, id: "oms_confirming", status: "confirming" }] }));
  const cancel = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...schedule, status: "cancelled" }));
  renderWithClient(<OutlookScheduledList onOpen={vi.fn()}/>);
  const buttons = await screen.findAllByRole("button", { name: "Cancel send" });
  expect(buttons).toHaveLength(1);
  if (buttons[0]) fireEvent.click(buttons[0]);
  await waitFor(() => { expect(cancel).toHaveBeenCalled(); });
  expect(sentBody(cancel)).toEqual({ version: 2 });
});

it("keeps the editor open when Escape is pressed during submission", async () => {
  const draft: Schemas["OutlookDraft"] = {
    messageId: "omg_test", version: "opened", subject: "Interview", body: "Tuesday is available.",
    to: ["nadia@example.test"], cc: [], bcc: [], hasAttachments: false, bodyTruncated: false,
    schedule: null, scheduleNeedsReview: false, webUrl: null,
  };
  vi.spyOn(api, "GET").mockResolvedValue(ok(draft));
  let finish = () => {};
  const put = vi.spyOn(api, "PUT").mockImplementation(() => new Promise((resolve) => {
    finish = () => { resolve(ok(schedule)); };
  }));
  const onClose = vi.fn();
  renderWithClient(<OutlookDraftEditor id="omg_test" onClose={onClose} />);
  fireEvent.click(await screen.findByRole("button", { name: "Send now" }));
  await waitFor(() => { expect(put).toHaveBeenCalledTimes(1); });
  fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
  expect(onClose).not.toHaveBeenCalled();
  finish();
  await waitFor(() => { expect(onClose).toHaveBeenCalledExactlyOnceWith("send"); });
});

it("opens later schedule pages and checks uncertain sends without submitting mail", async () => {
  const uncertain = { ...schedule, id: "oms_uncertain", status: "uncertain" as const };
  const get = vi.spyOn(api, "GET").mockResolvedValueOnce(ok({ items: [schedule], nextOffset: 50 }))
    .mockResolvedValue(ok({ items: [uncertain], nextOffset: null }));
  const resolve = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...uncertain, status: "needs_review" }));
  renderWithClient(<OutlookScheduledList onOpen={vi.fn()} />);
  fireEvent.click(await screen.findByRole("button", { name: "Next" }));
  fireEvent.click(await screen.findByRole("button", { name: "Check Outlook again" }));
  await waitFor(() => { expect(resolve).toHaveBeenCalledTimes(1); });
  expect(get).toHaveBeenCalledWith("/v1/outlook/scheduled", { params: { query: { offset: 50 } } });
  expect(resolve.mock.calls[0]?.[0]).toBe("/v1/outlook/scheduled/{sendId}/resolve");
  expect(sentBody(resolve)).toEqual({ version: 2 });
  fireEvent.click(screen.getByRole("button", { name: "Previous" }));
  await waitFor(() => { expect(get).toHaveBeenLastCalledWith("/v1/outlook/scheduled", { params: { query: { offset: 0 } } }); });
});

it("creates a draft on the first attachment and keeps unsaved text when removing it", async () => {
  const draft: Schemas["OutlookDraft"] = {
    messageId: "omg_test", version: "created", subject: "Interview", body: "Tuesday is available.",
    to: [], cc: [], bcc: [], hasAttachments: false, bodyTruncated: false,
    schedule: null, scheduleNeedsReview: false, webUrl: null,
  };
  const file = new File(["%PDF-test"], "Resume.pdf", { type: "application/pdf" });
  const post = vi.spyOn(api, "POST").mockImplementation((path) => Promise.resolve(
    path === "/v1/email-previews" ? ok({ subject: "", bodyHtml: "", missing: [], fieldValues: {} }) : ok(draft)));
  const upload = vi.fn<(url: string, options: RequestInit) => Promise<Response>>().mockResolvedValue(new Response(JSON.stringify({ ...draft, hasAttachments: true, version: "attached" })));
  vi.stubGlobal("fetch", upload);
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/messages/{messageId}/attachments" ? ok({ items: [{
      id: "test-file", fileName: file.name, sizeBytes: file.size, downloadable: true, savable: true,
    }], more: false }) : path === "/v1/email-signature" ? ok({ bodyHtml: "", version: 0, updatedAt: null }) : ok({ items: [], fields: [] })));
  const remove = vi.spyOn(api, "DELETE").mockResolvedValue(ok({ ...draft, version: "removed" }));
  vi.spyOn(window, "confirm").mockReturnValue(true);
  const send = vi.spyOn(api, "PUT");
  renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} />);
  fireEvent.change(screen.getByLabelText("Subject"), { target: { value: draft.subject } });
  const message = screen.getByRole("textbox", { name: "Message" });
  fireEvent.input(message, { target: { innerHTML: "<p><strong>Tuesday</strong> is available.</p>" } });
  fireEvent.change(screen.getByLabelText("Attachment file"), { target: { files: [file] } });
  expect(await screen.findByText(file.name)).toBeTruthy();
  const creates = (post.mock.calls as unknown[][]).filter((call) => call[0] === "/v1/outlook/drafts");
  expect(creates).toHaveLength(1);
  expect((creates[0]?.[1] as { body: unknown } | undefined)?.body).toMatchObject({ subject: draft.subject, bodyHtml: "<p><strong>Tuesday</strong> is available.</p>" });
  const [url, options] = upload.mock.calls[0] ?? [];
  expect(url).toBe("/v1/outlook/drafts/omg_test/attachments?version=created");
  expect(options?.method).toBe("POST");
  expect(options?.body instanceof FormData && options.body.get("file")).toEqual(file);
  fireEvent.input(message, { target: { innerHTML: "<p>What about <em>Wednesday</em>?</p>" } });
  fireEvent.click(screen.getByRole("button", { name: "Remove Resume.pdf" }));
  await waitFor(() => { expect(screen.queryByText(file.name)).toBeNull(); });
  expect(remove).toHaveBeenCalledWith("/v1/outlook/drafts/{messageId}/attachments/{attachmentId}", {
    params: { path: { messageId: draft.messageId, attachmentId: "test-file" }, query: { version: "attached" } },
  });
  expect(message.innerHTML).toBe("<p>What about <em>Wednesday</em>?</p>");
  expect(send).not.toHaveBeenCalled();
});

it("opens a new message with the caret in the message, not on a resize bar", () => {
  // As a browser does: opening the window focuses its first control.
  vi.spyOn(HTMLDialogElement.prototype, "showModal").mockImplementation(function (this: HTMLDialogElement) {
    this.open = true;
    this.querySelector<HTMLElement>('button, input, select, textarea, [tabindex="0"], [contenteditable="true"]')?.focus();
  });
  vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [], fields: [] }));
  vi.spyOn(api, "POST").mockResolvedValue(ok({ subject: "", bodyHtml: "", missing: [], salary: false, fieldValues: {} }));
  renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} />);
  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Message" }));
  const bars = screen.getAllByRole("separator");
  expect(bars.map((b) => b.getAttribute("aria-label"))).toEqual(["Editor width, left edge", "Editor width, right edge"]);
  expect(bars[0]?.compareDocumentPosition(screen.getByRole("textbox", { name: "Message" })) ?? 0, "after the content")
    .toBe(Node.DOCUMENT_POSITION_PRECEDING);
});
