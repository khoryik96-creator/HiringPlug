import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient } from "../test-utils";
import { LinkedinChecks } from "./LinkedinChecks";

let stopCompanion = () => {};
afterEach(() => {
  stopCompanion();
  cleanup();
  vi.restoreAllMocks();
});

/** A fake companion bridge: answers the page like the extension's bridge.js. */
function companion(answer: (action: string, args: Record<string, unknown>) => { result?: unknown; error?: string }) {
  const asked: { action: string; args: Record<string, unknown> }[] = [];
  // As a browser delivers the bridge's window.postMessage (jsdom leaves origin and source empty).
  const send = (m: object) => {
    setTimeout(() => {
      window.dispatchEvent(new MessageEvent("message", { data: { source: "hiringplug-companion", ...m }, origin: window.location.origin, source: window }));
    }, 0);
  };
  const listen = (e: MessageEvent) => {
    const m = e.data as { source?: string; type?: string; id?: string; action?: string; args?: Record<string, unknown> };
    if (m.source !== "hiringplug-app") return;
    if (m.type === "hello") send({ type: "ready", version: "1.0.0" });
    if (m.type === "call" && m.action && m.args) {
      asked.push({ action: m.action, args: m.args });
      send({ type: "reply", id: m.id, ...answer(m.action, m.args) });
    }
  };
  window.addEventListener("message", listen);
  stopCompanion = () => { window.removeEventListener("message", listen); };
  return asked;
}

const started = {
  companyUrl: "https://www.linkedin.com/company/maju-logistik/",
  checks: [
    { id: "clc_01jbz6w0m0k8q9x2r3s4t5v6w1", contactId: "cont_ana", contactName: "Ana Lim", profileUrl: "https://www.linkedin.com/in/ana" },
    { id: "clc_01jbz6w0m0k8q9x2r3s4t5v6w2", contactId: "cont_wei", contactName: "Wei", profileUrl: "https://www.linkedin.com/in/wei" },
  ],
  skipped: [{ contactId: "cont_nobody", contactName: "Nobody" }],
};

describe("checking employment on LinkedIn", () => {
  it("reads each contact's profile in this browser and saves what it shows", async () => {
    const asked = companion((action, args) => {
      if (action === "companyId") return { result: { company_url: args.company_url, company_id: "123456" } };
      if (args.url === "https://www.linkedin.com/in/ana") {
        return { result: { url: args.url, company_url: args.company_url, company_id: args.company_id, joined: "2019-04", left: "2025-08", current: false } };
      }
      return { error: "LinkedIn needs you to sign in, or the profile or company could not be confirmed." };
    });
    const post = vi.spyOn(api, "POST").mockImplementation(((path: string, init: { body: { error?: string } }) => {
      if (path === "/v1/contacts/linkedin-checks") return Promise.resolve(ok(started));
      const failed = init.body.error !== undefined;
      return Promise.resolve(ok({
        id: "clc_x", contactId: failed ? "cont_wei" : "cont_ana", status: failed ? "failed" : "observed", joined: failed ? null : "2019-04",
        left: failed ? null : "2025-08", current: failed ? null : false, markedLeft: !failed, error: init.body.error ?? null,
        createdAt: "2026-11-14T00:00:00Z", checkedAt: "2026-11-14T00:00:00Z",
      }));
    }) as never);
    renderWithClient(<LinkedinChecks companyId="comp_1" />);
    await new Promise((r) => setTimeout(r, 50)); // the companion says it is there
    fireEvent.click(screen.getByRole("button", { name: "Check employment on LinkedIn" }));

    expect(await screen.findByText(/Left Aug 2025 · marked as Has left/)).toBeTruthy();
    await waitFor(() => { expect(screen.getByText(/LinkedIn needs you to sign in/)).toBeTruthy(); });
    expect(screen.getByText(/No LinkedIn profile address/)).toBeTruthy();
    expect(asked.map((a) => a.action)).toEqual(["companyId", "employment", "employment"]);
    expect(asked[1]?.args).toMatchObject({ operation_id: started.checks[0]?.id, company_id: "123456", company_url: started.companyUrl });
    const answers = post.mock.calls.filter((c) => c[0] === "/v1/contacts/linkedin-checks/{checkId}/answer").map((c) => (c[1] as { body: unknown }).body);
    expect(answers).toEqual([
      { profileUrl: "https://www.linkedin.com/in/ana", companyUrl: started.companyUrl, linkedinCompanyId: "123456", joined: "2019-04", left: "2025-08", current: false },
      { error: "LinkedIn needs you to sign in, or the profile or company could not be confirmed." },
    ]);
  });

  it("says how to install the companion when it is not there", async () => {
    const post = vi.spyOn(api, "POST");
    renderWithClient(<LinkedinChecks companyId="comp_1" />);
    await new Promise((r) => setTimeout(r, 1600));
    fireEvent.click(screen.getByRole("button", { name: "Check employment on LinkedIn" }));
    expect(screen.getByText(/Install the HiringPlug LinkedIn Companion first/)).toBeTruthy();
    expect(post).not.toHaveBeenCalled();
  });
});
