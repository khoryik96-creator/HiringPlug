import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate } from "../test-utils";
import { WorkAuthorizationPicker } from "./WorkAuthorizationPicker";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const suggested = testCandidate({ workAuthorization: null,
  workAuthorizationSuggestion: { status: "expat_eligible", basis: "rp_t", reason: "The CV mentions an RP-T (Residence Pass-Talent)" } });

describe("a work authorization suggested from the CV", () => {
  it("is striped, not chosen, until someone confirms it with a click", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate()));
    renderWithClient(<WorkAuthorizationPicker candidate={suggested} canEdit />);
    const expat = screen.getByRole("button", { name: "Spousal / RP-T" });
    expect(expat.className).toBe("wa-suggested");
    expect(expat.getAttribute("aria-pressed")).toBe("false");
    expect(screen.getByRole("button", { name: "Not known" }).getAttribute("aria-pressed"), "not shown as the answer beside a suggestion").toBe("false");
    expect(screen.getByText(/suggested from the CV \(The CV mentions an RP-T/)).toBeTruthy();
    fireEvent.click(expat);
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ workAuthorization: { status: "expat_eligible", basis: "rp_t" } });
  });

  it("reads as suggested for those who may not change it", () => {
    renderWithClient(<WorkAuthorizationPicker candidate={suggested} canEdit={false} />);
    expect(screen.getByText("RP-T · suggested from the CV").className).toBe("wa-suggested-chip");
  });

  it("is not shown once one is set", () => {
    renderWithClient(<WorkAuthorizationPicker candidate={{ ...suggested, workAuthorization: { status: "local", basis: null, note: null } }} canEdit />);
    expect(screen.queryByText(/suggested from the CV/)).toBeNull();
  });
});
