import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { failure, invalidField, ok, renderWithClient, sentBody, testCandidate, testFieldSettings } from "../test-utils";
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
    change("LinkedIn profile", " linkedin.com/in/siti-aisyah ");
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    // Version 3 makes the API answer CONFLICT_STALE_VERSION instead of wiping their phone number.
    expect(sentBody(patch)).toMatchObject({ version: 3, currentTitle: "Senior Accountant", linkedinUrl: "linkedin.com/in/siti-aisyah" });
  });

  it("names the candidate who already has the email, with a link", async () => {
    vi.spyOn(api, "PATCH").mockResolvedValue(failure("CANDIDATE_DUPLICATE_EMAIL", "Another candidate already has this email.",
      { existingCandidateId: "cand_9", existingCandidateName: "Siti Aishah", existingOwnerName: "Aina" }, 409));
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Email"), { target: { value: "taken@example.com" } });
    save();
    expect((await screen.findByRole("link", { name: "Siti Aishah" })).getAttribute("href")).toBe("#/candidates/cand_9");
    expect(screen.getByText(/owned by Aina/)).toBeTruthy();
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

  it("chooses work authorization, with the pass an expat holds, in one row of choices", () => {
    renderWithClient(<CandidateForm candidate={testCandidate()} onDone={vi.fn()} />);
    const group = screen.getByRole("radiogroup", { name: "Work authorization" });
    expect(within(group).getAllByRole("radio").map((r) => r.textContent)).toEqual(["Local", "PR", "RP-T", "Spouse visa", "Other pass", "Needs pass", "Not known"]);
    fireEvent.click(within(group).getByRole("radio", { name: "Spouse visa" }));
    expect(within(group).getByRole("radio", { name: "Spouse visa" }).getAttribute("aria-checked")).toBe("true");
    expect(within(group).getByRole("radio", { name: "RP-T" }).getAttribute("aria-checked")).toBe("false");
    // A radio group: one tab stop, arrows move the choice.
    const spouse = within(group).getByRole("radio", { name: "Spouse visa" });
    expect(within(group).getAllByRole("radio").filter((r) => r.tabIndex === 0)).toEqual([spouse]);
    fireEvent.keyDown(spouse, { key: "ArrowRight" });
    expect(within(group).getByRole("radio", { name: "Other pass" }).getAttribute("aria-checked")).toBe("true");
    fireEvent.keyDown(within(group).getByRole("radio", { name: "Other pass" }), { key: "ArrowLeft" });
    expect(spouse.getAttribute("aria-checked")).toBe("true");
  });

  it("picks the specialization, IT skills and certifications from the Settings lists, and removes them", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields"
      ? ok(testFieldSettings()) : path === "/v1/me/preferences" ? ok({ favourites: {} })
        : ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] }))) as never);
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(testCandidate({ version: 4 })));
    const onDone = vi.fn();
    renderWithClient(<CandidateForm candidate={testCandidate({ skills: ["SAP", "Excel"] })} onDone={onDone} />);
    const skills = await screen.findByLabelText<HTMLButtonElement>("IT skills");
    await waitFor(() => { expect(skills.disabled, "once the lists load").toBe(false); });
    fireEvent.click(skills);
    await waitFor(() => { expect(optionsIn("All IT skills"), "the list, less those chosen").toEqual(["Java", "Python"]); });
    fireEvent.click(screen.getByRole("button", { name: "Python" }));
    fireEvent.click(screen.getByRole("button", { name: "Remove SAP" }));
    expect(screen.getByText("Excel"), "a word not in the list stays until removed").toBeTruthy();
    fireEvent.click(screen.getByLabelText("Professional certifications"));
    fireEvent.click(screen.getByRole("button", { name: "ACCA" }));
    fireEvent.click(screen.getByLabelText("Specialization"));
    expect(optionsIn("All specializations")).toEqual(["Cybersecurity", "Accounting & Finance"]);
    fireEvent.click(screen.getByRole("button", { name: "Cybersecurity" }));
    expect(screen.getByLabelText("Specialization").textContent).toContain("Cybersecurity");
    save();
    await waitFor(() => { expect(onDone).toHaveBeenCalled(); });
    expect(sentBody(patch)).toMatchObject({ skills: ["Excel", "Python"], certifications: ["ACCA"], specialization: "Cybersecurity" });
  });

  it("puts the person's favourite IT skills on top, and a star makes one a favourite", async () => {
    let saved: unknown = { favourites: { itSkill: ["Python"], country: ["SG"] } }; // as the server keeps them
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields"
      ? ok(testFieldSettings()) : path === "/v1/me/preferences" ? ok(saved)
        : ok({ tags: [], skills: [], industries: [], specializations: [], certifications: [] }))) as never);
    const put = vi.spyOn(api, "PUT").mockImplementation(((_: string, init: { body: { favourites: object } }) => {
      saved = { favourites: { ...(saved as { favourites: object }).favourites, ...init.body.favourites } };
      return Promise.resolve(ok(saved));
    }) as never);
    renderWithClient(<CandidateForm candidate={testCandidate({ skills: [] })} onDone={vi.fn()} />);
    const skills = await screen.findByLabelText<HTMLButtonElement>("IT skills");
    await waitFor(() => { expect(skills.disabled).toBe(false); });
    fireEvent.click(skills);
    await waitFor(() => { expect(optionsIn("Your favourites")).toEqual(["Python"]); });
    expect(optionsIn("All IT skills"), "every other skill is still there").toEqual(["Java", "SAP"]);
    fireEvent.click(screen.getByRole("button", { name: "Keep Java at the top (favourite)" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put), "only IT skills: the countries stay as saved").toEqual({ favourites: { itSkill: ["Python", "Java"] } });
    expect(screen.getByRole("button", { name: "Remove Java from your favourites" }), "starred at once").toBeTruthy();
    expect(optionsIn("All IT skills"), "but nothing moves under the mouse while open").toEqual(["Java", "SAP"]);
    fireEvent.click(skills);
    fireEvent.click(skills);
    await waitFor(() => { expect(optionsIn("Your favourites"), "on top the next time").toEqual(["Python", "Java"]); });
    const residence = screen.getByLabelText<HTMLSelectElement>("Country of residence");
    expect(residence.querySelector("optgroup")?.label, "a favourite country is on top").toBe("★ Your favourites");
    expect([...residence.querySelectorAll("optgroup")[0]?.querySelectorAll("option") ?? []].map((o) => o.value)).toEqual(["SG"]);
  });

  /** The items of a dropdown group, by its name. */
  const optionsIn = (group: string) => [...screen.getByRole("list", { name: group }).querySelectorAll(".fav-pick")].map((b) => b.textContent);
});
