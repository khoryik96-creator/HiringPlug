import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Compensation } from "../api/client";
import { invalidField, ok, renderWithClient, sentBody, testCandidate } from "../test-utils";
import { SalaryForm } from "./SalaryForm";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const recorded: Compensation = {
  version: 1, updatedAt: "2026-11-01T00:00:00Z",
  currentSalary: { amount: "8500.00", currency: "MYR", period: "month" }, expectedSalary: null,
};

/** An input inside the "Current salary" or "Expected salary" group. */
const input = (group: string, label: string) => within(screen.getByRole("group", { name: group })).getByLabelText(label);
const change = (group: string, label: string, value: string) => {
  fireEvent.change(input(group, label), { target: { value } });
};
const save = () => {
  fireEvent.click(screen.getByRole("button", { name: "Save salary" }));
};
/** The message shown under an input. */
const messageUnder = (group: string, label: string) => input(group, label).closest(".field")?.querySelector(".error")?.textContent;

describe("salary form", () => {
  it("saves with the version it was opened with, so a newer save by someone else is a conflict", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...recorded, version: 3 }));
    const onDone = vi.fn();
    const { rerender } = renderWithClient(<SalaryForm candidate={testCandidate({ compensation: recorded })} onDone={onDone} />);
    // Someone else changed the salary; a background refresh brings version 2.
    const theirs = { ...recorded, version: 2, currentSalary: { amount: "9200.00", currency: "MYR", period: "month" as const } };
    rerender(<SalaryForm candidate={testCandidate({ compensation: theirs })} onDone={onDone} />);
    change("Expected salary", "From (or the single figure)", "10000");
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ expectedVersion: 1 });
  });

  it("does not send the old breakdown with a new figure, so emails never show a stale package", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(recorded));
    const described: Compensation = { ...recorded, currentSalary: { amount: "8500.00", currency: "MYR", period: "month", description: "8k + 500 allowance" } };
    const onDone = vi.fn();
    renderWithClient(<SalaryForm candidate={testCandidate({ compensation: described })} onDone={onDone} />);
    change("Current salary", "Amount", "9500");
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ currentSalary: { amount: "9500" } });
    expect((sentBody(put) as { currentSalary: Record<string, unknown> }).currentSalary).not.toHaveProperty("description");
  });

  it("closes without saving when Add salary is saved empty", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(recorded));
    const onDone = vi.fn();
    renderWithClient(<SalaryForm candidate={testCandidate()} onDone={onDone} />);
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(put).not.toHaveBeenCalled();
  });

  it("refuses RM as a currency, with the message next to the currency", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(recorded));
    renderWithClient(<SalaryForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Current salary", "Amount", "8500");
    change("Current salary", "Currency", "RM");
    save();
    await waitFor(() => { expect(messageUnder("Current salary", "Currency")).toMatch(/3-letter currency code/); });
    expect(put).not.toHaveBeenCalled();
  });

  it("shows the API's message about a currency next to that currency", async () => {
    vi.spyOn(api, "PUT").mockResolvedValue(invalidField("expectedSalary.currency", "is not a currency we know"));
    renderWithClient(<SalaryForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Expected salary", "From (or the single figure)", "10000");
    change("Expected salary", "Currency", "XYZ");
    save();
    await waitFor(() => { expect(messageUnder("Expected salary", "Currency")).toBe("is not a currency we know"); });
  });

  it("asks for From when only To is filled, instead of dropping the figure", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(recorded));
    renderWithClient(<SalaryForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Expected salary", "To (optional)", "12000");
    save();
    await waitFor(() => { expect(messageUnder("Expected salary", "From (or the single figure)")).toMatch(/lower figure/); });
    expect(put).not.toHaveBeenCalled();
  });

  it("refuses a decimal comma instead of reading 8500,50 as 850050", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(recorded));
    renderWithClient(<SalaryForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Current salary", "Amount", "8500,50");
    save();
    await waitFor(() => { expect(messageUnder("Current salary", "Amount")).toMatch(/a point before cents/); });
    expect(put).not.toHaveBeenCalled();
  });

  it("accepts thousands separators", async () => {
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(recorded));
    renderWithClient(<SalaryForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Current salary", "Amount", "8,500");
    save();
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toMatchObject({ currentSalary: { amount: "8500", currency: "MYR", period: "month" } });
  });
});
