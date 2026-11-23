import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OpenTabs } from "./OpenTabs";
import { MAX_TABS, nameTab, recordOf, resetTabs } from "./openTabs";

beforeEach(() => { resetTabs(); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

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

  it("names each tab after what is open, marks the one you are on, and closes them", () => {
    const { rerender } = render(<OpenTabs hash="#/candidates/cand_01siti" />);
    expect(screen.getByRole("link", { name: /Candidate…/ })).toBeTruthy(); // until the page knows the name
    act(() => { nameTab("#/candidates/cand_01siti", "Siti Aisyah"); });
    rerender(<OpenTabs hash="#/jobs/jo_01acct" />);
    act(() => { nameTab("#/jobs/jo_01acct", "Accountant · Contoh Berhad"); });
    const nav = screen.getByRole("navigation", { name: "Open records" });
    expect([...nav.querySelectorAll(".open-tab-name")].map((n) => n.textContent)).toEqual(["Siti Aisyah", "Accountant · Contoh Berhad"]);
    expect(screen.getByRole("link", { name: /Accountant/ }).getAttribute("aria-current")).toBe("page");
    expect(screen.getByRole("link", { name: /Siti Aisyah/ }).getAttribute("href")).toBe("#/candidates/cand_01siti");

    fireEvent.click(screen.getByRole("button", { name: "Close Siti Aisyah" }));
    expect(screen.queryByRole("link", { name: /Siti Aisyah/ })).toBeNull();
  });

  it("keeps the newest few", () => {
    const { rerender } = render(<OpenTabs hash="#/candidates/cand_01a0" />);
    for (let n = 1; n <= MAX_TABS + 2; n++) rerender(<OpenTabs hash={`#/candidates/cand_01a${String(n)}`} />);
    expect(screen.getAllByRole("link")).toHaveLength(MAX_TABS);
    expect(screen.queryByRole("link", { name: /cand_01a0/ })).toBeNull();
  });
});
