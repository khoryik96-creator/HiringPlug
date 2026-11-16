import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../../api/client";
import { ok, renderWithClient, testFieldSettings } from "../../test-utils";
import { SearchFields } from "./AdvancedSearch";
import { activeChips, EMPTY, type SearchState, toBody } from "./candidateSearch";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Harness({ onState }: { onState: (s: SearchState) => void }) {
  const [draft, setDraft] = useState(EMPTY);
  return <SearchFields draft={draft} canSeeSalary={false} setDraft={(f) => { setDraft((d) => { const n = f(d); onState(n); return n; }); }} />;
}

describe("advanced search: IT skills, specialization and certifications", () => {
  it("offers the Settings lists (then words candidates already have) and sends what is chosen", async () => {
    vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/settings/fields"
      ? ok(testFieldSettings()) : ok({ tags: [], skills: ["Excel", "java"], industries: [], specializations: ["Audit"] }))) as never);
    let state = EMPTY;
    renderWithClient(<Harness onState={(s) => { state = s; }} />);
    const pick = async (filter: RegExp, choice: string) => {
      fireEvent.click(await screen.findByRole("button", { name: filter }));
      const box = await screen.findByRole("checkbox", { name: choice });
      fireEvent.click(box);
      return box.closest("[role=group], ul, div")?.parentElement as HTMLElement;
    };
    const skills = await pick(/^IT skills/, "Python");
    expect(within(skills).getAllByRole("checkbox").map((c) => c.getAttribute("aria-label") ?? c.closest("label")?.textContent.trim()),
      "the list first, then words in use (java is Java already)").toEqual(["Java", "Python", "SAP", "Excel"]);
    await pick(/^Specialization/, "Cybersecurity");
    await pick(/^Certifications/, "CISCO");

    expect(toBody(state, "", false)).toMatchObject({ skills: ["Python"], specialization: ["Cybersecurity"], certifications: ["CISCO"] });
    expect(activeChips(state, (k) => k).map((c) => c.label)).toEqual(["IT skills: Python", "Specialization: Cybersecurity", "Certifications: CISCO"]);
  });
});
