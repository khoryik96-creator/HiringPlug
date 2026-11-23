// The bar over the chosen candidates: why an icon is greyed out.
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Application } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { PipelineBoard } from "./PipelineBoard";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.removeItem("hp.pipelineView");
});

const person = (n: number, stage: Application["stage"]): Application => {
  const id = `01jbz6w0m0k8q9x2r3s4t5v${String(n).padStart(3, "0")}`;
  return {
    id: `appl_${id}`, candidateId: `cand_${id}`, candidateName: `Person ${String(n)}`, stage, version: 1,
    jobId: "jo_01jbz6w0m0k8q9x2r3s4t5v6w7", jobTitle: "Auditor", companyId: "comp_01jbz6w0m0k8q9x2r3s4t5v6w7", companyName: "Maybank",
    candidateTitle: "Accountant", candidateAvailability: null, reason: stage === "rejected" ? "not_qualified" : null, reasonNote: null, channel: null,
    stageChangedAt: "2026-11-20T00:00:00Z", createdAt: "2026-11-01T00:00:00Z", updatedAt: "2026-11-01T00:00:00Z",
  };
};
const greyed = (name: string) => {
  const b = screen.getByRole<HTMLButtonElement>("button", { name });
  expect(b.disabled).toBe(true);
  return b.title;
};

describe("the pipeline bar", () => {
  it("says a client link leaves out a chosen candidate who was rejected meanwhile", async () => {
    let rejected = false;
    vi.spyOn(api, "GET").mockImplementation((() => Promise.resolve(ok({ items: [person(1, "screening"), person(2, rejected ? "rejected" : "screening")] }))) as never);
    const { client } = renderWithClient(<PipelineBoard jobId="jo_01jbz6w0m0k8q9x2r3s4t5v6w7" canUpdate={false} permissions={["pipeline:read"]} onClientLink={vi.fn()} />);
    fireEvent.click(within(await screen.findByRole("tablist", { name: "Stages" })).getByRole("tab", { name: /All/ }));
    for (const row of within(screen.getByRole("list", { name: "Everyone" })).getAllByRole("listitem")) fireEvent.click(row);
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Client link for 2" }).disabled).toBe(false);
    rejected = true; // someone else rejects Person 2
    await client.invalidateQueries();
    await waitFor(() => { expect(greyed("Client link for 2")).toBe("Leave out rejected or withdrawn candidates for a client link"); });
  });

  it("says when too many are chosen for an email, a client link or formatting", async () => {
    localStorage.setItem("hp.pipelineView", "board");
    vi.spyOn(api, "GET").mockResolvedValue(ok({ items: Array.from({ length: 21 }, (_, n) => person(n + 1, "screening")) }));
    renderWithClient(<PipelineBoard jobId="jo_01jbz6w0m0k8q9x2r3s4t5v6w7" canUpdate={false}
      permissions={["pipeline:read", "emails:send", "cvformats:create"]} onClientLink={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Choose all 21 in Screening" }));
    expect(greyed("Email client about 21 candidates")).toBe("At most 10 candidates in one email to the client");
    expect(greyed("Client link for 21")).toBe("At most 20 candidates on one client link");
    expect(greyed("Format CVs for 21")).toBe("At most 20 CVs at a time");
  });
});
