import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Application } from "../api/client";
import { failure, renderWithClient } from "../test-utils";
import { MoveMenu } from "./ApplicationCard";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const app: Application = {
  channel: null, id: "appl_1", version: 3, jobId: "jo_1", jobTitle: "Auditor", companyId: "comp_1", companyName: "Contoh Berhad",
  candidateId: "cand_1", candidateName: "Siti Aminah", candidateTitle: null, candidateAvailability: null, stage: "applied",
  reason: null, reasonNote: null, category: null, stageChangedAt: "2026-11-20T00:00:00Z", createdAt: "2026-11-20T00:00:00Z", updatedAt: "2026-11-20T00:00:00Z",
};

describe("taking a candidate off a job", () => {
  it("asks first, then sends the version shown", async () => {
    const asked = vi.spyOn(window, "confirm").mockReturnValue(true);
    const del = vi.spyOn(api, "DELETE").mockResolvedValue({ data: undefined, error: undefined, response: new Response(null, { status: 204 }) });
    renderWithClient(<MoveMenu app={app} busy={false} onMove={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Move Siti Aminah on Auditor"), { target: { value: "__remove" } });
    await waitFor(() => { expect(del).toHaveBeenCalled(); });
    expect(asked).toHaveBeenCalledWith(expect.stringContaining("Take Siti Aminah off Auditor?"));
    expect(del.mock.calls[0]?.[1]).toEqual({ params: { path: { applicationId: "appl_1" }, query: { version: 3 } } });
  });

  it("says why when it is kept", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.spyOn(api, "DELETE").mockResolvedValue(failure("APPLICATION_NOT_REMOVABLE", "This application has an interview: move it to Withdrawn instead.", undefined, 409));
    renderWithClient(<MoveMenu app={app} busy={false} onMove={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Move Siti Aminah on Auditor"), { target: { value: "__remove" } });
    expect(await screen.findByText(/move it to Withdrawn instead/)).toBeTruthy();
  });

  it("does nothing when not confirmed", () => {
    vi.spyOn(window, "confirm").mockReturnValue(false);
    const del = vi.spyOn(api, "DELETE");
    renderWithClient(<MoveMenu app={app} busy={false} onMove={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Move Siti Aminah on Auditor"), { target: { value: "__remove" } });
    expect(del).not.toHaveBeenCalled();
  });
});
