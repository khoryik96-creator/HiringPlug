import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Schemas } from "../../api/client";
import { looksLikeWork, MeaningBanner, MeaningSwitch, readingInterval, WhyLine } from "./MeaningSearch";

afterEach(() => { cleanup(); });

const why: Schemas["MeaningWhy"] = {
  score: 0.88, strength: "strong", title: "Account Executive", titleWhen: "now", family: "accounting", level: "Executive",
  passage: "Full set of accounts and bank reconciliations", words: ["accounts"],
};

describe("search by meaning", () => {
  it("says how the search was read: the role, its field, the level it names and the rest", () => {
    const { container } = render(<MeaningBanner text="accounts executive for an audit firm" tooLong={false} onWords={() => undefined}
      outcome={{ status: "ready", read: 101, total: 101,
        understood: { role: "accounts executive", family: "accounting", level: 2, levelName: "Executive", context: "an audit firm", describes: false } }} />);
    expect(container.textContent).toBe(
      "Searching for accounts executive (accounting), executive level first · an audit firm, whatever their titles say. Best matches first.");
  });

  it("says work described is matched with people's work, and how far the CVs are read", () => {
    const { container } = render(<MeaningBanner text="someone who hired engineers" tooLong={false} onWords={() => undefined}
      outcome={{ status: "ready", read: 3, total: 10, understood: { family: "recruitment", describes: true } }} />);
    expect(container.textContent).toBe(
      "People whose work matches “someone who hired engineers” (recruitment). Best matches first. Still reading CVs: 3 of 10 candidates so far.");
  });

  it("while the words are read says so, and offers the words search when it takes long or is not set up", () => {
    const onWords = vi.fn();
    const { rerender } = render(<MeaningBanner outcome={{ status: "reading", understood: null }} text="headhunter" tooLong={false} onWords={onWords} />);
    expect(screen.getByRole("status").textContent).toBe(" Reading “headhunter” the way a recruiter would…");
    expect(screen.queryByRole("button")).toBeNull();
    rerender(<MeaningBanner outcome={{ status: "reading", understood: null }} text="headhunter" tooLong onWords={onWords} />);
    fireEvent.click(screen.getByRole("button", { name: "Search by words instead" }));
    expect(onWords).toHaveBeenCalledTimes(1);
    rerender(<MeaningBanner outcome={{ status: "unavailable", understood: null }} text="headhunter" tooLong={false} onWords={onWords} />);
    expect(screen.getByRole("status").textContent).toContain("not set up on this server");
    fireEvent.click(screen.getByRole("button", { name: "Search by words instead" }));
    expect(onWords).toHaveBeenCalledTimes(2);
  });

  it("says why each candidate was found: how strong, their title and its field, and where their CV says it", () => {
    const { container, rerender } = render(<WhyLine why={why} />);
    expect(container.textContent).toBe("Strong matchas Account Executive (now) · accountingFull set of accounts and bank reconciliationsCV says: accounts");
    expect(container.querySelector("q")?.textContent).toBe("Full set of accounts and bank reconciliations");
    rerender(<WhyLine why={why} showLevel />); // the search named a level
    expect(container.textContent).toContain("· accounting · level: executive");
    rerender(<WhyLine why={{ score: 0.4, strength: "possible", titleWhen: "until 2013", title: "Recruitment Consultant", words: [] }} />);
    expect(container.textContent).toBe("Possible matchas Recruitment Consultant (until 2013)");
  });

  it("switches on and off from the search box", () => {
    const onChange = vi.fn();
    const { rerender } = render(<MeaningSwitch on={false} onChange={onChange} />);
    const pill = screen.getByRole("button", { name: /Meaning/ });
    expect(pill.getAttribute("aria-pressed")).toBe("false");
    fireEvent.click(pill);
    expect(onChange).toHaveBeenCalledWith(true);
    rerender(<MeaningSwitch on onChange={onChange} />);
    expect(screen.getByRole("button", { name: /Meaning/ }).getAttribute("aria-pressed")).toBe("true");
  });

  it("offers itself for words that read as a role or work, not a name, email or phone", () => {
    expect(looksLikeWork("talent acquisition")).toBe(true);
    expect(looksLikeWork("aina@example.com")).toBe(false);
    expect(looksLikeWork("012 345 6789")).toBe(false);
    expect(looksLikeWork("hr")).toBe(false);
  });

  it("asks again soon while the words are read, then seldom", () => {
    expect(readingInterval(false, 0)).toBe(false);
    expect(readingInterval(true, 0)).toBe(700);
    expect(readingInterval(true, 21)).toBe(5000);
  });
});
