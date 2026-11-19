import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api, ApiError } from "../../api/client";
import * as files from "../../api/files";
import { ok, renderWithClient } from "../../test-utils";
import { OutlookAttachments } from "./OutlookAttachments";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
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
  expect(screen.queryByRole("button", { name: "Save Nadia – CV.pdf to candidate" })).toBeNull();
});

it.each([false, true])("saves to the confirmed candidate and handles exact duplicates (%s)", async (duplicate) => {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/outlook/messages/{messageId}/attachments" ? ok({
      items: [{ id: "cvFile", fileName: "Nadia – CV.pdf", sizeBytes: 4400, downloadable: true, savable: true }],
      more: false,
    }) : ok({
      items: [{ id: "cand_nadia", fullName: "Nadia Aziz", email: "nadia@example.test", currentTitle: "QA Engineer" }],
    }),
  ));
  const cv = new Blob(["Synthetic CV"]);
  const fetchFile = vi.spyOn(files, "fetchFile").mockResolvedValue(cv);
  const upload = vi.spyOn(files, "uploadCandidateDocument").mockResolvedValue(undefined);
  if (duplicate) {
    upload.mockRejectedValueOnce(new ApiError({
      error: { code: "DOCUMENT_DUPLICATE", message: "Exact duplicate", requestId: "req_test" },
    }));
  }

  renderWithClient(<OutlookAttachments messageId="omg_test" canSaveCv />);
  fireEvent.click(await screen.findByRole("button", { name: "Save Nadia – CV.pdf to candidate" }));
  const submit = screen.getByRole("button", { name: "Save to CV & files" });
  expect(submit.hasAttribute("disabled")).toBe(true);
  fireEvent.change(screen.getByLabelText("Candidate"), { target: { value: "Nadia" } });
  fireEvent.click(await screen.findByRole("button", { name: /Nadia Aziz.*nadia@example.test/ }));
  expect(fetchFile).not.toHaveBeenCalled();
  fireEvent.click(submit);
  expect((await screen.findByRole("status")).textContent).toBe(
    duplicate ? "This CV is already in Nadia Aziz’s profile." : "CV saved to Nadia Aziz’s profile.",
  );
  expect(fetchFile).toHaveBeenCalledWith("/v1/outlook/messages/omg_test/attachments/cvFile/file", "application/octet-stream");
  expect(upload).toHaveBeenCalledWith("cand_nadia", "original", cv, "Nadia – CV.pdf");
  expect(screen.getByRole("link", { name: "Open candidate" }).getAttribute("href")).toBe("#/candidates/cand_nadia?tab=cv");
});

it("does not offer Save CV for a file over the upload limit, which can still be downloaded", async () => {
  vi.spyOn(api, "GET").mockResolvedValue(ok({
    items: [{ id: "scan", fileName: "Scanned CV.pdf", sizeBytes: 15 * 1024 ** 2, downloadable: true, savable: false }],
    more: false,
  }));
  renderWithClient(<OutlookAttachments messageId="omg_test" canSaveCv />);
  expect(await screen.findByRole("button", { name: "Download Scanned CV.pdf" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Save Scanned CV.pdf to candidate" })).toBeNull();
});
