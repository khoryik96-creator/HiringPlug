import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { Menu } from "./Menu";

afterEach(cleanup);

describe("Menu", () => {
  it("opens, runs an item and closes", () => {
    const chosen = vi.fn();
    const { container } = render(<Menu label="Invoice">{(close) => <button type="button" onClick={() => { close(); chosen(); }}>PDF</button>}</Menu>);
    const details = container.querySelector("details");
    if (!details) throw new Error("no menu");
    details.open = true;
    fireEvent(details, new Event("toggle"));
    expect(details.open).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "PDF" }));
    expect(chosen).toHaveBeenCalled();
    expect(details.open).toBe(false);
  });

  it("closes on a click outside it", () => {
    const { container } = render(<><Menu label="Invoice">{() => <button type="button">PDF</button>}</Menu><p>elsewhere</p></>);
    const details = container.querySelector("details");
    if (!details) throw new Error("no menu");
    details.open = true;
    fireEvent(details, new Event("toggle"));
    fireEvent.pointerDown(screen.getByRole("button", { name: "PDF" }));
    expect(details.open, "a click inside keeps it open").toBe(true);
    fireEvent.pointerDown(screen.getByText("elsewhere"));
    expect(details.open).toBe(false);
  });
});
