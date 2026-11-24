import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import * as companion from "../companion";
import { failure, ok, sentBody } from "../test-utils";
import { ALL, company, lead, serve, show } from "./leadFindHRFixtures";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("a lead's Find HR panel", () => {
  it("lists the companies with the same or a similar name, and links the lead to the one chosen", async () => {
    const get = serve();
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, version: 5, companyId: "comp_1" }));
    show();
    expect(screen.getByRole("heading", { name: "Find HR · Contoh Logistik Sdn Bhd" })).toBeTruthy();
    const same = (await screen.findByText("Contoh Logistics Berhad")).closest("li") as HTMLElement;
    expect(same.textContent).toContain("in HiringPlug · similar name");
    expect(screen.getByText("in HiringPlug · same name · Shah Alam")).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/companies/named-like", { params: { query: { name: "Contoh Logistik Sdn Bhd" } } });
    // A company with the same name: adding a new one waits behind a link.
    expect(screen.queryByRole("form", { name: "New company" })).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "This one: Contoh Logistik Sdn Bhd" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(patch.mock.calls[0]?.[0]).toBe("/v1/job-leads/{jobLeadId}");
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_1" });
    // Step 2: the company's Find HR, in the panel.
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Find HR people" })).toBeTruthy();
  });

  it("adds the company with its LinkedIn page, then links the lead", async () => {
    serve({ matches: [] });
    const post = vi.spyOn(api, "POST").mockResolvedValue(ok({ ...company, id: "comp_9", name: "Contoh Logistik" }));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, companyId: "comp_9" }));
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    expect(screen.getByText("No company in HiringPlug has this name or one like it.")).toBeTruthy();
    fireEvent.change(within(form).getByLabelText("Name"), { target: { value: "Contoh Logistik" } });
    expect(within(form).getByRole("heading", { name: "Add Contoh Logistik as a new company" })).toBeTruthy();
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/contoh-logistik" } });
    fireEvent.click(within(form).getByRole("button", { name: "Add and link" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(post.mock.calls[0]?.[0]).toBe("/v1/companies");
    expect(sentBody(post)).toEqual({ name: "Contoh Logistik", linkedinUrl: "https://www.linkedin.com/company/contoh-logistik/" });
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_9" });
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
  });

  it("offers to link the company that has the name already", async () => {
    serve({ matches: [] });
    vi.spyOn(api, "POST").mockResolvedValue(failure("COMPANY_DUPLICATE_NAME", "A company with this name exists.", { existingCompanyId: "comp_1" }, 409));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok({ ...lead, companyId: "comp_1" }));
    show();
    fireEvent.click(await screen.findByRole("button", { name: "Add and link" }));
    fireEvent.click(await screen.findByRole("button", { name: "Link the lead to that one" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 4, companyId: "comp_1" });
  });

  it("keeps 'Link the lead to that one' beside its error when that link fails, to try again", async () => {
    serve({ matches: [] });
    vi.spyOn(api, "POST").mockResolvedValue(failure("COMPANY_DUPLICATE_NAME", "A company with this name exists.", { existingCompanyId: "comp_1" }, 409));
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(failure("CONFLICT_STALE_VERSION", "Changed by someone else.", {}, 409));
    show();
    fireEvent.click(await screen.findByRole("button", { name: "Add and link" }));
    fireEvent.click(await screen.findByRole("button", { name: "Link the lead to that one" }));
    expect(await screen.findByText("Changed by someone else.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Link the lead to that one" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
  });

  it("finds the company's LinkedIn page from its name with the companion", async () => {
    serve({ matches: [] });
    const call = vi.spyOn(companion, "companionCall").mockResolvedValue({ companies: [
      { url: "https://www.linkedin.com/company/contoh-logistik/", name: "Contoh Logistik", subtitle: "Truck Transportation • Shah Alam, Selangor" },
      { url: "https://www.linkedin.com/company/contoh-group/", name: "Contoh Group", subtitle: "" },
    ] });
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    fireEvent.click(within(form).getByRole("button", { name: "Find on LinkedIn" }));
    const found = await screen.findByRole("list", { name: "Found on LinkedIn" });
    expect(call.mock.calls[0]?.[0]).toBe("companySearch");
    expect(call.mock.calls[0]?.[1]).toMatchObject({ name: "Contoh Logistik Sdn Bhd" });
    expect(found.textContent).toContain("Truck Transportation • Shah Alam, Selangor");
    fireEvent.click(within(found).getByRole("button", { name: /Contoh Logistik/ }));
    expect(within(form).getByLabelText<HTMLInputElement>("LinkedIn page").value).toBe("https://www.linkedin.com/company/contoh-logistik/");
    expect(within(found).getByRole("button", { name: /Contoh Logistik/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("asks for the page to be pasted when the companion is too old to find it", async () => {
    serve({ matches: [], version: "1.5.0" });
    show();
    const form = await screen.findByRole("form", { name: "New company" });
    expect(within(form).queryByRole("button", { name: "Find on LinkedIn" })).toBeNull();
    expect(within(form).getByText(/update the LinkedIn companion to 1.6.0/)).toBeTruthy();
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "https://example.com/contoh" } });
    expect(within(form).getByText(/not a LinkedIn company page/)).toBeTruthy();
    expect(within(form).getByRole<HTMLButtonElement>("button", { name: "Add and link" }).disabled).toBe(true);
  });

  it("shows a linked lead's Find HR at once", async () => {
    const get = serve();
    show({ companyId: "comp_1" });
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
    // The company opens as a tab here, never a browser tab, and the panel stays.
    const name = screen.getByRole("link", { name: "Contoh Logistik Sdn Bhd" });
    expect(name.getAttribute("target")).toBeNull();
    const click = new MouseEvent("click", { bubbles: true, cancelable: true });
    name.dispatchEvent(click);
    expect(click.defaultPrevented).toBe(true);
    expect(screen.getByRole("region", { name: "Find HR" })).toBeTruthy();
    expect(get.mock.calls.some((c) => c[0] === "/v1/companies/named-like")).toBe(false);
  });

  it("asks a linked company without a LinkedIn page for it first, and saves it", async () => {
    serve({ saved: { ...company, linkedinUrl: null } });
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(company));
    show({ companyId: "comp_1" });
    const form = await screen.findByRole("form", { name: "Its LinkedIn page" });
    expect(screen.queryByRole("region", { name: "Find HR" })).toBeNull();
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "https://www.linkedin.com/company/contoh-logistik/about/" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save page" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(patch.mock.calls[0]?.[0]).toBe("/v1/companies/{companyId}");
    expect(sentBody(patch)).toEqual({ version: 2, linkedinUrl: "https://www.linkedin.com/company/contoh-logistik/" });
    expect(await screen.findByRole("region", { name: "Find HR" })).toBeTruthy();
  });

  it("says what is missing instead of offering what the user may not do", async () => {
    serve({ matches: [] });
    show({}, ["leads:read", "companies:read", "contacts:read"]);
    expect(await screen.findByText(/needs the permission to manage leads/)).toBeTruthy();
    expect(screen.queryByRole("form", { name: "New company" })).toBeNull();
    cleanup();
    serve();
    show({}, ["leads:read", "leads:manage", "companies:read", "contacts:read"]);
    fireEvent.click(await screen.findByRole("button", { name: /None of these/ }));
    expect(screen.getByText(/needs the permission to create companies/)).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /^This one/ })).toHaveLength(2);
    cleanup();
    serve({ saved: { ...company, linkedinUrl: null } });
    show({ companyId: "comp_1" }, ["leads:read", "companies:read", "contacts:read"]);
    expect(await screen.findByText(/adding it needs the permission to edit companies/)).toBeTruthy();
  });

  it("closes with Escape in the panel or its close button, but not on Escape elsewhere, while typing, or in a dialog", async () => {
    serve({ matches: [] });
    const onClose = vi.fn();
    show({}, ALL, onClose);
    const panel = screen.getByRole("complementary", { name: "Find HR for Contoh Logistik Sdn Bhd" });
    const form = await within(panel).findByRole("form", { name: "New company" });
    // Elsewhere on the page (the open records bar, the list's search box): not the panel's.
    fireEvent.keyDown(document.body, { key: "Escape" });
    // Typing in the panel: what was typed stays.
    fireEvent.keyDown(within(form).getByLabelText("Name"), { key: "Escape" });
    fireEvent.keyDown(within(form).getByLabelText("LinkedIn page"), { key: "Escape" });
    const dialog = document.createElement("dialog");
    const inDialog = document.createElement("button");
    dialog.append(inDialog);
    panel.append(dialog);
    fireEvent.keyDown(inDialog, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    dialog.remove();
    fireEvent.keyDown(within(panel).getByRole("heading", { name: "Find HR · Contoh Logistik Sdn Bhd" }), { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("puts the focus on its title when it opens, and sticks under the bars at the top of the page", async () => {
    const bar = document.createElement("header");
    bar.className = "topbar";
    Object.defineProperty(bar, "offsetHeight", { value: 56 });
    const tabs = document.createElement("nav");
    tabs.className = "open-tabs";
    Object.defineProperty(tabs, "offsetHeight", { value: 40 });
    document.body.append(bar, tabs);
    try {
      serve();
      show();
      expect(document.activeElement).toBe(screen.getByRole("heading", { name: "Find HR · Contoh Logistik Sdn Bhd" }));
      const panel = screen.getByRole("complementary", { name: "Find HR for Contoh Logistik Sdn Bhd" });
      expect(panel.style.getPropertyValue("--lead-hr-top")).toBe("108px");
      await screen.findByText("Contoh Logistics Berhad");
    } finally {
      bar.remove();
      tabs.remove();
    }
  });

  it("says which company has the LinkedIn page already instead of saving it to a second", async () => {
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: false, version: null });
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/companies/{companyId}" ? { ...company, linkedinUrl: null }
        : path === "/v1/companies" ? { items: [{ id: "comp_7", name: "Contoh Group" }] } : { items: [] },
    ))) as never);
    const patch = vi.spyOn(api, "PATCH");
    show({ companyId: "comp_1" });
    const form = await screen.findByRole("form", { name: "Its LinkedIn page" });
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/contoh-logistik" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save page" }));
    expect(await within(form).findByText(/Contoh Group in HiringPlug has this LinkedIn page/)).toBeTruthy();
    expect(get).toHaveBeenCalledWith("/v1/companies", { params: { query: { linkedinCompany: "contoh-logistik", limit: 1 } } });
    expect(within(form).getByRole("link", { name: "Open Contoh Group ↗" }).getAttribute("href")).toBe("#/companies/comp_7");
    expect(within(form).getByRole<HTMLButtonElement>("button", { name: "Save page" }).disabled).toBe(true);
    expect(patch).not.toHaveBeenCalled();
  });

  it("saves the page again with the company's current version after someone else changed it", async () => {
    let version = 2;
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: false, version: null });
    const get = vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(ok(
      path === "/v1/companies/{companyId}" ? { ...company, version, linkedinUrl: null } : { items: [] },
    ))) as never);
    const companyReads = () => get.mock.calls.filter((c) => c[0] === "/v1/companies/{companyId}").length;
    const patch = vi.spyOn(api, "PATCH")
      .mockImplementationOnce((() => { version = 3; return Promise.resolve(failure("CONFLICT_STALE_VERSION", "This company was changed by someone else.", undefined, 409)); }) as never)
      .mockResolvedValueOnce(ok({ ...company, version: 4 }));
    show({ companyId: "comp_1" });
    const form = await screen.findByRole("form", { name: "Its LinkedIn page" });
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/contoh-logistik" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save page" }));
    expect(await within(form).findByText(/changed by someone else/)).toBeTruthy();
    // Read again: the company as it is now.
    await waitFor(() => { expect(companyReads()).toBe(2); });
    await waitFor(() => { expect(within(form).getByRole<HTMLButtonElement>("button", { name: "Save page" }).disabled).toBe(false); });
    fireEvent.click(within(form).getByRole("button", { name: "Save page" }));
    await waitFor(() => { expect(patch).toHaveBeenCalledTimes(2); });
    expect(sentBody(patch, 1)).toEqual({ version: 3, linkedinUrl: "https://www.linkedin.com/company/contoh-logistik/" });
  });

  it("shows a saved link that is not a company page, and replaces it only once asked", async () => {
    serve({ saved: { ...company, linkedinUrl: "https://lnkd.in/contoh" } });
    const patch = vi.spyOn(api, "PATCH").mockResolvedValue(ok(company));
    show({ companyId: "comp_1" });
    const form = await screen.findByRole("form", { name: "Its LinkedIn page" });
    expect(form.textContent).toContain("Its link in HiringPlug, https://lnkd.in/contoh, is not a LinkedIn company page.");
    fireEvent.change(within(form).getByLabelText("LinkedIn page"), { target: { value: "linkedin.com/company/contoh-logistik" } });
    fireEvent.click(within(form).getByRole("button", { name: "Save page" }));
    expect(await within(form).findByText("Replace https://lnkd.in/contoh with https://www.linkedin.com/company/contoh-logistik/?")).toBeTruthy();
    expect(patch).not.toHaveBeenCalled();
    fireEvent.click(within(form).getByRole("button", { name: "Replace it" }));
    await waitFor(() => { expect(patch).toHaveBeenCalled(); });
    expect(sentBody(patch)).toEqual({ version: 2, linkedinUrl: "https://www.linkedin.com/company/contoh-logistik/" });
  });

  it("sticks under the open records bar once it shows", async () => {
    const bar = document.createElement("header");
    bar.className = "topbar";
    Object.defineProperty(bar, "offsetHeight", { value: 56 });
    document.body.append(bar);
    const tabs = document.createElement("nav");
    tabs.className = "open-tabs";
    Object.defineProperty(tabs, "offsetHeight", { value: 40 });
    try {
      serve();
      show();
      const panel = screen.getByRole("complementary", { name: "Find HR for Contoh Logistik Sdn Bhd" });
      expect(panel.style.getPropertyValue("--lead-hr-top")).toBe("68px");
      // The first record opened: its bar shows under the top bar.
      document.body.append(tabs);
      await waitFor(() => { expect(panel.style.getPropertyValue("--lead-hr-top")).toBe("108px"); });
      tabs.remove();
      await waitFor(() => { expect(panel.style.getPropertyValue("--lead-hr-top")).toBe("68px"); });
      await screen.findByText("Contoh Logistics Berhad");
    } finally {
      bar.remove();
      tabs.remove();
    }
  });
});

