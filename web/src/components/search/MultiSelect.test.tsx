import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MultiSelect } from "./MultiSelect";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe("MultiSelect", () => {
  it("opens on the other side when its preferred side runs off the window", () => {
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({ left: -24, right: 296, top: 0, bottom: 0, width: 320, height: 0, x: -24, y: 0, toJSON: () => ({}) });
    render(<MultiSelect label="Subindustry" align="end" choices={[{ value: "a", label: "FSI – Banking" }]} value={[]} onChange={() => undefined} />);
    fireEvent.click(screen.getByRole("button", { name: /Subindustry/ }));
    expect(screen.getByRole("listbox", { name: "Subindustry" }).className).not.toContain("align-end");
  });

  it("keeps its preferred side when it fits", () => {
    render(<MultiSelect label="Subindustry" align="end" choices={[{ value: "a", label: "FSI – Banking" }]} value={[]} onChange={() => undefined} />);
    fireEvent.click(screen.getByRole("button", { name: /Subindustry/ }));
    expect(screen.getByRole("listbox", { name: "Subindustry" }).className).toContain("align-end");
  });

  it("lists the person's favourites first, starred, and a star adds or removes one", () => {
    const toggle = vi.fn();
    const fav = { list: ["SAP"], has: (v: string) => v === "SAP", toggle };
    render(<MultiSelect label="IT skills" choices={["Java", "Python", "SAP"].map((v) => ({ value: v, label: v }))} value={[]} onChange={() => undefined} fav={fav} />);
    fireEvent.click(screen.getByRole("button", { name: /IT skills/ }));
    expect(screen.getAllByRole("option").map((o) => o.textContent), "favourites first, then all the rest").toEqual(["SAP", "Java", "Python"]);
    expect(screen.getByText("★ Your favourites")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Remove SAP from your favourites" }).getAttribute("aria-pressed")).toBe("true");
    fireEvent.click(screen.getByRole("button", { name: "Keep Java at the top (favourite)" }));
    expect(toggle).toHaveBeenCalledWith("Java");
    expect((screen.getByRole("option", { name: "Java" }).querySelector("input") as HTMLInputElement).checked, "a star does not tick the item").toBe(false);
  });
});
