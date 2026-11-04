import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Permission, type Schemas } from "../api/client";
import { ok, renderWithClient, sentBody } from "../test-utils";
import { exportText } from "./dataExport";
import { consentText, PrivacyPanel, todayISO } from "./PrivacyPanel";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const consent: Schemas["Consent"] = { version: 2, status: "withdrawn", method: "email", day: "2026-11-03", note: null, updatedAt: "2026-11-03T02:00:00Z" };
const request = (over: Partial<Schemas["PrivacyRequest"]>): Schemas["PrivacyRequest"] => ({
  id: "prq_1", version: 1, candidateId: "cand_1", candidateName: "Siti", kind: "access", receivedOn: "2026-10-01", details: "By email",
  status: "open", outcome: null, dueOn: "2026-10-22", overdue: true, daysOpen: 34, closedAt: null, closedByName: null, createdAt: "2026-10-01T00:00:00Z", ...over,
});
const RECORD: Permission[] = ["privacy:read", "privacy:record"];

describe("data protection panel", () => {
  it("is not shown, or fetched, without the permission", () => {
    const get = vi.spyOn(api, "GET");
    renderWithClient(<PrivacyPanel candidateId="cand_1" candidateName="Siti" permissions={["candidates:read"]} />);
    expect(screen.queryByRole("region", { name: "Data protection" })).toBeNull();
    expect(get).not.toHaveBeenCalled();
  });

  it("says when a candidate is not to be contacted, and lists their requests with when they are due", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ consent, doNotContact: true, requests: [request({})] }));
    renderWithClient(<PrivacyPanel candidateId="cand_1" candidateName="Siti" permissions={["privacy:read"]} />);
    expect((await screen.findByText(/Do not contact/)).textContent).toContain("Withdrawn on 3 Nov 2026 (Email)");
    expect(screen.getByText("A copy of their data").closest("li")?.textContent).toContain("overdue since 22 Oct 2026");
    expect(screen.queryByRole("button", { name: "Record consent" })).toBeNull();
  });

  it("records consent with the version it showed", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ consent, doNotContact: true, requests: [] }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({ ...consent, status: "given", version: 3 }));
    renderWithClient(<PrivacyPanel candidateId="cand_1" candidateName="Siti" permissions={RECORD} />);
    fireEvent.click(await screen.findByRole("button", { name: "Record consent" }));
    expect(screen.getByLabelText<HTMLSelectElement>("Consent").value).toBe("given");
    fireEvent.change(screen.getByLabelText("How"), { target: { value: "phone" } });
    fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "  Agreed on a call " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ expectedVersion: 2, status: "given", method: "phone", day: todayISO(), note: "Agreed on a call" });
  });

  it("logs a request", async () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok({ consent: null, doNotContact: false, requests: [] }));
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok(request({})));
    renderWithClient(<PrivacyPanel candidateId="cand_1" candidateName="Siti" permissions={RECORD} />);
    expect(await screen.findByText(/No consent recorded/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Log a request" }));
    fireEvent.change(screen.getByLabelText("Request"), { target: { value: "removal" } });
    fireEvent.click(screen.getByRole("button", { name: "Log request" }));
    await waitFor(() => { expect(post).toHaveBeenCalled(); });
    expect(sentBody(post)).toEqual({ kind: "removal", receivedOn: todayISO(), details: null });
  });
});

describe("a copy of a candidate's data", () => {
  it("reads as plain text, with what was left out", () => {
    const text = exportText({
      candidateId: "cand_1", candidateName: "Siti Aisyah", company: "Maju Recruitment", exportedAt: "2026-11-04T03:00:00Z",
      sections: [
        { title: "Profile", items: [{ fields: [{ label: "Name", value: "Siti Aisyah" }, { label: "Email", value: "siti@example.com" }] }] },
        { title: "Notes", items: [] },
        { title: "CVs and documents", items: [{ fields: [{ label: "Text read from it", value: "Line one\nLine two" }] }] },
      ],
      notIncluded: ["Salary (current and expected): you may not see it."],
    });
    expect(text).toBe([
      "Personal data held about Siti Aisyah by Maju Recruitment", "As of 2026-11-04", "",
      "PROFILE", "-------", "Name: Siti Aisyah", "Email: siti@example.com", "",
      "NOTES", "-----", "(none)", "",
      "CVS AND DOCUMENTS", "-----------------", "Text read from it: Line one", "    Line two", "",
      "NOT INCLUDED IN THIS COPY", "- Salary (current and expected): you may not see it.", "",
    ].join("\n"));
    expect(consentText(null)).toBe("No consent recorded");
    expect(todayISO(new Date(2026, 0, 5))).toBe("2026-01-05");
  });
});
