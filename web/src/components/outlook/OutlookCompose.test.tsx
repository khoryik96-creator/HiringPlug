import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../../api/client";
import { ok, renderWithClient } from "../../test-utils";
import { unfilledFields } from "./OutlookComposeTools";
import { OutlookDraftEditor } from "./OutlookDraftEditor";
import { atStart, ownPart, withSignature } from "./outlookSignature";

beforeAll(() => {
  if (typeof HTMLDialogElement.prototype.showModal !== "function") {
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value(this: HTMLDialogElement) { this.open = true; } });
  }
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const signature = `<p><strong>Nadia Aziz</strong> · Senior Consultant</p>`;
const template = {
  id: "etp_profiles", name: "Profiles for client", subject: "Profiles for [Job title]", body: "",
  bodyHtml: "<p>Hi, please find the profile of [Full name], expecting [Expected salary].</p>", version: 3,
};
const fields: Schemas["EmailField"][] = [
  { token: "[Full name]", key: "candidate.fullName", label: "Full name", salary: false },
  { token: "[Expected salary]", key: "candidate.expectedSalary", label: "Expected salary", salary: true },
];

/** The bodies sent to one path. */
function bodiesTo(mock: { mock: { calls: unknown[][] } }, path: string): Record<string, unknown>[] {
  return mock.mock.calls.filter((c) => c[0] === path).map((c) => (c[1] as { body: Record<string, unknown> }).body);
}

function serve(sig = signature) {
  vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
    path === "/v1/email-signature" ? ok({ bodyHtml: sig, version: sig ? 1 : 0, updatedAt: null })
      : path === "/v1/email-templates" ? ok({ items: [template], fields })
        : ok({ items: [] }),
  ));
  return vi.spyOn(api, "POST").mockImplementation((path) => Promise.resolve(
    path === "/v1/email-previews" ? ok({ subject: "", bodyHtml: "", missing: [], salary: false,
      fieldValues: { "[Full name]": "Safwat Ahmad", "[Expected salary]": "RM 6,700 / month" } })
      : ok({ ...template, id: "etp_new", name: "Shortlist follow-up" }),
  ));
}

describe("HiringPlug tools in the Outlook editor", () => {
  it("adds the signature to a new message, with a line above it to write on", async () => {
    serve();
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} permissions={["candidates:read"]} />);
    const message = screen.getByRole("textbox", { name: "Message" });
    await waitFor(() => { expect(message.innerHTML).toContain("Nadia Aziz"); });
    expect(message.querySelectorAll("[data-signature]")).toHaveLength(1);
    expect(message.firstElementChild?.matches("p")).toBe(true);
  });

  it("uses a template with the candidate's details, salary included, and saves what was written as a template", async () => {
    const post = serve("");
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} about={{ candidateId: "cand_safwat" }}
      permissions={["candidates:read", "candidates.compensation:read", "emails.templates:create"]} />);
    await waitFor(() => { expect(bodiesTo(post, "/v1/email-previews")).not.toHaveLength(0); });
    expect(bodiesTo(post, "/v1/email-previews")[0]).toMatchObject({ candidateId: "cand_safwat" });
    const tools = screen.getByRole("group", { name: "HiringPlug tools" });
    fireEvent.change(await within(tools).findByLabelText("Template"), { target: { value: "etp_profiles" } });
    fireEvent.click(within(tools).getByRole("button", { name: "Use template" }));
    const message = screen.getByRole("textbox", { name: "Message" });
    await waitFor(() => { expect(message.textContent).toContain("RM 6,700 / month"); });
    expect(message.textContent).toContain("Safwat Ahmad");
    expect(screen.getByLabelText<HTMLInputElement>("Subject").value).toBe("Profiles for [Job title]");
    expect(screen.getByRole("combobox", { name: "Insert field" })).toBeTruthy();

    fireEvent.change(within(tools).getByLabelText("Template name"), { target: { value: "Shortlist follow-up" } });
    fireEvent.click(within(tools).getByRole("button", { name: "Save as new template" }));
    await waitFor(() => { expect(bodiesTo(post, "/v1/email-templates")).toHaveLength(1); });
    const saved = bodiesTo(post, "/v1/email-templates")[0];
    expect(saved?.name).toBe("Shortlist follow-up");
    expect(saved?.bodyHtml).toContain("data-field");
  });

  it("keeps a reply's subject and puts a template above the quoted message", async () => {
    serve("");
    const draft: Schemas["OutlookDraft"] = {
      messageId: "omg_reply", version: "v1", subject: "RE: Profiles", body: "", bodyHtml: "<p><br></p><hr><blockquote>Earlier message</blockquote>",
      to: ["samuel@client.test"], cc: [], bcc: [], hasAttachments: false, bodyTruncated: false, schedule: null, scheduleNeedsReview: false, webUrl: null,
    };
    vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
      path === "/v1/outlook/drafts/{messageId}" ? ok(draft) : path === "/v1/email-templates" ? ok({ items: [template], fields })
        : path === "/v1/email-signature" ? ok({ bodyHtml: signature, version: 1, updatedAt: null }) : ok({ items: [] })));
    renderWithClient(<OutlookDraftEditor id="omg_reply" focusMessage fresh onClose={vi.fn()} permissions={[]} />);
    const message = await screen.findByRole("textbox", { name: "Message" });
    await waitFor(() => { expect(message.innerHTML).toContain("Nadia Aziz"); });
    const html = message.innerHTML;
    expect(html.indexOf("Nadia Aziz")).toBeLessThan(html.indexOf("Earlier message"));
    expect(screen.getByRole<HTMLButtonElement>("button", { name: "Send now" }).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "etp_profiles" } });
    fireEvent.click(screen.getByRole("button", { name: "Use template" }));
    await waitFor(() => { expect(message.textContent).toContain("please find the profile"); });
    expect(message.innerHTML.indexOf("please find")).toBeLessThan(message.innerHTML.indexOf("Earlier message"));
    expect(screen.getByLabelText<HTMLInputElement>("Subject").value).toBe("RE: Profiles");
  });
});

describe("candidate details in an Outlook message", () => {
  it("puts the page's candidate in as a block with their salary, notice period and location", async () => {
    window.localStorage.removeItem("hiringplug.shortlist.details");
    window.localStorage.removeItem("hiringplug.shortlist.layout");
    vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
      path === "/v1/email-signature" ? ok({ bodyHtml: signature, version: 1, updatedAt: null })
        : path === "/v1/email-templates" ? ok({ items: [], fields }) : ok({ items: [] })));
    vi.spyOn(api, "POST").mockImplementation(() => Promise.resolve(ok({ subject: "", bodyHtml: "", missing: [], salary: true,
      candidateVersion: 3, compensationVersion: 2,
      fieldValues: { "[Full name]": "Safwat Ahmad", "[Current salary]": "RM 5,800 / month", "[Expected salary]": "RM 6,700 / month", "[Notice period]": "1 month" } })));
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} about={{ candidateId: "cand_safwat" }}
      permissions={["candidates:read", "candidates.compensation:read"]} />);
    const panel = screen.getByRole("group", { name: "Candidates" });
    await within(panel).findByText("Safwat Ahmad");
    await waitFor(() => { expect(screen.getByRole("textbox", { name: "Message" }).innerHTML).toContain("Nadia Aziz"); });
    const insert = within(panel).getByRole("button", { name: "Insert candidates into the message" });
    expect(insert.textContent).toBe("Insert");
    expect(within(panel).queryByRole("button", { name: /Refresh candidate details/ }), "no CVs to refresh in Outlook").toBeNull();
    fireEvent.click(insert);
    const message = screen.getByRole("textbox", { name: "Message" });
    await waitFor(() => { expect(message.textContent).toContain("Expected salary: RM 6,700 / month"); });
    expect(message.textContent).toContain("1. Safwat Ahmad");
    expect(message.textContent).toContain("Current salary: RM 5,800 / month");
    expect(message.textContent).toContain("Notice period: 1 month");
    expect(message.querySelectorAll("p > b")).toHaveLength(1);
    expect(message.innerHTML.indexOf("Safwat Ahmad"), "above the signature").toBeLessThan(message.innerHTML.indexOf("Nadia Aziz"));
  });

  it("leaves salary out for a role that may not see salaries", () => {
    window.localStorage.removeItem("hiringplug.shortlist.details");
    serve("");
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} about={{ candidateId: "cand_safwat" }} permissions={["candidates:read"]} />);
    const panel = screen.getByRole("group", { name: "Candidates" });
    expect(within(panel).queryByRole("checkbox", { name: "Expected salary" })).toBeNull();
    expect(within(panel).getByRole("checkbox", { name: "Notice period" })).toBeTruthy();
  });
});

describe("signature and template placement", () => {
  it("puts the signature once, before the quoted message", () => {
    const reply = "<p><br></p><hr><p>From: Samuel</p><blockquote>Hi</blockquote>";
    const signed = withSignature(reply, signature);
    expect(signed.indexOf("Nadia")).toBeLessThan(signed.indexOf("From: Samuel"));
    expect(withSignature(signed, signature)).toBe(signed);
    expect(withSignature("", signature)).toMatch(/^<p><br><\/p><div data-signature="true">/);
    expect(withSignature("<p>x</p>", `<div data-signature="true">${signature}</div>`).match(/data-signature/g)).toHaveLength(1);
  });

  it("takes for a template only what was written, and puts a template at the start", () => {
    const html = `<p>Dear Samuel,</p><div data-signature="true">${signature}</div><hr><blockquote>Old</blockquote>`;
    expect(ownPart(html)).toBe("<p>Dear Samuel,</p>");
    expect(atStart("<p><br></p><hr><blockquote>Old</blockquote>", "<p>New</p>")).toBe("<p>New</p><hr><blockquote>Old</blockquote>");
  });

  it("names the fields still in brackets, in the subject or the message", () => {
    const tokens = ["[Client contact]", "[Notice period]", "[Full name]"];
    expect(unfilledFields("<p>Hi [Client contact], <b>Safwat</b> has [Notice period] notice.</p>", "Update on Safwat", tokens))
      .toEqual(["[Client contact]", "[Notice period]"]);
    expect(unfilledFields("<p>All filled.</p>", "About [Full name]", tokens)).toEqual(["[Full name]"]);
  });

  it("uses a plain-text template as text, never as markup", async () => {
    serve("");
    vi.spyOn(api, "GET").mockImplementation((path) => Promise.resolve(
      path === "/v1/email-templates" ? ok({ items: [{ ...template, bodyHtml: null, body: "Hi <img src=x onerror=alert(1)> & welcome" }], fields })
        : path === "/v1/email-signature" ? ok({ bodyHtml: "", version: 0, updatedAt: null }) : ok({ items: [] })));
    renderWithClient(<OutlookDraftEditor id={null} onClose={vi.fn()} permissions={[]} />);
    await screen.findByRole("option", { name: "Profiles for client" });
    fireEvent.change(screen.getByLabelText("Template"), { target: { value: "etp_profiles" } });
    fireEvent.click(screen.getByRole("button", { name: "Use template" }));
    const message = screen.getByRole("textbox", { name: "Message" });
    await waitFor(() => { expect(message.textContent).toContain("Hi <img src=x onerror=alert(1)> & welcome"); });
    expect(message.querySelector("img")).toBeNull();
  });
});
