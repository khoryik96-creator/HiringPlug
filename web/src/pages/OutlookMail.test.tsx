import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { OutlookMail } from "./OutlookMail";

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
