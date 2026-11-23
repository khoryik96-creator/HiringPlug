import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { api, type Permission, type Schemas } from "../api/client";
import { ok, renderWithClient, testMe } from "../test-utils";
import { INLINE, inlineTabs, OpenTabs } from "./OpenTabs";
import { forgetTabs, MAX_TABS, nameTab, openTab, recordOf, resetTabs, setTabsOwner } from "./openRecords";

beforeEach(() => { resetTabs(); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function bell(items: Schemas["Notification"][] = [], permissions: Permission[] = ["notifications:read"]) {
  return vi.spyOn(api, "GET").mockImplementation(((path: string) => Promise.resolve(path === "/v1/me" ? ok(testMe(permissions))
    : ok({ unseen: items.length, morningEmail: false, mutedKinds: [], asOf: "2026-11-23T02:00:00Z", items }))) as never);
}

/** The paths a mocked GET was asked for. */
const paths = (get: { mock: { calls: unknown[][] } }) => get.mock.calls.map((c) => c[0]);

/** Opens these records one after another, naming each tab; ends on the last. */
function openAll(records: [string, string][]) {
  const view = renderWithClient(<OpenTabs hash={records[0]?.[0] ?? ""} />);
  for (const [href, name] of records) {
    view.rerender(<OpenTabs hash={href} />);
    act(() => { nameTab(href, name); });
  }
  return view;
}
const names = () => [...screen.getByRole("navigation", { name: "Open records" }).querySelectorAll(".open-tab-name")].map((n) => n.textContent);

describe("open records", () => {
  it("knows which record a page is about", () => {
    expect(recordOf("#/candidates/cand_01abc")).toEqual({ href: "#/candidates/cand_01abc", kind: "candidate" });
    expect(recordOf("#/candidates/cand_01abc/screening")).toEqual({ href: "#/candidates/cand_01abc", kind: "candidate" });
    expect(recordOf("#/candidates/cand_01abc?hl=SAP")?.href).toBe("#/candidates/cand_01abc");
    expect(recordOf("#/jobs/jo_01abc")?.kind).toBe("job");
    expect(recordOf("#/contacts/cont_01abc")?.kind).toBe("contact");
    expect(recordOf("#/companies/comp_01abc")?.kind).toBe("company");
    expect(recordOf("#/candidates")).toBeNull();
    expect(recordOf("#/companies/duplicates")).toBeNull();
  });

  it("names each tab after what is open, grouped by kind, and marks the one you are on", () => {
    bell();
    const view = renderWithClient(<OpenTabs hash="#/candidates/cand_01siti" />);
    expect(screen.getByRole("link", { name: /Candidate…/ })).toBeTruthy(); // until the page knows the name
    act(() => { nameTab("#/candidates/cand_01siti", "Siti Aisyah"); });
    view.rerender(<OpenTabs hash="#/jobs/jo_01acct" />);
    act(() => { nameTab("#/jobs/jo_01acct", "Accountant · Contoh Berhad"); });
    view.rerender(<OpenTabs hash="#/candidates/cand_01lim" />);
    act(() => { nameTab("#/candidates/cand_01lim", "Lim Wei"); });
    expect(names()).toEqual(["Siti Aisyah", "Lim Wei", "Accountant · Contoh Berhad"]);
    expect(within(screen.getByRole("group", { name: "Jobs" })).getByRole("link").textContent).toBe("Accountant · Contoh Berhad");
    expect(screen.getByRole("link", { name: /Lim Wei/ }).getAttribute("aria-current")).toBe("page");
    fireEvent.click(screen.getByRole("button", { name: "Close Siti Aisyah" }));
    expect(names()).toEqual(["Lim Wei", "Accountant · Contoh Berhad"]);
  });

  it("selects with Ctrl-click and Shift-click, and closes what is selected, or all", () => {
    bell();
    openAll([["#/candidates/cand_01a", "Aina"], ["#/candidates/cand_01b", "Badrul"], ["#/candidates/cand_01c", "Chong"], ["#/contacts/cont_01d", "Devi"]]);
    fireEvent.click(screen.getByRole("link", { name: /Aina/ }), { ctrlKey: true });
    fireEvent.click(screen.getByRole("link", { name: /Chong/ }), { ctrlKey: true });
    expect(screen.getByText("2 selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("link", { name: /Chong/ }), { ctrlKey: true }); // picked again: left out
    expect(screen.getByText("1 selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("link", { name: /Devi/ }), { shiftKey: true }); // a run, from Chong to Devi
    expect(screen.getByText("2 selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Close selected" }));
    expect(names()).toEqual(["Aina", "Badrul"]);
    expect(window.location.hash).toBe("#/candidates/cand_01b"); // you were on Devi: the tab beside opens
    fireEvent.click(screen.getByRole("button", { name: "Select all" }));
    expect(screen.getByText("2 selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    fireEvent.click(screen.getByRole("button", { name: "Close all" }));
    expect(screen.queryByRole("navigation", { name: "Open records" })).toBeNull();
  });

  it("lights a tab green when its CV is formatted, until it is opened", async () => {
    bell([{ id: "cv_formatted:scv_1", kind: "cv_formatted", title: "CV formatted: Aina", detail: "", href: "#/candidates/cand_01a", at: "2026-11-23T01:00:00Z", new: true }]);
    const view = openAll([["#/candidates/cand_01a", "Aina"], ["#/jobs/jo_01j", "Accountant"]]);
    const tab = () => screen.getByRole("link", { name: /Aina/ }).closest(".open-tab");
    await waitFor(() => { expect(tab()?.className).toContain("done"); });
    view.rerender(<OpenTabs hash="#/candidates/cand_01a" />);
    expect(tab()?.className).not.toContain("done");
    view.rerender(<OpenTabs hash="#/jobs/jo_01j" />);
    expect(tab()?.className).not.toContain("done"); // seen: not lit again by the same notification
  });

  it("never lights a record you were looking at when it finished, or opened since", async () => {
    bell([
      { id: "cv_formatted:scv_2", kind: "cv_formatted", title: "CV formatted: Badrul", detail: "", href: "#/candidates/cand_01b", at: "2026-11-23T01:00:00Z", new: true },
      { id: "cv_formatted:scv_1", kind: "cv_formatted", title: "CV formatted: Aina", detail: "", href: "#/candidates/cand_01a", at: "2026-11-23T01:00:00Z", new: true },
    ]);
    const view = renderWithClient(<OpenTabs hash="#/candidates/cand_01a" />); // the news comes while on Aina's page
    await waitFor(() => { expect(paths(vi.mocked(api.GET)).includes("/v1/notifications")).toBe(true); });
    await act(async () => { await new Promise((r) => setTimeout(r, 0)); });
    view.rerender(<OpenTabs hash="#/candidates/cand_01b" />); // Badrul's tab opened afterwards: you see it there
    act(() => { nameTab("#/candidates/cand_01b", "Badrul"); nameTab("#/candidates/cand_01a", "Aina"); });
    view.rerender(<OpenTabs hash="#/jobs/jo_01j" />);
    await waitFor(() => { expect(screen.getByRole("link", { name: /Badrul/ }).closest(".open-tab")?.className).not.toContain("done"); });
    expect(screen.getByRole("link", { name: /Aina/ }).closest(".open-tab")?.className).not.toContain("done"); // seen on its page
  });

  it("with many open, keeps the latest few of a kind in the row (and the one you are on) and the rest under +N", () => {
    const list = Array.from({ length: 9 }, (_, n) => ({ href: `#/candidates/cand_0${String(n)}` }));
    const { inline, more } = inlineTabs(list, "#/candidates/cand_01");
    expect(inline.map((t) => t.href)).toEqual(["#/candidates/cand_01", ...list.slice(-INLINE).map((t) => t.href)]);
    expect(more.map((t) => t.href)).toEqual(["#/candidates/cand_04", "#/candidates/cand_03", "#/candidates/cand_02", "#/candidates/cand_00"]); // newest first
    expect(inlineTabs(list.slice(0, INLINE + 1)).more).toEqual([]); // one more than fits: no "+1"
  });

  it("finds an open record by name", () => {
    bell();
    openAll(Array.from({ length: 8 }, (_, n) => [`#/candidates/cand_0${String(n)}`, n === 2 ? "Nurul Huda" : `Person ${String(n)}`] as [string, string]));
    expect(screen.getByText("+4")).toBeTruthy(); // 8 open: the latest 4 in the row (you are on the last)
    fireEvent.change(screen.getByRole("searchbox", { name: "Find an open record" }), { target: { value: "nurul" } });
    const found = screen.getByRole("dialog", { name: "All open records" });
    expect(within(found).getAllByRole("link").map((a) => a.textContent)).toEqual(["Nurul Huda"]);
    fireEvent.keyDown(screen.getByRole("searchbox", { name: "Find an open record" }), { key: "Enter" });
    expect(window.location.hash).toBe("#/candidates/cand_02");
  });

  it("are the signed-in user's own, forgotten on signing out", () => {
    bell();
    act(() => { openTab("#/candidates/cand_01a", "candidate"); nameTab("#/candidates/cand_01a", "Aina"); });
    renderWithClient(<OpenTabs hash="#/dashboard" />);
    expect(screen.getByRole("link", { name: /Aina/ })).toBeTruthy();
    act(() => { forgetTabs(); setTabsOwner("tnt_test:usr_other"); }); // someone else signs in on this browser
    expect(screen.queryByRole("navigation", { name: "Open records" })).toBeNull();
    act(() => { forgetTabs(); setTabsOwner("tnt_test:usr_test"); }); // the first comes back: signing out forgot them
    expect(screen.queryByRole("navigation", { name: "Open records" })).toBeNull();
  });

  it("follows another window of the browser", () => {
    bell();
    renderWithClient(<OpenTabs hash="#/candidates/cand_01a" />);
    const theirs = [{ href: "#/candidates/cand_01a", kind: "candidate", label: "Aina" }, { href: "#/jobs/jo_01j", kind: "job", label: "Accountant" }];
    act(() => {
      localStorage.setItem("hp.openTabs:tnt_test:usr_test", JSON.stringify(theirs));
      window.dispatchEvent(new StorageEvent("storage", { key: "hp.openTabs:tnt_test:usr_test" }));
    });
    expect(screen.getByRole("link", { name: /Accountant/ })).toBeTruthy();
  });

  it("asks before closing the page you typed into", () => {
    bell();
    const ask = vi.spyOn(window, "confirm").mockReturnValue(false);
    renderWithClient(<><OpenTabs hash="#/candidates/cand_01a" /><form><input aria-label="Notes" /></form></>);
    act(() => { nameTab("#/candidates/cand_01a", "Aina"); });
    fireEvent.input(screen.getByRole("textbox", { name: "Notes" }), { target: { value: "Wants 15k" } });
    fireEvent.click(screen.getByRole("button", { name: "Close Aina" }));
    expect(ask).toHaveBeenCalled();
    expect(screen.getByRole("link", { name: /Aina/ })).toBeTruthy(); // kept: said no
  });

  it("asks nobody for the bell's list without the permission", async () => {
    const get = bell([], []);
    renderWithClient(<OpenTabs hash="#/candidates/cand_01a" />);
    await waitFor(() => { expect(paths(get).includes("/v1/me")).toBe(true); });
    await act(async () => { await new Promise((r) => setTimeout(r, 20)); });
    expect(paths(get).includes("/v1/notifications")).toBe(false);
  });

  it("keeps the newest few", () => {
    bell();
    const view = renderWithClient(<OpenTabs hash="#/candidates/cand_01a0" />);
    for (let n = 1; n <= MAX_TABS + 2; n++) view.rerender(<OpenTabs hash={`#/candidates/cand_01a${String(n)}`} />);
    expect(new Set(screen.getAllByRole("link").map((a) => a.getAttribute("href"))).size).toBe(MAX_TABS);
    expect(screen.getByText(`All open (${String(MAX_TABS)})`)).toBeTruthy();
  });
});
