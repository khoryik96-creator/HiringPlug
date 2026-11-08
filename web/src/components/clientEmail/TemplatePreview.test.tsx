import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { failure, ok, renderWithClient } from "../../test-utils";
import { TemplatePreview } from "./TemplatePreview";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("template preview", () => {
  it("says why when the preview cannot be made", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: [{ id: "cand_1", fullName: "Siti Aisyah", currentTitle: null }], nextCursor: null }));
    vi.spyOn(api, "POST").mockResolvedValue(failure("VALIDATION_FAILED", "The subject is too long."));
    renderWithClient(<TemplatePreview subject="Hi" html="<p>Hello</p>" bannerUrls={{}} />);
    fireEvent.change(screen.getByLabelText("Candidate to preview with"), { target: { value: "Siti" } });
    fireEvent.click(await screen.findByRole("button", { name: "Siti Aisyah" }));
    expect((await screen.findByRole("alert")).textContent).toContain("The subject is too long.");
  });
});
