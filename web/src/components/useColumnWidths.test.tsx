import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { useColumnWidths } from "./useColumnWidths";

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});

function Table({ name = "people" }: { name?: string }) {
  const ref = useRef<HTMLTableElement>(null);
  const w = useColumnWidths(name, ["name", "email"], ref);
  return (
    <>
      {w.set && <button type="button" onClick={w.reset}>Reset column widths</button>}
      <table ref={ref} className={w.set ? "sized" : undefined} style={w.tableStyle}>
        <thead><tr>
          <th data-column="name" style={w.style("name")}>Name{w.handle("name", "Name")}</th>
          <th data-column="email" style={w.style("email")}>Email{w.handle("email", "Email")}</th>
        </tr></thead>
        <tbody><tr><td>Nurul Aisyah</td><td>nurul@example.com</td></tr></tbody>
      </table>
    </>
  );
}

const header = (name: string) => screen.getByRole("columnheader", { name: new RegExp(`^${name}`) });

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
});
