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
});
