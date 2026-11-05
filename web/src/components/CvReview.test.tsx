import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody, testCandidate } from "../test-utils";
import { CvReview } from "./CvReview";
import { noticeFrom, suggestions } from "./cvSuggestions";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const s = (value: string, quote = value) => ({ value, quote });

const profile: Schemas["ParsedProfile"] = {
  parser: "rules@1",
  fullName: s("Siti Aisyah Binti Ahmad", "SITI AISYAH BINTI AHMAD"),
  email: s("siti@example.com"),
  phone: s("+60 12-345 6789"),
  linkedinUrl: null,
  location: s("Petaling Jaya, Selangor"),
  nationality: s("MY", "Nationality: Malaysian"),
  residenceCountry: null, industry: null,
  noticePeriod: s("1 month", "Notice period: 1 month"),
  skills: [s("SAP FICO"), s("Excel", "Excel (Advanced)")],
  experience: [
    { title: s("Accountant"), company: s("Old Co"), dates: s("2016 - 2019"), startDate: "2016", endDate: "2019", current: false },
    { title: s("Senior Accountant"), company: s("Nestlé Malaysia Berhad"), dates: s("Jan 2020 – Present"), startDate: "2020-01", endDate: null, current: true },
  ],
  education: [{ qualification: s("Bachelor of Accounting"), institution: s("Universiti Malaya"), dates: s("2015"), endYear: 2015 }],
  dropped: 1,
};

describe("suggestions from a CV", () => {
  it("offers what differs, ticking only fields the profile does not have", () => {
    const candidate = testCandidate({ email: "siti@example.com", currentTitle: "Accountant", noticePeriod: null, location: null });
    const rows = suggestions(profile, candidate);
    expect(rows.map((r) => [r.key, r.value, r.checked])).toEqual([
      ["name", "Siti Aisyah Binti Ahmad", false], // the profile has a name: never replaced by default
      ["phone", "+60 12-345 6789", true],
      ["location", "Petaling Jaya, Selangor", true],
      ["noticePeriod", "1 month", true],
      ["currentTitle", "Senior Accountant", false], // the current job, not the first listed
      ["currentCompany", "Nestlé Malaysia Berhad", true],
    ]);
    // email and nationality (MY) already match the profile, so they are not offered.
    expect(noticeFrom("2 week")).toEqual({ value: 2, unit: "week" });
    expect(noticeFrom("soon")).toBeNull();
  });
});

describe("reviewing a CV", () => {
  it("saves the ticked details and new skills with the version it was opened with", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ id: "doc_1", parsedProfile: profile }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate({ version: 4 })));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(testCandidate({ version: 5 })));
    const onDone = vi.fn();
    const candidate = testCandidate({ phone: null, location: null, skills: ["Excel"], version: 3 });
    renderWithClient(<CvReview documentId="doc_1" candidate={candidate} onDone={onDone} />);

    expect((await screen.findAllByText(/CV says: “SITI AISYAH BINTI AHMAD”/)).length).toBe(1);
    expect(screen.getByText("Senior Accountant — Nestlé Malaysia Berhad")).toBeTruthy();
    expect(screen.getByText(/1 value\(s\) were left out/)).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Use Name"));
    fireEvent.click(screen.getByLabelText("Use Location"));
    fireEvent.click(screen.getByRole("button", { name: "Use ticked details" }));

    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({
      version: 3,
      firstName: "Siti",
      lastName: "Aisyah Binti Ahmad",
      phone: "+60 12-345 6789",
      location: null, // unticked
      noticePeriod: { value: 1, unit: "month" },
      currentTitle: "Accountant", // already filled, so not ticked by default
      currentCompany: "Nestlé Malaysia Berhad",
      skills: ["Excel", "SAP FICO"], // Excel was already there
    });
    // The profile had no history, so the CV's is saved too, on top of the version the first save returned.
    expect(sentBody(put)).toEqual({
      version: 4,
      experience: [
        { title: "Accountant", company: "Old Co", startDate: "2016", endDate: "2019", current: false, description: null },
        { title: "Senior Accountant", company: "Nestlé Malaysia Berhad", startDate: "2020-01", endDate: null, current: true, description: null },
      ],
      education: [{ qualification: "Bachelor of Accounting", institution: "Universiti Malaya", endYear: 2015 }],
    });
  });

  it("never replaces existing work history unless asked", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ id: "doc_1", parsedProfile: profile }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(testCandidate({ version: 4 })));
    const history = [{ title: "Clerk", company: "Kept Co", startDate: null, endDate: null, current: true, description: null }];
    const onDone = vi.fn();
    renderWithClient(<CvReview documentId="doc_1" candidate={testCandidate({ experience: history })} onDone={onDone} />);
    const replace = await screen.findByLabelText<HTMLInputElement>(/Replace the profile's work history/);
    expect(replace.checked).toBe(false);
    fireEvent.click(replace);
    for (const box of screen.getAllByRole<HTMLInputElement>("checkbox")) if (box !== replace && box.checked) fireEvent.click(box);
    fireEvent.click(screen.getByRole("button", { name: "Use ticked details" }));
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ version: 3 }); // only the history was saved, at the opened version
  });
});
