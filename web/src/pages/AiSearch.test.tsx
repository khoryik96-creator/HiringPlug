import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, sentBody, testFieldSettings } from "../test-utils";
import { AiSearch } from "./AiSearch";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const done = {
  id: "tsr_01jbz6w0m0k8q9x2r3s4t5v6w7", status: "done", jd: "SAP BW consultant", boolean: null, strict: true, considered: 4, sent: 2,
  error: null, summary: "BPC is rare.", mustHaves: ["SAP BW"], niceToHaves: ["ABAP"], suggestedBoolean: "SAP AND BW",
  results: [
    { candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w7", name: "Aina", currentTitle: "SAP Consultant", fit: 86, verdict: "strong", reasons: ["SAP BW"], gaps: [] },
    { candidateId: "cand_01jbz6w0m0k8q9x2r3s4t5v6w8", name: "Ben", currentTitle: null, fit: 40, verdict: "weak", reasons: [], gaps: ["No BPC"] },
  ],
  createdAt: "2026-11-08T00:00:00Z", finishedAt: "2026-11-08T00:01:00Z",
};

function serve() {
  vi.spyOn(api, "GET").mockImplementation(((path: string) => {
    if (path === "/v1/talent-searches") return Promise.resolve(ok({ items: [] }));
    if (path === "/v1/talent-searches/{searchId}") return Promise.resolve(ok(done));
    if (path === "/v1/settings/fields") return Promise.resolve(ok(testFieldSettings()));
    return Promise.resolve(ok({ tags: [], skills: ["SAP BW"], industries: [], specializations: [] }));
  }) as never);
}

describe("AI candidate search", () => {
  it("asks for a JD or a boolean, sends them with the filters, and shows the shortlist best first", async () => {
    serve();
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...done, status: "queued", results: [] }));
    renderWithClient(<AiSearch canSeeSalary canReadJd={false} />);
    const find = screen.getByRole("button", { name: "Find candidates" });
    expect((find as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByPlaceholderText(/Responsibilities, requirements/), { target: { value: "SAP BW consultant" } });
    fireEvent.click(screen.getByText(/^Filters/));
    fireEvent.click(screen.getByRole("button", { name: /^Lives in/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: "Malaysia (MY)" }));
    fireEvent.click(screen.getByLabelText("Strict must-haves"));
    fireEvent.click(find);
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ jd: "SAP BW consultant", strict: true, rank: 15, search: { limit: 25, skillsMatch: "all", residence: ["MY"] } });

    const aina = await screen.findByRole("link", { name: "Aina" });
    expect(aina.getAttribute("href")).toBe("#/candidates/cand_01jbz6w0m0k8q9x2r3s4t5v6w7?hl=" + encodeURIComponent("SAP AND BW"));
    expect(screen.getByText("BPC is rare.")).toBeTruthy();
    expect(screen.getByText("No BPC")).toBeTruthy();
    expect(screen.getAllByRole("listitem").filter((li) => li.classList.contains("fit-card")).map((li) => li.textContent.slice(0, 2))).toEqual(["86", "40"]);

    fireEvent.click(screen.getByRole("button", { name: "Use it" }));
    expect(screen.getByPlaceholderText<HTMLInputElement>(/SAP AND \(BW OR BPC\)/).value).toBe("SAP AND BW");
  });
});
