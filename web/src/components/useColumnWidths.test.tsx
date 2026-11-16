import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useColumnWidths } from "./useColumnWidths";

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  vi.restoreAllMocks();
});

function Table({ name = "people" }: { name?: string }) {
  const ref = useRef<HTMLTableElement>(null);
  const w = useColumnWidths(name, ["name", "email"], ref);
  return (
    <>
      {w.set && <button type="button" onClick={w.reset}>Reset column widths</button>}
      <table ref={ref} className={w.set ? "sized" : undefined} style={w.tableStyle}>
        <thead><tr>
          <th {...w.head("name", "Name")}>Name{w.handle("name", "Name")}</th>
          <th {...w.head("email", "Email")}>Email{w.handle("email", "Email")}</th>
        </tr></thead>
        <tbody><tr><td>Nurul Aisyah</td><td>nurul@example.com</td></tr></tbody>
      </table>
    </>
  );
}

const header = (name: string) => screen.getByRole("columnheader", { name });

describe("column widths, like a spreadsheet", () => {
  it("are left to the table until one is set, then every column keeps its width", () => {
    render(<Table />);
    expect(header("Name").style.width).toBe("");
    expect(screen.getByRole("table").className).toBe("");
    const edge = screen.getByRole("separator", { name: /Width of Name/ });
    fireEvent.keyDown(edge, { key: "ArrowRight" }); // the narrowest is 48
    fireEvent.keyDown(edge, { key: "ArrowRight" });
    expect(header("Name").style.width).toBe("64px");
    expect(header("Email").style.width, "the others keep the width they have").toBe("48px");
    expect(screen.getByRole("table").className).toBe("sized");
    expect(screen.getByRole("table").style.width).toBe("112px");
    fireEvent.keyDown(edge, { key: "ArrowLeft" });
    expect(header("Name").style.width).toBe("48px");
  });

  it("are remembered for that list on this device, and can be reset", () => {
    const first = render(<Table />);
    fireEvent.keyDown(screen.getByRole("separator", { name: /Width of Email/ }), { key: "ArrowRight" });
    fireEvent.keyDown(screen.getByRole("separator", { name: /Width of Email/ }), { key: "ArrowRight" });
    first.unmount();
    render(<Table />);
    expect(header("Email").style.width).toBe("64px");
    cleanup();
    render(<Table name="other" />);
    expect(header("Email").style.width, "another list has its own").toBe("");
    cleanup();
    render(<Table />);
    fireEvent.click(screen.getByRole("button", { name: "Reset column widths" }));
    expect(header("Email").style.width).toBe("");
    expect(window.localStorage.getItem("hiringplug.columnWidths.people")).toBeNull();
  });

  it("does not sort or choose when the edge is clicked", () => {
    let clicks = 0;
    function Clickable() {
      const ref = useRef<HTMLTableElement>(null);
      const w = useColumnWidths("x", ["name"], ref);
      return <table ref={ref}><thead><tr><th data-column="name" onClick={() => { clicks++; }}>Name{w.handle("name", "Name")}</th></tr></thead></table>;
    }
    render(<Clickable />);
    fireEvent.click(screen.getByRole("separator"));
    expect(clicks).toBe(0);
  });

  it("are named by their label alone, not the drag edge's instructions", () => {
    render(<Table />);
    expect(screen.getAllByRole("columnheader").map((h) => h.getAttribute("aria-label"))).toEqual(["Name", "Email"]);
    expect(screen.getByRole("columnheader", { name: "Name" })).toBeTruthy();
  });

  it("keep the width reached when the browser takes the pointer away mid-drag", () => {
    // jsdom has no pointer capture: a stand-in records the call.
    const capture = vi.fn();
    Object.defineProperty(HTMLElement.prototype, "setPointerCapture", { value: capture, configurable: true });
    render(<Table />);
    const edge = screen.getByRole("separator", { name: /Width of Name/ });
    fireEvent.pointerDown(edge, { button: 0, clientX: 100, pointerId: 1 });
    expect(capture).toHaveBeenCalled();
    expect(document.body.classList.contains("resizing-columns")).toBe(true);
    fireEvent.pointerMove(edge, { clientX: 180, pointerId: 1 }); // jsdom draws the column 0 wide: 0 + 80
    fireEvent.pointerCancel(edge, { pointerId: 1 });
    expect(document.body.classList.contains("resizing-columns"), "the drag is over").toBe(false);
    expect(header("Name").style.width).toBe("80px");
    fireEvent.pointerMove(edge, { clientX: 400, pointerId: 1 });
    expect(header("Name").style.width, "moving no longer resizes").toBe("80px");
    expect(window.localStorage.getItem("hiringplug.columnWidths.people"), "and it is kept").toContain("\"name\":80");
    delete (HTMLElement.prototype as { setPointerCapture?: unknown }).setPointerCapture;
  });
});
