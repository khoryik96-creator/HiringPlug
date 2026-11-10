import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { JdFiles } from "./JdFiles";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const jd = (id: string, fileName: string, contentType: string, status: Schemas["JdFile"]["status"] = "read"): Schemas["JdFile"] => ({
  id, jobId: "jo_1", fileName, contentType, sizeBytes: 14_744, status, error: null, details: null, createdAt: "2026-11-10T04:24:00Z",
});
const listed = [jd("jdf_1", "Hub Operations Manager JD.pdf", "application/pdf"), jd("jdf_2", "JD.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "reading")];

describe("job descriptions kept with a job", () => {
  it("are listed to view (PDF) or download (Word), and dropped in", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: listed }));
    const fetch = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify(listed[0]), { status: 201 }));
    renderWithClient(<JdFiles jobId="jo_1" permissions={["jobs:read", "jobs:update"]} />);
    expect(await screen.findByRole("button", { name: "Hub Operations Manager JD.pdf" })).toBeTruthy();
    expect(screen.getAllByText(/14 KB · added/)).toHaveLength(2);
    expect(screen.getByText(/reading…/)).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "View" })).toHaveLength(1); // a Word file is downloaded, not shown
    const del = screen.getByRole<HTMLButtonElement>("button", { name: "Delete JD.docx" });
    expect(del.disabled, "deleting is a permission of its own (admins)").toBe(true);

    fireEvent.change(screen.getByLabelText("Drop the client's JD to keep it here"), { target: { files: [new File(["%PDF"], "JD v2.pdf")] } });
    await waitFor(() => { expect(fetch).toHaveBeenCalled(); });
    expect(fetch.mock.calls[0]?.[0]).toBe("/v1/jobs/jo_1/jd-files");
  });

  it("are deleted by those given jobs.documents:delete; viewers only look", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: listed }));
    const del = vi.spyOn(api, "DELETE").mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) });
    vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<JdFiles jobId="jo_1" permissions={["jobs:read", "jobs.documents:delete"]} />);
    fireEvent.click(await screen.findByRole("button", { name: "Delete Hub Operations Manager JD.pdf" }));
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    expect(del.mock.calls[0]?.[1]).toEqual({ params: { path: { jdFileId: "jdf_1" } } });
    expect(screen.queryByLabelText("Drop the client's JD to keep it here"), "adding needs jobs:update").toBeNull();
  });
});
