import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../../api/client";
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
      { id: "cvFile", fileName: "Nadia – CV.pdf", sizeBytes: 4400, downloadable: true },
      { id: "linked", fileName: "Work samples", sizeBytes: 0, downloadable: false },
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
});
