import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { CandidateList } from "./CandidateList";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const summary = {
  id: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", fullName: "Siti Aisyah", email: null, phone: null, currentTitle: "Accountant",
  currentCompany: null, location: null, workAuthorizationStatus: null, tags: ["relocate"], updatedAt: "2026-11-01T00:00:00Z",
};

describe("candidate list", () => {
  it("filters by skill and tag from the values already in use, and shows tags by the name", async () => {
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => {
      if (path === "/v1/candidates/used-values") {
        return Promise.resolve(ok({ tags: ["relocate"], skills: ["SAP"], industries: [], specializations: [] }));
      }
      return Promise.resolve(ok({ items: [summary], nextCursor: null }));
    }));
    renderWithClient(<CandidateList canCreate={false} />);
    expect(await screen.findByText("Siti Aisyah")).toBeTruthy();
    expect(screen.getAllByText("relocate").some((el) => el.classList.contains("chip"))).toBe(true);

    fireEvent.change(await screen.findByLabelText("Skill"), { target: { value: "SAP" } });
    await screen.findByRole("option", { name: "relocate" });
    fireEvent.change(screen.getByLabelText("Tag"), { target: { value: "relocate" } });
    await waitFor(() => {
      expect(get).toHaveBeenCalledWith("/v1/candidates", { params: { query: { skill: "SAP", tag: "relocate" } } });
    });
  });
});
