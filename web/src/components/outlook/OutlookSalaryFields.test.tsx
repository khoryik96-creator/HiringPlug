// Salary fields in an Outlook message: their values are asked for once one
// is in the message or the subject (counted as shown), go in where they are
// (the subject too), and sending waits for them: a message never goes out
// with "[Expected salary]" in it.
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { ok, renderWithClient } from "../../test-utils";
import { OutlookDraftEditor } from "./OutlookDraftEditor";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const fields: Schemas["EmailField"][] = [
  { token: "[Full name]", key: "candidate.fullName", label: "Full name", salary: false },
  { token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true },
];
const draft = { messageId: "omg_new", version: "v1", subject: "", body: "", to: [], cc: [], bcc: [], hasAttachments: false,
  bodyTruncated: false, schedule: null, scheduleNeedsReview: false, webUrl: null };

/**
 * The API: the salary's value only when asked for (otherwise named as withheld), that answer held back
 * until `answer` is called; a new draft from what was written.
 */
function serve() {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/email-signature" ? ok({ bodyHtml: "", version: 0, updatedAt: null })
      : path === "/v1/email-templates" ? ok({ items: [], fields }) : ok({ items: [] })));
  let answer!: () => void;
  const salary = new Promise<void>((resolve) => { answer = resolve; });
  const post = vi.spyOn(api, "POST").mockImplementation(async (path, init?: { body?: { includeSalary?: boolean } }) => {
    if (path === "/v1/outlook/drafts") return ok(draft);
    if (init?.body?.includeSalary) await salary;
    return ok({ subject: "", bodyHtml: "", missing: [], salary: false,
      fieldValues: { "[Full name]": "Safwat Ahmad", ...(init?.body?.includeSalary ? { "[Expected salary]": "RM 6,700 / month" } : {}) },
      withheldFields: init?.body?.includeSalary ? [] : ["[Expected salary]"] });
  });
  const put = vi.spyOn(api, "PUT").mockResolvedValue(ok({}));
  return { answer, post, put };
}

function write(subject: string, html: string) {
  fireEvent.change(screen.getByLabelText("To"), { target: { value: "client@example.test" } });
  fireEvent.change(screen.getByLabelText("Subject"), { target: { value: subject } });
  const message = screen.getByRole("textbox", { name: "Message" });
  message.innerHTML = html;
  fireEvent.input(message);
  return message;
}

/** The body of the new draft made from what was written. */
const draftMade = (post: { mock: { calls: unknown[][] } }) =>
  (post.mock.calls.find((c) => c[0] === "/v1/outlook/drafts")?.[1] as { body: { subject: string; bodyHtml: string } }).body;

const salaryAsked = (post: { mock: { calls: unknown[][] } }) =>
  post.mock.calls.filter((c) => c[0] === "/v1/email-previews" && (c[1] as { body: { includeSalary?: boolean } }).body.includeSalary === true).length;

describe("salary fields in an Outlook message", () => {
  it("waits to send until the salary is in, never sending its [Expected salary]", async () => {
    const api = serve();
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} about={{ candidateId: "cand_safwat" }}
      permissions={["candidates:read", "candidates.compensation:read"]} />);
    await waitFor(() => { expect(api.post).toHaveBeenCalled(); });
    const message = write("Profile", "<p>Expecting [Expected salary].</p>");
    const send = screen.getByRole<HTMLButtonElement>("button", { name: "Send now" });
    await waitFor(() => { expect(salaryAsked(api.post)).toBe(1); });
    expect(send.disabled).toBe(true);
    expect(screen.getByText(/Putting in \[Expected salary\]…/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Send later" }));
    fireEvent.change(screen.getByLabelText(/Send at/), { target: { value: "2099-01-01T09:00" } });
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Schedule email" }).disabled).toBe(true);

    api.answer();
    await waitFor(() => { expect(message.textContent).toContain("RM 6,700 / month"); });
    await waitFor(() => { expect(send.disabled).toBe(false); });
    fireEvent.click(send);
    await waitFor(() => { expect(api.put).toHaveBeenCalled(); });
    expect(draftMade(api.post).bodyHtml).toContain("RM 6,700 / month");
    expect(draftMade(api.post).bodyHtml).not.toContain("[Expected salary]");
  });

  it("puts a salary field in the subject in as its value, as the message's", async () => {
    const api = serve();
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} about={{ candidateId: "cand_safwat" }}
      permissions={["candidates:read", "candidates.compensation:read"]} />);
    await waitFor(() => { expect(api.post).toHaveBeenCalled(); });
    write("[Full name], expecting [Expected salary]", "<p>Please see the profile.</p>");
    await waitFor(() => { expect(salaryAsked(api.post)).toBe(1); }); // the subject's salary field asked for it (counted)
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Send now" }).disabled).toBe(true);
    api.answer();
    const subject = screen.getByLabelText<HTMLInputElement>("Subject");
    await waitFor(() => { expect(subject.value).toBe("Safwat Ahmad, expecting RM 6,700 / month"); });
    fireEvent.click(screen.getByRole("button", { name: "Save draft" }));
    await waitFor(() => { expect(api.post.mock.calls.some((c) => c[0] === "/v1/outlook/drafts")).toBe(true); });
    expect(draftMade(api.post).subject).toBe("Safwat Ahmad, expecting RM 6,700 / month");
  });
});
