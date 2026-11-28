import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api, type Schemas } from "../api/client";
import { Access } from "../pages/Access";
import { invalidField, ok, renderWithClient, sentBody } from "../test-utils";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const ORG = "72f988bf-86f1-41af-91ab-2d7cd011db47";
const CALLBACK = "https://ats.contoh.my/v1/auth/microsoft/callback";
const settings = (over: Partial<Schemas["MicrosoftSignInSettings"]> = {}): Schemas["MicrosoftSignInSettings"] => ({
  enabled: false, organisationId: null, required: false, callbackUrl: CALLBACK, appSource: "agency", clientId: "c1d2", acctClaimMissing: false, connectsOutlook: false, ...over,
});
const access: Schemas["AccessSettings"] = { roles: [], permissions: [] };
const twoStep = { enabled: true, enabledAt: null, recoveryCodesLeft: 10, required: false };

function serve(ms: Schemas["MicrosoftSignInSettings"]) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(
    path === "/v1/agency/microsoft-sign-in" ? ok(ms) : path === "/v1/me/two-step" ? ok(twoStep)
      : path === "/v1/access/named-roles" ? ok({ items: [] }) : ok(access))) as never);
}

describe("Roles and access › Microsoft sign-in", () => {
  it("is for owners only", async () => {
    const get = serve(settings());
    renderWithClient(<Access />);
    expect(await screen.findByRole("region", { name: "Two-step sign-in for everyone" })).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Microsoft sign-in" })).toBeNull();
    expect((get.mock.calls as unknown as [string][]).some(([path]) => path === "/v1/agency/microsoft-sign-in")).toBe(false);
    cleanup();
    renderWithClient(<Access owner />);
    expect(await screen.findByRole("region", { name: "Microsoft sign-in" })).toBeTruthy();
  });

  it("shows the address to register, with a copy button, and whose app it is", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    serve(settings());
    renderWithClient(<Access owner />);
    const box = await screen.findByRole("region", { name: "Microsoft sign-in" });
    expect(await within(box).findByText(CALLBACK)).toBeTruthy();
    expect(within(box).getByRole("link", { name: "Settings › Email connection" }).getAttribute("href")).toBe("#/email-connection");
    fireEvent.click(within(box).getByRole("button", { name: "Copy redirect address" }));
    await waitFor(() => { expect(writeText).toHaveBeenCalledWith(CALLBACK); });
    Reflect.deleteProperty(navigator, "clipboard");
  });

  it("explains how to set an app up when there is none, or names the server's", async () => {
    serve(settings({ appSource: "none", clientId: null }));
    renderWithClient(<Access owner />);
    expect(await screen.findByText(/There is no Microsoft app yet/)).toBeTruthy();
    cleanup();
    serve(settings({ appSource: "server" }));
    renderWithClient(<Access owner />);
    expect(await screen.findByText(/this server's Microsoft app/)).toBeTruthy();
  });

  it("warns that Microsoft refuses a plain http address other than localhost", async () => {
    serve(settings({ callbackUrl: "http://ats.contoh.my/v1/auth/microsoft/callback" }));
    renderWithClient(<Access owner />);
    expect(await screen.findByText(/Microsoft accepts only https addresses/)).toBeTruthy();
    cleanup();
    serve(settings({ callbackUrl: "http://localhost:5173/v1/auth/microsoft/callback" }));
    renderWithClient(<Access owner />);
    await screen.findByRole("button", { name: "Save" });
    expect(screen.queryByText(/Microsoft accepts only https addresses/)).toBeNull();
  });

  it("saves the team's sign-in, asking before requiring Microsoft, and says it is recorded", async () => {
    serve(settings());
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(settings({ enabled: true, organisationId: ORG, required: true })));
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    renderWithClient(<Access owner />);
    const required = await screen.findByLabelText<HTMLInputElement>(/Everyone must sign in with Microsoft/);
    expect(required.disabled).toBe(true); // only once the team may sign in with Microsoft
    fireEvent.click(screen.getByLabelText(/Let the team sign in with Microsoft/));
    fireEvent.change(screen.getByLabelText(/Your Microsoft organisation/), { target: { value: ` ${ORG} ` } });
    fireEvent.click(required);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(confirm).toHaveBeenCalled();
    expect(sentBody(put)).toEqual({ enabled: true, organisationId: ORG, required: true, connectsOutlook: false });
    expect((await screen.findByRole("status")).textContent).toContain("audit log");
  });

  it("shows what is wrong beside the field", async () => {
    serve(settings());
    vi.spyOn(api, "PUT").mockResolvedValue(invalidField("organisationId", "enter the Directory (tenant) ID"));
    renderWithClient(<Access owner />);
    fireEvent.click(await screen.findByLabelText(/Let the team sign in with Microsoft/));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect((await screen.findByRole("alert")).textContent).toBe("Organisation: enter the Directory (tenant) ID");
    expect(screen.getByLabelText(/Your Microsoft organisation/).getAttribute("aria-invalid")).toBe("true");
  });

  it("warns before changing the organisation or turning it off: links and Microsoft sessions end", async () => {
    serve(settings({ enabled: true, organisationId: ORG }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(settings({ enabled: true, organisationId: ORG })));
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderWithClient(<Access owner />);
    const box = await screen.findByRole("region", { name: "Microsoft sign-in" });
    expect((await within(box).findByText(/optional claims/)).textContent).toContain("xms_edov");
    fireEvent.change(within(box).getByLabelText(/Your Microsoft organisation/), { target: { value: "11111111-2222-3333-4444-555555555555" } });
    fireEvent.click(within(box).getByRole("button", { name: "Save" }));
    expect(confirm.mock.calls[0]?.[0]).toContain("Every Microsoft account linked to the team is unlinked");
    expect(put).not.toHaveBeenCalled();
    fireEvent.change(within(box).getByLabelText(/Your Microsoft organisation/), { target: { value: ORG.toUpperCase() } });
    fireEvent.click(within(box).getByLabelText(/Let the team sign in with Microsoft/));
    fireEvent.click(within(box).getByRole("button", { name: "Save" }));
    expect(confirm.mock.calls[1]?.[0]).toContain("Turn Microsoft sign-in off?"); // the same organisation, in capitals: no warning for it
    expect(confirm).toHaveBeenCalledTimes(2);
    confirm.mockReturnValue(true);
    fireEvent.click(within(box).getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ enabled: false, organisationId: ORG.toUpperCase(), required: false, connectsOutlook: false });
  });
  it("warns when the last Microsoft sign-in came without the acct claim", async () => {
    serve(settings({ enabled: true, organisationId: ORG, acctClaimMissing: true }));
    renderWithClient(<Access owner />);
    const warning = await screen.findByRole("alert");
    expect(warning.textContent).toContain("Add the acct optional claim in Microsoft Entra.");
    expect(warning.textContent).toContain("links no account to the team by its address");
    cleanup();
    serve(settings({ enabled: true, organisationId: ORG }));
    renderWithClient(<Access owner />);
    await screen.findByRole("region", { name: "Microsoft sign-in" });
    await screen.findByText(/optional claims/);
    expect(screen.queryByText(/Add the/)).toBeNull();
  });

  it("turns on connecting Outlook at sign-in, kept with the rest", async () => {
    serve(settings({ enabled: true, organisationId: ORG }));
    const put = vi.spyOn(api, "PUT").mockResolvedValue(ok(settings({ enabled: true, organisationId: ORG, connectsOutlook: true })));
    renderWithClient(<Access owner />);
    const box = await screen.findByRole("region", { name: "Microsoft sign-in" });
    const outlook = await within(box).findByRole<HTMLInputElement>("checkbox", { name: /Also connect Outlook mail and calendar/ });
    fireEvent.click(outlook);
    fireEvent.click(within(box).getByRole("button", { name: "Save" }));
    await waitFor(() => { expect(put).toHaveBeenCalled(); });
    expect(sentBody(put)).toEqual({ enabled: true, organisationId: ORG, required: false, connectsOutlook: true });
    await waitFor(() => { expect(outlook.checked).toBe(true); });
  });
});
