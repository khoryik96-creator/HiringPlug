import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Contact } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { EmailSuggestion } from "./EmailSuggestion";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const ana = { id: "cont_ana", version: 3, email: null } as Contact;

describe("a suggested email", () => {
  it("is offered for a contact without one, and used (then verified)", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ suggestion: { email: "ana.lim@acme.test", pattern: "first.last", matches: 3, of: 4 } }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...ana, version: 4, email: "ana.lim@acme.test" }));
    renderWithClient(<EmailSuggestion contact={ana} canEdit />);
    expect((await screen.findByRole("status")).textContent).toContain("Suggested email: ana.lim@acme.test (3 of 4 colleagues' addresses are spelled first.last@)");
    fireEvent.click(screen.getByRole("button", { name: "Use and verify" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 3, email: "ana.lim@acme.test" });
  });

  it("is not asked for when the contact has an email, and says nothing without a pattern", async () => {
    const get = vi.spyOn(api, "GET").mockResolvedValue(ok({ suggestion: null }));
    const { rerender } = renderWithClient(<EmailSuggestion contact={{ ...ana, email: "ana@acme.test" }} canEdit />);
    expect(get).not.toHaveBeenCalled();
    rerender(<EmailSuggestion contact={ana} canEdit />);
    await waitFor(() => { expect(get).toHaveBeenCalled(); });
    expect(screen.queryByRole("status")).toBeNull();
  });
});
