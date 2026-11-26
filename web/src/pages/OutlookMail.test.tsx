import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { listLimit, OutlookMail } from "./OutlookMail";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
  localStorage.clear(); // where each test's mail was left
});

it("finds mail from the candidate context and explicitly links the selected conversation", async () => {
  window.location.hash = "#/outlook?candidateId=cand_test&search=nadia%40example.test";
  const message = {
    id: "omg_test", folderId: "omf_inbox", conversation: "graph-thread", subject: "Interview on Tuesday",
    sender: "nadia@example.test", receivedAt: "2026-11-18T01:00:00Z", isRead: true, isDraft: false,
  };
  const get = vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({
      connected: true, enabled: true, folders: [{ id: "omf_inbox", name: "Inbox", kind: "inbox" }], items: [message],
    }) : ok({ text: "Tuesday works for me." }),
  ));
  const link = vi.spyOn(api, "PUT").mockResolvedValue({ response: new Response(null, { status: 204 }) });
  renderWithClient(<OutlookMail/>);
  fireEvent.click(await screen.findByRole("button", { name: /Interview on Tuesday/ }));
  expect(await screen.findByText("Tuesday works for me.")).toBeTruthy();
  expect(get.mock.calls[0]?.[1]).toMatchObject({ params: { query: { search: "nadia@example.test" } } });
  fireEvent.click(screen.getByRole("button", { name: "Link to candidate" }));
  await waitFor(() => { expect(link).toHaveBeenCalled(); });
  expect(sentBody(link)).toEqual({ candidateId: "cand_test" });
  expect(await screen.findByText("Conversation linked to this profile.")).toBeTruthy();
  const post = vi.spyOn(api, "POST").mockResolvedValue({ response: new Response(null, { status: 202 }) });
  fireEvent.click(screen.getByRole("button", { name: "Reply" }));
  fireEvent.change(screen.getByLabelText("Reply to sender"), { target: { value: "Tuesday works." } });
  fireEvent.click(screen.getByRole("button", { name: "Save in Outlook Drafts" }));
  expect(await screen.findByText("Reply saved in Outlook Drafts.")).toBeTruthy();
  expect(sentBody(post)).toMatchObject({ text: "Tuesday works.", mode: "reply", saveDraft: true });
});

const forwardSource = {
  id: "omg_original", folderId: "omf_inbox", conversation: "thread", subject: "Availability for interview",
  sender: "nadia@example.test", receivedAt: "2026-11-18T01:00:00Z", isRead: true, isDraft: false,
};
const forwardDraft: Schemas["OutlookDraft"] = {
  messageId: "omg_forward", version: "review-version", subject: "FW: Availability for interview",
  body: "Please review.\n\nI can make Tuesday.", to: ["james@example.test", "lina@example.test"], cc: [], bcc: [],
  hasAttachments: true, bodyTruncated: false, schedule: null, scheduleNeedsReview: false, webUrl: null,
};

function showForwardInbox(draft = forwardDraft) {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({
      connected: true, enabled: true, folders: [
        { id: "omf_inbox", name: "Inbox", kind: "inbox" }, { id: "omf_drafts", name: "Drafts", kind: "drafts" },
      ],
      items: [forwardSource, { ...forwardSource, id: "omg_other", subject: "Another message" }],
    }) : path === "/v1/outlook/drafts/{messageId}" ? ok(draft)
      : path === "/v1/outlook/messages/{messageId}/attachments" ? ok({
        items: [{ id: "cvFile", fileName: "Nadia CV.pdf", sizeBytes: 4400, downloadable: true, savable: true }], more: false,
      }) : ok({ text: "I can make Tuesday." }),
  ));
  renderWithClient(<OutlookMail/>);
}

it("opens the native forward draft with its original attachment before any send", async () => {
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok(forwardDraft));
  const put = vi.spyOn(api, "PUT");
  showForwardInbox();
  fireEvent.click(await screen.findByRole("button", { name: /Availability for interview/ }));
  fireEvent.click(screen.getByRole("button", { name: "Forward" }));
  expect(document.activeElement).toBe(screen.getByLabelText("To"));
  fireEvent.change(screen.getByLabelText("To"), { target: { value: "james@example.test; lina@example.test" } });
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "Please review." } });
  fireEvent.click(screen.getByRole("button", { name: "Review forward" }));

  expect(await screen.findByRole("dialog", { name: "Outlook draft" })).toBeTruthy();
  expect(await screen.findByRole("button", { name: "Download Nadia CV.pdf" })).toBeTruthy();
  expect(screen.getByLabelText<HTMLInputElement>("To").value).toBe("james@example.test, lina@example.test");
  expect(screen.getByRole("textbox", { name: "Message" }).textContent).toContain("I can make Tuesday.");
  expect(post).toHaveBeenCalledWith("/v1/outlook/messages/{messageId}/forward", {
    params: { path: { messageId: forwardSource.id } },
    body: { to: forwardDraft.to, note: "Please review." },
  });
  expect(put).not.toHaveBeenCalled();
});

it("does not open the previous message's forward after changing messages", async () => {
  let finish = () => {};
  const post = vi.spyOn(api, "POST").mockImplementation(() => new Promise((resolve) => {
    finish = () => { resolve(ok(forwardDraft)); };
  }));
  showForwardInbox();
  fireEvent.click(await screen.findByRole("button", { name: /Availability for interview/ }));
  fireEvent.click(screen.getByRole("button", { name: "Forward" }));
  fireEvent.change(screen.getByLabelText("To"), { target: { value: "james@example.test" } });
  fireEvent.click(screen.getByRole("button", { name: "Review forward" }));
  await waitFor(() => { expect(post).toHaveBeenCalledTimes(1); });
  fireEvent.click(screen.getByRole("button", { name: /Another message/ }));
  await act(async () => { finish(); await Promise.resolve(); });
  await waitFor(() => { expect(screen.getByRole("heading", { name: "Another message" })).toBeTruthy(); });
  expect(screen.queryByRole("dialog")).toBeNull();
});

it("opens the existing ATS composer and its profile context from Outlook", async () => {
  window.location.hash = "#/outlook?contactId=cont_test";
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({ connected: true, enabled: true, folders: [], items: [] })
      : path === "/v1/mailbox" ? ok({ connected: true, address: "recruiter@example.test", provider: "microsoft" })
        : ok({ items: [], fields: [] }),
  ));
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ subject: "", bodyHtml: "", missing: [], fieldValues: {} }));
  renderWithClient(<OutlookMail permissions={["candidates:read", "documents:read", "emails.templates:create"]} />);
  fireEvent.click(await screen.findByRole("button", { name: "Write with HiringPlug" }));
  expect(screen.getByRole("dialog", { name: "Write with HiringPlug" })).toBeTruthy();
  expect(screen.getByLabelText("Template")).toBeTruthy();
  expect(screen.getByText("Candidates and CVs (0)")).toBeTruthy();
  expect(screen.getByRole("button", { name: "Preview email" })).toBeTruthy();
  await waitFor(() => {
    expect(post.mock.calls[0]?.[0]).toBe("/v1/email-previews");
    expect(sentBody(post)).toMatchObject({ contactId: "cont_test" });
  });
});

it.each([
  ["Reply", false],
  ["Reply all", true],
] as const)("opens %s with the caret ready to type above the quoted message", async (action, replyAll) => {
  const draft: Schemas["OutlookDraft"] = {
    ...forwardDraft, subject: "RE: Availability for interview", body: "I can make Tuesday.",
    bodyHtml: "<p><br></p><blockquote>I can make Tuesday.</blockquote>", hasAttachments: false,
    to: replyAll ? ["nadia@example.test", "lina@example.test"] : ["nadia@example.test"],
  };
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok(draft));
  const send = vi.spyOn(api, "PUT");
  showForwardInbox(draft);
  fireEvent.click(await screen.findByRole("button", { name: /Availability for interview/ }));
  fireEvent.click(screen.getByRole("button", { name: action }));
  const message = await screen.findByRole("textbox", { name: "Message" });
  expect(document.activeElement).toBe(message);
  expect(message.querySelector("blockquote")?.textContent).toBe("I can make Tuesday.");
  expect(screen.getByLabelText<HTMLInputElement>("To").value).toBe(draft.to.join(", "));
  const sendNow = screen.getByRole<HTMLButtonElement>("button", { name: "Send now" });
  expect(sendNow.disabled).toBe(true);
  fireEvent.input(message, { target: { innerHTML: "<p><b>Tuesday works.</b></p><blockquote>I can make Tuesday.</blockquote>" } });
  expect(sendNow.disabled).toBe(false);
  expect(post).toHaveBeenCalledWith("/v1/outlook/messages/{messageId}/reply-draft", {
    params: { path: { messageId: forwardSource.id } }, body: { replyAll },
  });
  expect(send).not.toHaveBeenCalled();
});

it("keeps the message list to about half of what the folders leave, so dragging back starts where the bar is", () => {
  expect(listLimit(1280, 200)).toBe(550);
  expect(listLimit(900, 200)).toBe(341);
  expect(listLimit(600, 200), "never narrower than the list's least").toBe(260);
  expect(listLimit(4000, 200), "nor wider than its most").toBe(760);
});

it("opens again where the person left it: their folder, message and open draft", async () => {
  window.location.hash = "#/outlook";
  const folders = [{ id: "omf_inbox", name: "Inbox", kind: "inbox" }, { id: "omf_drafts", name: "Drafts", kind: "drafts" }];
  const inbox = { id: "omg_in", folderId: "omf_inbox", conversation: "t1", subject: "Interview on Tuesday", sender: "nadia@example.test", receivedAt: "2026-11-18T01:00:00Z", isRead: true, isDraft: false };
  const drafted = { id: "omg_d", folderId: "omf_drafts", conversation: "t2", subject: "Shortlist for Grab", sender: "me@example.test", receivedAt: "2026-11-18T02:00:00Z", isRead: true, isDraft: true };
  const draft: Schemas["OutlookDraft"] = { messageId: "omg_d", version: "v", subject: "Shortlist for Grab", body: "Three profiles.", to: [], cc: [], bcc: [],
    hasAttachments: false, bodyTruncated: false, schedule: null, scheduleNeedsReview: false, webUrl: null };
  const get = vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: { folder?: string } } }) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({ connected: true, enabled: true, folders,
      items: init?.params?.query?.folder === "omf_drafts" ? [drafted] : [inbox] })
      : path === "/v1/outlook/drafts/{messageId}" ? ok(draft) : ok({ items: [], more: false, fields: [] }))) as never);
  renderWithClient(<OutlookMail userId="usr_1" />);
  fireEvent.click(await screen.findByRole("button", { name: "Drafts" }));
  fireEvent.click(await screen.findByRole("button", { name: /Shortlist for Grab/ }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
  cleanup(); // away to another page
  get.mockClear();
  renderWithClient(<OutlookMail userId="usr_1" />);
  expect(await screen.findByRole("dialog")).toBeTruthy(); // the draft, open again
  const listed = (get.mock.calls as unknown[][]).find((c) => c[0] === "/v1/outlook/mail");
  expect(listed?.[1]).toMatchObject({ params: { query: { folder: "omf_drafts" } } });
  cleanup();
  renderWithClient(<OutlookMail userId="usr_2" />); // someone else's mail starts at their inbox
  await screen.findByRole("button", { name: /Interview on Tuesday/ });
  expect(screen.queryByRole("dialog")).toBeNull();
});
