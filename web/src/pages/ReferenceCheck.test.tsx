import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
import { ReferenceCheck } from "./ReferenceCheck";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("reference check", () => {
  it("asks the company's questions and saves the answers as a reference check note", async () => {
    const c = testCandidate();
    const settings = testFieldSettings() as unknown as { lists: { name: string; options: unknown[] }[] };
    settings.lists.push({ name: "reference_check", label: "Reference check", kind: "open", options: [
      { key: "referee", label: "Referee", hidden: false, builtIn: true },
      { key: "rehire", label: "Would you rehire them?", hidden: false, builtIn: true },
      { key: "old", label: "Hidden question", hidden: true, builtIn: false },
    ] } as never);
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(path === "/v1/settings/fields" ? settings : c))) as never);
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ id: "note_1" }));
    renderWithClient(<ReferenceCheck candidateId={c.id} permissions={["notes:create"]} />);
    fireEvent.change(await screen.findByLabelText("Referee"), { target: { value: "Ahmad, Finance Manager at Contoh" } });
    fireEvent.change(screen.getByLabelText("Would you rehire them?"), { target: { value: "Yes, without a doubt" } });
    expect(screen.queryByLabelText("Hidden question")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save the reference check" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({
      subjectType: "candidate", subjectId: c.id, kind: "reference", sensitive: false,
      body: "Referee: Ahmad, Finance Manager at Contoh\nWould you rehire them?: Yes, without a doubt",
    });
    expect((await screen.findByRole("status")).textContent).toContain("Saved");
  });
});
