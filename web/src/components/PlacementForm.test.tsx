import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { guardNumberFields } from "../numericInput";
import { guardRequiredFields } from "../requiredFields";
import { ok, RECRUITER, renderWithClient } from "../test-utils";
import { PlacementForm } from "./PlacementForm";

const stops: (() => void)[] = [];
beforeEach(() => { stops.push(guardRequiredFields(), guardNumberFields()); }); // as main.tsx does
afterEach(() => {
  for (const stop of stops.splice(0)) stop();
  cleanup();
  vi.restoreAllMocks();
});

const defaults: Schemas["PlacementDefaults"] = {
  companyId: "comp_1", companyName: "Maju Foods", jobTitle: "Finance Manager", jobType: "permanent", termsSet: false, feePercent: null,
  paymentTermsDays: 30, guaranteeDays: 90, contactId: null, tiers: [], salaryCurrency: "MYR",
};

describe("recording a placement", () => {
  it("outlines what is missing instead of greying out the button, and takes only numbers in number fields", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/applications/{applicationId}/placement-defaults" ? defaults : { items: [], nextCursor: null },
    ))) as never);
    const post = vi.spyOn(api, "POST");
    renderWithClient(<PlacementForm applicationId="appl_1" permissions={RECRUITER.concat("placements.fees:update")} onDone={() => undefined} />);
    await waitFor(() => { expect(screen.getByLabelText<HTMLInputElement>("Job title").value).toBe("Finance Manager"); });

    const record = screen.getByRole<HTMLButtonElement>("button", { name: "Record placement" });
    expect(record.disabled).toBe(false);
    fireEvent.click(record);
    const start = screen.getByLabelText<HTMLInputElement>("Start date");
    expect(post).not.toHaveBeenCalled();
    expect(start.hasAttribute("data-missing")).toBe(true); // outlined in red
    expect(screen.getByLabelText("Job title").hasAttribute("data-missing")).toBe(false);
    fireEvent.input(start, { target: { value: "2026-12-01" } });
    expect(start.hasAttribute("data-missing")).toBe(false);

    const fee = screen.getByLabelText<HTMLInputElement>("Fee %");
    const typed = new InputEvent("beforeinput", { data: "x", inputType: "insertText", bubbles: true, cancelable: true });
    expect(fee.dispatchEvent(typed)).toBe(false); // a letter does not go in
  });
});
