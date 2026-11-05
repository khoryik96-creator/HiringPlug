import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { CandidateExtras, extrasBody } from "./MandatoryExtras";
import { SourcePicker } from "./SourcePicker";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Harness() {
  const [values, setValues] = useState<Record<string, string>>({});
  return <><CandidateExtras values={values} onChange={setValues} /><output>{JSON.stringify(extrasBody(values))}</output></>;
}

describe("mandatory fields on quick add", () => {
  it("asks for the pass held when the candidate may already work here", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings([{ form: "candidate", field: "workAuthorization" }])));
    renderWithClient(<Harness />);
    expect(screen.queryByLabelText("Pass held")).toBeNull();
    fireEvent.change(await screen.findByLabelText("Work authorization"), { target: { value: "expat_eligible" } });
    fireEvent.change(screen.getByLabelText("Pass held"), { target: { value: "rp_t" } });
    expect(JSON.parse(screen.getByRole("status").textContent)).toEqual({
      workAuthorization: { status: "expat_eligible", basis: "rp_t", note: null },
    });
  });

  it("asks only for the company's mandatory fields not already on the form", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings([
      { form: "candidate", field: "email" }, { form: "candidate", field: "source" }, { form: "candidate", field: "noticePeriod" },
      { form: "candidate", field: "location" }, { form: "job", field: "location" },
    ])));
    renderWithClient(<Harness />);
    fireEvent.click(within(await screen.findByRole("group", { name: "Source" })).getByRole("button", { name: /Referral/ }));
    fireEvent.change(screen.getByLabelText("Notice period"), { target: { value: "2_week" } });
    fireEvent.change(screen.getByLabelText("Location"), { target: { value: " Penang " } });
    expect(screen.queryByLabelText("Email")).toBeNull(); // the quick form already asks for it
    expect(JSON.parse(screen.getByRole("status").textContent)).toEqual({
      source: "referral", noticePeriod: { value: 2, unit: "week" }, location: "Penang",
    });
  });
});

describe("source picker", () => {
  it("shows the company's sources as icons side by side; picking the chosen one again clears it", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    let value = "linkedin";
    const { rerender } = renderWithClient(<SourcePicker value={value} onChange={(v) => { value = v; }} />);
    const group = await screen.findByRole("group", { name: "Source" });
    const pressed = () => within(group).getAllByRole("button").filter((b) => b.getAttribute("aria-pressed") === "true").map((b) => b.textContent);
    await waitFor(() => { expect(within(group).getAllByRole("button")).toHaveLength(6); });
    expect(pressed()).toEqual(["inLinkedIn"]);
    fireEvent.click(within(group).getByRole("button", { name: /LinkedIn/ }));
    expect(value).toBe("");
    rerender(<SourcePicker value="job_fair" onChange={() => undefined} />);
    expect(within(group).getByRole("button", { name: /job_fair/ }).getAttribute("aria-pressed")).toBe("true"); // no longer listed: still shown
  });
});
