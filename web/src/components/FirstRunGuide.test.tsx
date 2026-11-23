import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Permission } from "../api/client";
import { stepsFor } from "../help/firstRun";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { FirstRunGuide } from "./FirstRunGuide";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const OWNER: Permission[] = ["settings:manage", "placements:read", "identity.team:manage", "emails:send", "jobs:create", "jobs:read",
  "intake:create", "candidates:read", "aiproviders:manage", "pipeline:read"];
const SOURCER: Permission[] = ["intake:create", "candidates:read", "jobs:read", "pipeline:read"];

/** A new company: nothing set up but the AI (on the server's own provider). */
type Records = { prefs?: object; invoiceVersion?: number; team?: number; connected?: boolean; jobs?: number; candidates?: number; ai?: string };
function company(r: Records = {}) {
  const list = (n = 0) => ({ items: Array.from({ length: n }, (_, i) => ({ id: String(i) })), nextCursor: null });
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
    path === "/v1/me/preferences" ? (r.prefs ?? { candidateColumns: null, dashboardBlocks: null, guideDismissed: false })
      : path === "/v1/invoice-settings" ? { version: r.invoiceVersion ?? 0 }
        : path === "/v1/team" ? list(r.team ?? 1)
          : path === "/v1/mailbox" ? { connected: r.connected ?? false }
            : path === "/v1/jobs" ? list(r.jobs)
              : path === "/v1/candidates" ? list(r.candidates)
                : path === "/v1/ai-providers" ? { active: r.ai ?? "server", serverAvailable: true }
                  : {},
  ))) as never);
}

const steps = () => [...document.querySelectorAll(".first-run-steps li")].map((li) =>
  `${li.querySelector("strong")?.textContent ?? ""}${li.classList.contains("done") ? " ✓" : ""}`);

describe("the getting-started checklist", () => {
  it("offers each person only the steps they can do", () => {
    expect(stepsFor(OWNER).map((s) => s.id)).toEqual(["company", "team", "email", "job", "candidates", "ai"]);
    expect(stepsFor(SOURCER).map((s) => s.id)).toEqual(["candidates"]);
    expect(stepsFor(["pipeline:read"])).toEqual([]);
  });

  it("ticks the steps the records show are done, each linking to its screen", async () => {
    company({ team: 3, candidates: 1 });
    renderWithClient(<FirstRunGuide permissions={OWNER} />);
    await screen.findByRole("region", { name: "Get HiringPlug ready" });
    expect(steps()).toEqual(["Add your company's details", "Invite your team ✓", "Connect your email", "Add your first job",
      "Import candidates ✓", "Set up the AI provider ✓"]);
    expect(screen.getByText("3 of 6 done")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Connect email" }).getAttribute("href")).toBe("#/email-connection");
    expect(screen.getByRole("link", { name: "Open Invoicing" }).getAttribute("href")).toBe("#/invoicing");
    expect(screen.queryByRole("link", { name: "Open Team" }), "a done step has no button").toBeNull();
  });

  it("counts the AI as not set up when it is off, or the server has none", async () => {
    company({ ai: "off" });
    renderWithClient(<FirstRunGuide permissions={["aiproviders:manage", "emails:send"]} />);
    await screen.findByRole("region", { name: "Get HiringPlug ready" });
    expect(steps()).toEqual(["Connect your email", "Set up the AI provider"]);
  });

  it("goes away once everything is done, and never shows to an agency already set up", async () => {
    const get = company({ invoiceVersion: 2, team: 4, connected: true, jobs: 1, candidates: 1 });
    renderWithClient(<FirstRunGuide permissions={OWNER} />);
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/ai-providers"); });
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByRole("region", { name: "Get HiringPlug ready" })).toBeNull();
  });

  it("is hidden for good with Hide checklist, remembered for this person", async () => {
    const prefs = { guideDismissed: false };
    company({ prefs });
    const put = vi.spyOn(api, "PUT").mockImplementation((() => {
      prefs.guideDismissed = true;
      return Promise.resolve(ok({ ...prefs }));
    }) as never);
    renderWithClient(<FirstRunGuide permissions={SOURCER} />);
    fireEvent.click(await screen.findByRole("button", { name: "Hide checklist" }));
    await waitFor(() => { expect(screen.queryByRole("region", { name: "Get HiringPlug ready" })).toBeNull(); });
    expect(sentBody(put)).toEqual({ guideDismissed: true });
  });

  it("stays hidden for someone who hid it, without checking anything", async () => {
    const get = company({ prefs: { guideDismissed: true } });
    renderWithClient(<FirstRunGuide permissions={OWNER} />);
    await waitFor(() => { expect(get).toHaveBeenCalledWith("/v1/me/preferences"); });
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByRole("region", { name: "Get HiringPlug ready" })).toBeNull();
    expect(get).not.toHaveBeenCalledWith("/v1/team");
  });
});
