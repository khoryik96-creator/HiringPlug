import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { OutlookMail } from "./OutlookMail";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
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

it.each([
  ["#/outlook?candidateId=cand_test", true],
  ["#/outlook?candidateId=cand_test&search=nadia%40example.test", false],
])("suggests the page's candidate for Save CV only in their linked mail (%s)", async (hash, suggested) => {
  window.location.hash = hash;
  const message = {
    id: "omg_cv", folderId: "omf_inbox", conversation: "graph-thread", subject: "My CV",
    sender: "someone@example.test", receivedAt: "2026-11-18T01:00:00Z", isRead: true, isDraft: false, hasAttachments: true,
  };
  const get = vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({
      connected: true, enabled: true, folders: [{ id: "omf_inbox", name: "Inbox", kind: "inbox" }], items: [message],
    }) : path === "/v1/outlook/messages/{messageId}/attachments" ? ok({
      items: [{ id: "cvFile", fileName: "CV.pdf", sizeBytes: 4400, downloadable: true, savable: true }], more: false,
    }) : path === "/v1/candidates/{candidateId}" ? ok({ id: "cand_test", fullName: "Nadia Aziz" })
      : ok({ text: "Attached." }),
  ));
  renderWithClient(<OutlookMail canSaveCv/>);
  fireEvent.click(await screen.findByRole("button", { name: /My CV/ }));
  fireEvent.click(await screen.findByRole("button", { name: "Save CV.pdf to candidate" }));
  const submit = screen.getByRole("button", { name: "Save to CV & files" });
  if (suggested) {
    await waitFor(() => { expect(submit.hasAttribute("disabled")).toBe(false); });
  } else {
    expect(submit.hasAttribute("disabled")).toBe(true);
  }
  expect((get.mock.calls as unknown[][]).some((call) => call[0] === "/v1/candidates/{candidateId}")).toBe(suggested);
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

function showForwardInbox() {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/mail" ? ok({
      connected: true, enabled: true, folders: [
        { id: "omf_inbox", name: "Inbox", kind: "inbox" }, { id: "omf_drafts", name: "Drafts", kind: "drafts" },
      ],
      items: [forwardSource, { ...forwardSource, id: "omg_other", subject: "Another message" }],
    }) : path === "/v1/outlook/drafts/{messageId}" ? ok(forwardDraft)
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
  expect(screen.getByLabelText<HTMLTextAreaElement>("Message").value).toContain("I can make Tuesday.");
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
