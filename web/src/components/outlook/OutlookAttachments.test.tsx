import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import * as files from "../../api/files";
import { ok, renderWithClient, testMe } from "../../test-utils";
import { CvImportIndicator, titleWithCounts } from "./CvImportIndicator";
import { OutlookAttachments } from "./OutlookAttachments";
import { OutlookMessageRow } from "./OutlookMessageList";
import { resetCvImports } from "./outlookCvImports";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  resetCvImports();
  window.localStorage.removeItem("hiringplug.outlook.cvImports");
});

it("shows Outlook-only attachments and downloads a file from the current message", async () => {
  const list = vi.spyOn(api, "GET").mockResolvedValue(ok({
    items: [
      { id: "cvFile", fileName: "Nadia – CV.pdf", sizeBytes: 4400, downloadable: true, savable: true },
      { id: "linked", fileName: "Work samples", sizeBytes: 0, downloadable: false, savable: false },
    ],
    more: true,
  }));
  const download = vi.spyOn(files, "downloadFrom").mockResolvedValue(undefined);

  renderWithClient(<OutlookAttachments messageId="omg_test" />);
  fireEvent.click(await screen.findByRole("button", { name: "Download Nadia – CV.pdf" }));
  await waitFor(() => {
    expect(download).toHaveBeenCalledWith("/v1/outlook/messages/omg_test/attachments/cvFile/file", "Nadia – CV.pdf", "application/octet-stream");
  });
  expect(list).toHaveBeenCalledWith("/v1/outlook/messages/{messageId}/attachments", expect.objectContaining({
    params: { path: { messageId: "omg_test" } },
  }));
  expect(screen.getByText("Available in Outlook")).toBeTruthy();
  expect(screen.getByText("Open in Outlook to see the remaining attachments.")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Download Work samples" })).toBeNull();
  expect(screen.queryByRole("button", { name: /Save/ })).toBeNull();
  expect(screen.queryByRole("button", { name: "Import Nadia – CV.pdf as a candidate" })).toBeNull();
});

const me = testMe(["intake:create"]);
const nadia = { id: "cvFile", fileName: "Nadia.PDF", sizeBytes: 4000, downloadable: true, savable: true };
const importItem = (status: Schemas["CvImportItemStatus"], error: string | null = null): Schemas["CvImportItem"] => ({
  id: "cvit_1", fileName: "Nadia.PDF", status, candidateId: status === "failed" ? null : "cand_nadia",
  candidateName: status === "failed" ? null : "Nadia Aziz", error, updatedAt: "2026-11-20T05:00:00Z",
});
const counts = { reading: 0, created: 0, added: 0, unread: 0, failed: 0, duplicate: 0 };

/** The API for an import: the message's files, the import made, then its progress (reading, then outcome). */
function serveImport(outcome: Schemas["CvImportItem"]) {
  let polls = 0;
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/me" ? ok(me)
      : path === "/v1/outlook/messages/{messageId}/attachments" ? ok({
        items: [nadia, { id: "zip", fileName: "Work samples.zip", sizeBytes: 4000, downloadable: true, savable: true },
          { id: "oversize", fileName: "Large CV.pdf", sizeBytes: 15 * 1024 ** 2, downloadable: true, savable: false }], more: false,
      })
        : path === "/v1/cv-imports" ? ok({ items: [{ id: "cvi_1", source: "direct", createdAt: "2026-11-20T05:00:00Z", counts,
          items: [polls++ ? outcome : importItem("reading")] }] })
          : ok({ lists: [] }),
  ));
  const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "cvi_1", source: "direct", createdAt: "2026-11-20T05:00:00Z", counts, items: [] }));
  vi.spyOn(files, "fetchFile").mockResolvedValue(new Blob(["Synthetic CV"], { type: "application/pdf" }));
  const upload = vi.fn(() => Promise.resolve(new Response(JSON.stringify(importItem("reading")), { status: 201 })));
  vi.stubGlobal("fetch", upload);
  return { post, upload };
}

it("imports a CV in place: importing, then Uploaded, with the candidate to open", async () => {
  const { post, upload } = serveImport(importItem("created"));
  renderWithClient(<OutlookAttachments messageId="omg_test" canImportCv userId={me.user.id} />);
  const start = await screen.findByRole("button", { name: "Import Nadia.PDF as a candidate" });
  expect(screen.queryByRole("button", { name: /Import (Work samples|Large CV)/ }), "only CV files Outlook can hand over").toBeNull();
  fireEvent.click(start);

  const importing = await screen.findByLabelText("Importing…: Nadia.PDF");
  expect(importing.className).toContain("importing");
  expect(post).toHaveBeenCalledWith("/v1/cv-imports", { body: { source: "direct" } });
  expect(upload).toHaveBeenCalledWith("/v1/cv-imports/cvi_1/files", expect.objectContaining({ method: "POST" }));

  const done = await screen.findByLabelText("Uploaded: Nadia.PDF", {}, { timeout: 5000 });
  expect(done.className).toContain("done");
  fireEvent.click(done);
  const who = await screen.findByRole("link", { name: "Nadia Aziz" });
  expect(who.getAttribute("href")).toBe("#/candidates/cand_nadia");
  expect(screen.getByRole("status").textContent).toBe("New candidate: Nadia Aziz");
});

it("shows a failed import in red, with why and Try again", async () => {
  serveImport(importItem("failed", "This file is not a CV."));
  renderWithClient(<OutlookAttachments messageId="omg_test" canImportCv userId={me.user.id} />);
  const start = await screen.findByRole("button", { name: "Import Nadia.PDF as a candidate" });
  fireEvent.click(start);
  const failed = await screen.findByLabelText("Failed: Nadia.PDF", {}, { timeout: 5000 });
  expect(failed.className).toContain("failed");
  expect(screen.getByRole("status").textContent).toContain("This file is not a CV.");
  expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
});

it("outlines the message a CV came from, and counts imports beside Settings", async () => {
  serveImport(importItem("created"));
  const message: Schemas["OutlookMessage"] = { id: "omg_test", folderId: "omf_inbox", conversation: "t", subject: "My CV", sender: "nadia@example.test",
    receivedAt: "2026-11-18T01:00:00Z", isRead: true, isDraft: false, hasAttachments: true, participants: [], webUrl: null, senderName: "Nadia", flagged: false };
  renderWithClient(<><CvImportIndicator userId={me.user.id} /><OutlookAttachments messageId="omg_test" canImportCv userId={me.user.id} />
    <OutlookMessageRow item={message} selected={false} folders={[]} cvImport="done" onOpen={vi.fn()} onMove={vi.fn()} onFlag={vi.fn()} /></>);
  expect(document.querySelector(".outlook-row.cv-done")).not.toBeNull();
  const start = await screen.findByRole("button", { name: "Import Nadia.PDF as a candidate" });
  fireEvent.click(start);
  await screen.findByLabelText("CV imports from Outlook: 1 importing");
  const indicator = await screen.findByLabelText("CV imports from Outlook: 1 imported", {}, { timeout: 5000 });
  expect(indicator.className).toContain("done");
  expect(document.title.startsWith("(1✓) ")).toBe(true);
});

it("puts the counts in the tab's title, and takes them out", () => {
  expect(titleWithCounts("HiringPlug", { importing: 1, done: 2, failed: 1 })).toBe("(1… 2✓ 1✗) HiringPlug");
  expect(titleWithCounts("(1… 2✓ 1✗) HiringPlug", { importing: 0, done: 0, failed: 0 })).toBe("HiringPlug");
});

it("shows an import cut off on its way (the tab closed) as failed, to try again", async () => {
  serveImport(importItem("created"));
  window.localStorage.setItem("hiringplug.outlook.cvImports", JSON.stringify([{ userId: me.user.id, messageId: "omg_test", attachmentId: "cvFile",
    fileName: "Nadia.PDF", status: "uploading", at: new Date(Date.now() - 10 * 60_000).toISOString(), seen: false }]));
  renderWithClient(<OutlookAttachments messageId="omg_test" canImportCv userId={me.user.id} />);
  const failed = await screen.findByLabelText("Failed: Nadia.PDF");
  expect(failed.className).toContain("failed");
  expect(screen.getByRole("status").textContent).toContain("interrupted");
  expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
});

it("follows an import still being read that is no longer among the latest, by its own address", async () => {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/messages/{messageId}/attachments" ? ok({ items: [nadia], more: false })
      : path === "/v1/cv-imports" ? ok({ items: [] })
        : path === "/v1/cv-imports/{cvImportId}" ? ok({ id: "cvi_old", source: "direct", createdAt: "2026-11-01T00:00:00Z", counts, items: [importItem("added")] })
          : ok({ lists: [] }),
  ));
  window.localStorage.setItem("hiringplug.outlook.cvImports", JSON.stringify([{ userId: me.user.id, messageId: "omg_test", attachmentId: "cvFile",
    fileName: "Nadia.PDF", status: "reading", importId: "cvi_old", itemId: "cvit_1", at: "2026-11-01T00:00:00Z", seen: false }]));
  renderWithClient(<OutlookAttachments messageId="omg_test" canImportCv userId={me.user.id} />);
  const done = await screen.findByLabelText("Uploaded: Nadia.PDF");
  fireEvent.click(done);
  expect((await screen.findByRole("status")).textContent).toBe("Added to Nadia Aziz");
});

it("reads finished imports once, not on every page", async () => {
  const get = vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/cv-imports" ? ok({ items: [{ id: "cvi_1", source: "direct", createdAt: "2026-11-20T05:00:00Z", counts, items: [importItem("created")] }] }) : ok({ lists: [] })));
  window.localStorage.setItem("hiringplug.outlook.cvImports", JSON.stringify([{ userId: me.user.id, messageId: "omg_test", attachmentId: "cvFile",
    fileName: "Nadia.PDF", status: "created", importId: "cvi_1", itemId: "cvit_1", candidateId: "cand_nadia", at: "2026-11-20T05:00:00Z", seen: true }]));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const view = render(<QueryClientProvider client={client}><CvImportIndicator userId={me.user.id} /></QueryClientProvider>);
  await waitFor(() => { expect(get).toHaveBeenCalledTimes(1); });
  view.unmount();
  render(<QueryClientProvider client={client}><CvImportIndicator userId={me.user.id} /></QueryClientProvider>);
  window.dispatchEvent(new Event("focus"));
  await new Promise((r) => setTimeout(r, 50));
  expect(get, "nothing being read: not fetched again").toHaveBeenCalledTimes(1);
});

it("takes the counts out of the tab's title when the badge goes (signed out)", async () => {
  serveImport(importItem("created"));
  document.title = "HiringPlug";
  window.localStorage.setItem("hiringplug.outlook.cvImports", JSON.stringify([{ userId: me.user.id, messageId: "omg_test", attachmentId: "cvFile",
    fileName: "Nadia.PDF", status: "failed", error: "Not a CV.", at: "2026-11-20T05:00:00Z", seen: false }]));
  const view = render(<QueryClientProvider client={new QueryClient()}><CvImportIndicator userId={me.user.id} /></QueryClientProvider>);
  await waitFor(() => { expect(document.title).toBe("(1✗) HiringPlug"); });
  view.unmount();
  expect(document.title).toBe("HiringPlug");
});
