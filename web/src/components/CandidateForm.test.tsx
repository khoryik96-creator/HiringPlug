import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { failure, invalidField, ok, renderWithClient, testCandidate } from "../test-utils";
import { CandidateForm } from "./CandidateForm";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const change = (label: string, value: string) => {
  fireEvent.change(screen.getByLabelText(label), { target: { value } });
};
const save = () => {
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
};

describe("candidate form", () => {
  it("saves with the version it was opened with, so a newer save by someone else is a conflict", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate({ version: 4 })));
    const onDone = vi.fn();
    const { rerender } = renderWithClient(<CandidateForm candidate={testCandidate({ version: 3 })} onDone={onDone} />);
    // Meanwhile someone else saved a phone number; a background refresh brings version 4.
    rerender(<CandidateForm candidate={testCandidate({ version: 4, phone: "+60123456789" })} onDone={onDone} />);
    change("Current title", "Senior Accountant");
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    // Version 3 makes the API answer CONFLICT_STALE_VERSION instead of wiping their phone number.
    expect(patch.mock.calls[0]?.[1].body).toMatchObject({ version: 3, currentTitle: "Senior Accountant" });
  });

  it("closes without saving when nothing changed", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate()));
    const onDone = vi.fn();
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={onDone} />);
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(patch).not.toHaveBeenCalled();
  });

  it("shows the API's message about the note under the note", async () => {
    vi.spyOn(api, "PATCH").mockResolvedValue(invalidField("workAuthorization.note", "maximum string length is 500"));
    renderWithClient(<CandidateForm candidate={testCandidate({ workAuthorization: { status: "local" } })} onDone={vi.fn()} />);
    change("Work authorization note", "x".repeat(600));
    save();
    const message = await screen.findByText("maximum string length is 500");
    expect(message.closest(".field")?.querySelector("label")?.textContent).toBe("Work authorization note");
    // Linked to the input, so a screen reader reads it with the field.
    const note = screen.getByLabelText("Work authorization note");
    expect(note.getAttribute("aria-invalid")).toBe("true");
    expect(note.getAttribute("aria-describedby")).toBe(message.id);
  });

  it("lists messages that no field shows, so a failed save is never silent", async () => {
    vi.spyOn(api, "PATCH").mockResolvedValue(invalidField("body", "The request body could not be read."));
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Current title", "Senior Accountant");
    save();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Some fields are invalid.");
    expect(alert.textContent).toContain("body: The request body could not be read.");
  });

  it("says when someone else saved first, with the reference, and offers their version", async () => {
    const message = "This candidate was changed by someone else. Reload it and try again.";
    vi.spyOn(api, "PATCH").mockResolvedValue(failure("CONFLICT_STALE_VERSION", message, undefined, 409));
    const onDone = vi.fn();
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={onDone} />);
    change("Current title", "Senior Accountant");
    save();
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain(message);
    expect(alert.textContent).toContain("req_test");
    fireEvent.click(within(alert).getByRole("button", { name: /Reload their version/ }));
    expect(onDone).toHaveBeenCalled();
  });

  it("accepts every email the API accepts, e.g. josé@example.com", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate()));
    renderWithClient(<CandidateForm candidate={testCandidate({ email: "josé@example.com" })} onDone={vi.fn()} />);
    change("Current title", "Senior Accountant");
    save();
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
  });

  it("refuses a country name rather than cutting it into another country's code", async () => {
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate()));
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={vi.fn()} />);
    change("Nationality (2-letter code)", "China");
    save();
    expect(await screen.findByText(/Use the 2-letter country code/)).toBeTruthy();
    expect(patch).not.toHaveBeenCalled();
  });

  it("asks which pass an expat holds only when it applies", () => {
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={vi.fn()} />);
    expect(screen.queryByText("Pass held")).toBeNull();
    change("Work authorization", "expat_eligible");
    expect(screen.getByText("Pass held")).toBeTruthy();
  });
});
