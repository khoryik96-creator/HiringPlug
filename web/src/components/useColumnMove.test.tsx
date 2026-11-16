import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { drawHeadings } from "../test-utils";
import { moved, useColumnMove, useStoredOrder } from "./useColumnMove";

afterEach(() => {
  cleanup();
  window.localStorage.clear();
  vi.restoreAllMocks();
});


const LABELS: Record<string, string> = { email: "Email", phone: "Phone", city: "City" };

function Table({ onSort = () => undefined }: { onSort?: (c: string) => void }) {
  const ref = useRef<HTMLTableElement>(null);
  const cols = useStoredOrder("people", ["email", "phone", "city"]);
  const move = useColumnMove(ref, cols.order, cols.save);
  return (
    <>
      {cols.changed && <button type="button" onClick={cols.reset}>Reset column order</button>}
      <table ref={ref}>
        <thead><tr>
          <th data-column="name">Name</th>
          {cols.order.map((c) => (
            <th key={c} data-column={c} {...move.props(c)}>
              <button type="button" onClick={() => { onSort(c); }}>{LABELS[c]}</button>
            </th>
          ))}
        </tr></thead>
        <tbody><tr><td>Nurul</td>{cols.order.map((c) => <td key={c}>{c}-value</td>)}</tr></tbody>
      </table>
    </>
  );
}

const headings = () => screen.getAllByRole("columnheader").map((h) => h.textContent);

/** Presses on a heading, moves to x, and lets go there. */
function drag(label: string, x: number) {
  const th = screen.getByRole("columnheader", { name: label });
  const button = screen.getByRole("button", { name: label });
  fireEvent.pointerDown(button, { button: 0, clientX: th.getBoundingClientRect().left + 50 });
  fireEvent.pointerMove(document, { clientX: x });
  fireEvent.pointerUp(document, { clientX: x });
  fireEvent.click(button); // the browser's click at the end of the press
}

describe("moving columns by their headings, like a spreadsheet", () => {
  it("puts a column where its heading is let go, cells too, and keeps it on this device", () => {
    drawHeadings();
    const sorted = vi.fn();
    const first = render(<Table onSort={sorted} />);
    expect(headings()).toEqual(["Name", "Email", "Phone", "City"]);
    drag("City", 120); // over the left half of Email
    expect(headings()).toEqual(["Name", "City", "Email", "Phone"]);
    expect(screen.getByText("Nurul").closest("tr")?.textContent).toBe("Nurulcity-valueemail-valuephone-value");
    expect(sorted, "letting go does not also sort").not.toHaveBeenCalled();
    first.unmount();

    render(<Table />);
    expect(headings(), "remembered").toEqual(["Name", "City", "Email", "Phone"]);
    fireEvent.click(screen.getByRole("button", { name: "Reset column order" }));
    expect(headings()).toEqual(["Name", "Email", "Phone", "City"]);
    expect(window.localStorage.getItem("hiringplug.columnOrder.people")).toBeNull();
  });

  it("past the last heading puts the column at the end", () => {
    drawHeadings();
    render(<Table />);
    drag("Email", 900);
    expect(headings()).toEqual(["Name", "Phone", "City", "Email"]);
  });

  it("on a touch screen, the late click that ends a drag does not sort either", () => {
    drawHeadings();
    const sorted = vi.fn();
    render(<Table onSort={sorted} />);
    vi.useFakeTimers();
    try {
      const button = screen.getByRole("button", { name: "City" });
      fireEvent.pointerDown(button, { button: 0, clientX: 350, pointerType: "touch" });
      fireEvent.pointerMove(document, { clientX: 120, pointerType: "touch" });
      fireEvent.pointerUp(document, { clientX: 120, pointerType: "touch" });
      vi.advanceTimersByTime(300); // a finger's click comes later than a mouse's
      fireEvent.click(screen.getByRole("button", { name: "City" }));
      expect(sorted).not.toHaveBeenCalled();
      expect(headings()).toEqual(["Name", "City", "Email", "Phone"]);
      vi.advanceTimersByTime(600);
      fireEvent.click(screen.getByRole("button", { name: "City" }));
      expect(sorted, "a later tap sorts as usual").toHaveBeenCalledWith("city");
    } finally {
      vi.useRealTimers();
    }
  });

  it("still sorts on a click that does not move", () => {
    drawHeadings();
    const sorted = vi.fn();
    render(<Table onSort={sorted} />);
    const button = screen.getByRole("button", { name: "Phone" });
    fireEvent.pointerDown(button, { button: 0, clientX: 250 });
    fireEvent.pointerMove(document, { clientX: 253 }); // a hand's tremble, not a drag
    fireEvent.pointerUp(document, { clientX: 253 });
    fireEvent.click(button);
    expect(sorted).toHaveBeenCalledWith("phone");
    expect(headings()).toEqual(["Name", "Email", "Phone", "City"]);
  });

  it("shows where the column will go while it is dragged", () => {
    drawHeadings();
    render(<Table />);
    fireEvent.pointerDown(screen.getByRole("button", { name: "Email" }), { button: 0, clientX: 150 });
    fireEvent.pointerMove(document, { clientX: 380 }); // the right half of City
    expect(screen.getByRole("columnheader", { name: "Email" }).className).toContain("moving");
    expect(screen.getByRole("columnheader", { name: "City" }).className).toContain("drop-after");
    expect(document.body.classList.contains("moving-column")).toBe(true);
    fireEvent.pointerUp(document, { clientX: 380 });
    expect(document.body.classList.contains("moving-column")).toBe(false);
    expect(headings()).toEqual(["Name", "Phone", "City", "Email"]);
  });

  it("moves with Alt+Shift+arrows from the keyboard", () => {
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((f) => { f(0); return 0; });
    render(<Table />);
    act(() => { screen.getByRole("button", { name: "Email" }).focus(); });
    fireEvent.keyDown(screen.getByRole("button", { name: "Email" }), { key: "ArrowRight", altKey: true, shiftKey: true });
    expect(headings()).toEqual(["Name", "Phone", "Email", "City"]);
    expect(document.activeElement?.textContent, "the focus stays on it").toBe("Email");
    fireEvent.keyDown(screen.getByRole("button", { name: "Email" }), { key: "ArrowRight" });
    expect(headings(), "a plain arrow does nothing").toEqual(["Name", "Phone", "Email", "City"]);
  });

  it("puts new columns, not in the saved order, at the end", () => {
    window.localStorage.setItem("hiringplug.columnOrder.people", JSON.stringify(["city", "gone", "email"]));
    render(<Table />);
    expect(headings()).toEqual(["Name", "City", "Email", "Phone"]);
  });

  it("computes the order", () => {
    expect(moved(["a", "b", "c"], "c", "a", false)).toEqual(["c", "a", "b"]);
    expect(moved(["a", "b", "c"], "a", "b", true)).toEqual(["b", "a", "c"]);
    expect(moved(["a", "b", "c"], "a", "a", true)).toEqual(["a", "b", "c"]);
  });
});
