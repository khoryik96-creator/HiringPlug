import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { FavDropdown } from "./FavDropdown";

afterEach(() => { cleanup(); });

const choices = ["Java", "Python"].map((v) => ({ value: v, label: v }));

describe("FavDropdown", () => {
  it("starts afresh after a click elsewhere", () => {
    render(<><FavDropdown aria-label="Add an IT skill" text="Add" noun="IT skills" choices={choices} onPick={() => undefined} /><p>elsewhere</p></>);
    fireEvent.click(screen.getByRole("button", { name: "Add an IT skill" }));
    fireEvent.change(screen.getByLabelText("Find IT skills"), { target: { value: "xyz" } });
    expect(screen.getByText("None match.")).toBeTruthy();
    fireEvent.mouseDown(screen.getByText("elsewhere"));
    fireEvent.click(screen.getByRole("button", { name: "Add an IT skill" }));
    expect(screen.getByLabelText<HTMLInputElement>("Find IT skills").value, "the old search is gone").toBe("");
    expect(screen.getByRole("button", { name: "Java" })).toBeTruthy();
  });

  it("gives the focus back to its button on Escape", () => {
    render(<FavDropdown aria-label="Add an IT skill" text="Add" noun="IT skills" choices={choices} onPick={() => undefined} />);
    const button = screen.getByRole("button", { name: "Add an IT skill" });
    fireEvent.click(button);
    fireEvent.keyDown(screen.getByLabelText("Find IT skills"), { key: "Escape" });
    expect(screen.queryByLabelText("Find IT skills")).toBeNull();
    expect(document.activeElement).toBe(button);
  });
});
