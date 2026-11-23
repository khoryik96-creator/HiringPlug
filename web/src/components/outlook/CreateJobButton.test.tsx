import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import * as files from "../../api/files";
import { JobList } from "../../pages/JobList";
import { ok, renderWithClient, testMe } from "../../test-utils";
import { handJd, takeJd } from "../jdHandover";
import { OutlookAttachments } from "./OutlookAttachments";

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.unstubAllGlobals(); takeJd(); window.location.hash = ""; });

it("creates a job from a job description attached to an email", async () => {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/me" ? ok(testMe(["intake:create", "jobs:create"])) : ok({
    items: [
      { id: "jd", fileName: "Finance Manager JD.docx", sizeBytes: 9000, downloadable: true, savable: true },
      { id: "logo", fileName: "logo.png", sizeBytes: 900, downloadable: true, savable: true },
    ],
    more: false,
  }))) as never);
  const fetched = vi.spyOn(files, "fetchFile").mockResolvedValue(new Blob(["Finance Manager, Contoh Berhad"]));
  renderWithClient(<OutlookAttachments messageId="omg_1" canImportCv userId="usr_1" />);
  fireEvent.click(await screen.findByRole("button", { name: "Create a job from Finance Manager JD.docx" }));
  await waitFor(() => { expect(window.location.hash).toBe("#/jobs"); });
  expect(fetched).toHaveBeenCalledWith("/v1/outlook/messages/omg_1/attachments/jd/file", "application/octet-stream");
  expect(takeJd()?.name).toBe("Finance Manager JD.docx");
  expect(screen.queryByRole("button", { name: "Create a job from logo.png" })).toBeNull(); // not a job description
});

it("opens Add job on Jobs with the job description handed over, reading it at once", async () => {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/me" ? ok(testMe(["jobs:create"]))
    : ok({ items: [], nextCursor: null, mandatory: [], lists: [] }))) as never);
  const send = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "jdf_1", fileName: "Finance Manager JD.docx", status: "reading", details: null }), { status: 201 }));
  vi.stubGlobal("fetch", send);
  handJd(new File(["Finance Manager"], "Finance Manager JD.docx"));
  renderWithClient(<JobList canCreate />);
  expect(await screen.findByRole("form", { name: "Add job" })).toBeTruthy();
  await waitFor(() => { expect(send).toHaveBeenCalledWith("/v1/jd-files", expect.objectContaining({ method: "POST" })); });
  expect(takeJd()).toBeNull(); // taken once
});
