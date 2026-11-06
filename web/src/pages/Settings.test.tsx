import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { Settings } from "./Settings";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("settings", () => {
  it("adds, reorders and hides options, then saves the list in order", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { options: { key?: string; label: string; hidden: boolean }[] } }) =>
      Promise.resolve(ok({ name: "candidate_source", label: "Candidate source", kind: "open",
        options: init.body.options.map((o) => ({ key: o.key ?? "job_fair", label: o.label, hidden: o.hidden, builtIn: !!o.key })) }))) as never);
    renderWithClient(<Settings />);
    const sources = await screen.findByRole("region", { name: "Candidate source" });
    expect(within(sources).queryByRole("button", { name: "Remove" })).toBeNull(); // built-in ones are hidden, not removed

    fireEvent.change(within(sources).getByLabelText("New candidate source option"), { target: { value: "Job fair" } });
    fireEvent.click(within(sources).getByRole("button", { name: "Add" }));
    fireEvent.click(within(sources).getByRole("button", { name: "Move Job fair up" }));
    const [, linkedIn] = within(sources).getAllByRole("checkbox");
    if (linkedIn) fireEvent.click(linkedIn); // hide LinkedIn
    fireEvent.click(within(sources).getByRole("button", { name: "Save candidate source" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    const body = sentBody(put) as { options: { key?: string; label: string; hidden: boolean }[] };
    expect(body.options.map((o) => o.label)).toEqual(["JobStreet", "LinkedIn", "Referral", "Careers page", "Direct", "Job fair", "Other"]);
    expect(body.options[1]).toEqual({ key: "linkedin", label: "LinkedIn", hidden: true });
    expect(body.options[5]).toEqual({ label: "Job fair", hidden: false });
  });

  it("adds a notice period as a number and unit", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(invalidField("options[4].key", "a number of days, weeks or months"));
    renderWithClient(<Settings />);
    const notice = await screen.findByRole("region", { name: "Notice period" });
    fireEvent.change(within(notice).getByLabelText("Number"), { target: { value: "6" } });
    fireEvent.click(within(notice).getByRole("button", { name: "Add" }));
    expect(within(notice).getByDisplayValue("6 months")).toBeTruthy();
    fireEvent.click(within(notice).getByRole("button", { name: "Save notice period" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect((sentBody(put) as { options: { key: string }[] }).options.at(-1)?.key).toBe("6_month");
    expect(await within(notice).findByText("a number of days, weeks or months")).toBeTruthy();
  });

  it("chooses mandatory fields; the app's own stay on", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ ...(testFieldSettings() as object), mandatory: [
      { form: "candidate", field: "firstName", label: "First name", mandatory: true, alwaysRequired: true },
      { form: "candidate", field: "email", label: "Email", mandatory: false, alwaysRequired: false },
      { form: "job", field: "location", label: "Location", mandatory: false, alwaysRequired: false },
    ] }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(testFieldSettings()));
    renderWithClient(<Settings />);
    const panel = await screen.findByRole("region", { name: "Mandatory fields" });
    expect(within(panel).getByRole<HTMLInputElement>("checkbox", { name: /First name/ }).disabled).toBe(true);
    fireEvent.click(within(panel).getByRole("checkbox", { name: "Email" }));
    fireEvent.click(within(panel).getByRole("button", { name: "Save mandatory fields" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ fields: [
      { form: "candidate", field: "firstName", mandatory: true },
      { form: "candidate", field: "email", mandatory: true },
      { form: "job", field: "location", mandatory: false },
    ] });
  });

  it("chooses who sees emails that show salary", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) =>
      Promise.resolve(ok(path === "/v1/settings/policies" ? { emailSalaryVisibility: "salary_roles" } : testFieldSettings()))) as never);
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ emailSalaryVisibility: "everyone" }));
    renderWithClient(<Settings />);
    const choice = await screen.findByLabelText<HTMLSelectElement>("Who sees emails that show salary");
    expect(choice.value).toBe("salary_roles");
    fireEvent.change(choice, { target: { value: "everyone" } });
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(put.mock.calls[0]?.[0]).toBe("/v1/settings/policies");
    expect(sentBody(put)).toEqual({ emailSalaryVisibility: "everyone" });
    expect(await screen.findByText(/Saved: emails sent from now on follow it/)).toBeTruthy();
  });
});
