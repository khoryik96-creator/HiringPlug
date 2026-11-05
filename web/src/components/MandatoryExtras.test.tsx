import { cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { CandidateExtras, extrasBody } from "./MandatoryExtras";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Harness() {
  const [values, setValues] = useState<Record<string, string>>({});
  return <><CandidateExtras values={values} onChange={setValues} /><output>{JSON.stringify(extrasBody(values))}</output></>;
}

describe("mandatory fields on quick add", () => {
  it("asks only for the company's mandatory fields not already on the form", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings([
      { form: "candidate", field: "email" }, { form: "candidate", field: "source" }, { form: "candidate", field: "noticePeriod" },
      { form: "candidate", field: "location" }, { form: "job", field: "location" },
    ])));
    renderWithClient(<Harness />);
    fireEvent.change(await screen.findByLabelText("Source"), { target: { value: "referral" } });
    fireEvent.change(screen.getByLabelText("Notice period"), { target: { value: "2_week" } });
    fireEvent.change(screen.getByLabelText("Location"), { target: { value: " Penang " } });
    expect(screen.queryByLabelText("Email")).toBeNull(); // the quick form already asks for it
    expect(JSON.parse(screen.getByRole("status").textContent)).toEqual({
      source: "referral", noticePeriod: { value: 2, unit: "week" }, location: "Penang",
    });
  });
});
