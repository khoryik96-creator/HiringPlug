import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { focusManager } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { ok, RECRUITER, renderWithClient } from "../test-utils";
import { CandidateEmails } from "./CandidateEmails";
import { ContactEmails } from "./ContactEmails";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  focusManager.setFocused(undefined);
});

const email = (status: Schemas["EmailStatus"], texts = true): Schemas["Email"] => ({
  id: "eml_1", candidateId: "cand_1", contactId: null, applicationId: null, to: ["hr@client.test"], cc: [], bcc: [], attachments: [],
  subject: texts ? "Siti for Finance Manager" : "", body: texts ? "Expecting RM 12,000 per month" : "", bodyHtml: null, salary: true,
  status, error: null, sentBy: "usr_1", sentByName: "Aina", createdAt: "2026-11-24T01:00:00Z", sentAt: status === "sent" ? "2026-11-24T01:00:05Z" : null,
  bouncedTo: [], bouncedAt: null,
});

/** The API: the full list (counted as read), or only how each is getting on (statusOnly: no texts). */
function serve() {
  let delivered = false;
  const get = vi.spyOn(api, "GET").mockImplementation(((path: string, init?: { params?: { query?: { statusOnly?: boolean } } }) => {
    if (path !== "/v1/emails") return Promise.resolve(ok({ items: [] }));
    const statusOnly = init?.params?.query?.statusOnly === true;
    return Promise.resolve(ok({ items: [email(delivered ? "sent" : "queued", !statusOnly)] }));
  }) as never);
  const reads = (statusOnly: boolean) => get.mock.calls
    .filter((c) => c[0] === "/v1/emails" && ((c[1] as { params: { query: { statusOnly?: boolean } } }).params.query.statusOnly === true) === statusOnly).length;
  return { deliver: () => { delivered = true; }, reads };
}

describe("sent emails while one is on its way", () => {
  for (const [who, page] of [
    ["a candidate's", () => <CandidateEmails candidateId="cand_1" email="siti@example.com" permissions={RECRUITER} />],
    ["a contact's", () => <ContactEmails contactId="cont_1" email="hr@client.test" permissions={RECRUITER} />],
  ] as const) {
    it(`asks ${who} emails only how they are getting on, and never reads them again on focus`, async () => {
      const api = serve();
      vi.useFakeTimers({ shouldAdvanceTime: true });
      renderWithClient(page());
      const list = await screen.findByRole("region", { name: "Emails" });
      expect(await within(list).findByText("Sending")).toBeTruthy();
      expect(within(list).getByText("Expecting RM 12,000 per month")).toBeTruthy();
      api.deliver();
      await vi.advanceTimersByTimeAsync(3_100);
      await waitFor(() => { expect(within(list).getByText("Sent")).toBeTruthy(); });
      expect(within(list).getByText("Siti for Finance Manager")).toBeTruthy(); // the texts kept from the list
      expect(api.reads(true)).toBeGreaterThan(0);
      await vi.advanceTimersByTimeAsync(10_000);
      // Back to the window: an email that shows salary is not read again (each read is counted).
      focusManager.setFocused(false);
      focusManager.setFocused(true);
      await vi.advanceTimersByTimeAsync(100);
      expect(api.reads(false)).toBe(1);
    });
  }
});
