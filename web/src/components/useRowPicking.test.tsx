import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { useRowPicking } from "./useRowPicking";

afterEach(() => { cleanup(); });

const NAMES = ["Aina", "Ben", "Chris", "Dina", "Eve"];

function List() {
  const [chosen, setChosen] = useState<ReadonlySet<string>>(new Set());
  const set = (ids: string[], on: boolean) => {
    setChosen((cur) => { const next = new Set(cur); for (const id of ids) { if (on) next.add(id); else next.delete(id); } return next; });
  };
  const picking = useRowPicking({ ids: NAMES, chosen, set, clear: () => { setChosen(new Set()); } });
  return (
    <>
      <input aria-label="Search" />
      <button type="button">More ▾</button>
      <button type="button" onClick={() => { setChosen(new Set()); }}>Clear</button>
      <table {...picking.tableProps}><tbody>
        {NAMES.map((n) => <tr key={n} {...picking.rowProps(n)}><td><a href={`#/${n}`}>{n}</a></td><td>{n} details</td></tr>)}
      </tbody></table>
      <output aria-label="Chosen">{[...chosen].sort().join(",")}</output>
    </>
  );
}
const chosen = () => screen.getByLabelText("Chosen").textContent;
const cell = (n: string) => screen.getByText(`${n} details`);

describe("choosing rows with the usual shortcuts", () => {
  it("chooses a range with Shift+click, and one more or less with Ctrl/⌘+click", () => {
    render(<List />);
    fireEvent.click(cell("Ben"));
    fireEvent.click(cell("Dina"), { shiftKey: true });
    expect(chosen(), "Ben to Dina").toBe("Ben,Chris,Dina");
    fireEvent.click(cell("Chris"), { ctrlKey: true });
    expect(chosen(), "Chris left out").toBe("Ben,Dina");
    fireEvent.click(cell("Eve"), { metaKey: true });
    expect(chosen()).toBe("Ben,Dina,Eve");
    fireEvent.click(cell("Aina"), { shiftKey: true });
    expect(chosen(), "from Eve (chosen) up to Aina").toBe("Aina,Ben,Chris,Dina,Eve");
    fireEvent.click(cell("Chris"));
    fireEvent.click(cell("Aina"), { shiftKey: true });
    expect(chosen(), "Chris was unchosen, so the range is unchosen").toBe("Dina,Eve");
  });

  it("chooses all with Ctrl/⌘+A in the list, and none with Esc, but not while typing", () => {
    render(<List />);
    fireEvent.keyDown(screen.getByRole("link", { name: "Ben" }), { key: "a", ctrlKey: true });
    expect(chosen()).toBe("Aina,Ben,Chris,Dina,Eve");
    fireEvent.keyDown(screen.getByLabelText("Search"), { key: "Escape" });
    expect(chosen(), "Esc in the search box is the search box's").toBe("Aina,Ben,Chris,Dina,Eve");
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(chosen()).toBe("");
  });

  it("moves with ↑↓, chooses while moving with Shift, and toggles with Space", () => {
    render(<List />);
    const ben = screen.getByRole("link", { name: "Ben" });
    ben.focus();
    fireEvent.keyDown(ben, { key: "ArrowDown" });
    expect(document.activeElement).toBe(screen.getByRole("link", { name: "Chris" }));
    expect(chosen(), "moving alone chooses nothing").toBe("");
    fireEvent.keyDown(document.activeElement as Element, { key: "ArrowDown", shiftKey: true });
    expect(chosen()).toBe("Chris,Dina");
    fireEvent.keyDown(document.activeElement as Element, { key: " " });
    expect(chosen(), "Space on Dina unchooses her").toBe("Chris");
  });

  it("starts a Shift+click afresh after the rows were cleared some other way", () => {
    render(<List />);
    fireEvent.click(cell("Ben"));
    fireEvent.click(screen.getByRole("button", { name: "Clear" }));
    fireEvent.click(cell("Dina"), { shiftKey: true });
    expect(chosen(), "chooses Dina, not 'unchoose Ben to Dina'").toBe("Dina");
  });

  it("keeps the rows when Esc only closes a menu elsewhere", () => {
    render(<List />);
    fireEvent.click(cell("Ben"));
    fireEvent.keyDown(screen.getByRole("button", { name: "More ▾" }), { key: "Escape" });
    expect(chosen()).toBe("Ben");
    fireEvent.keyDown(screen.getByRole("link", { name: "Ben" }), { key: "Escape" });
    expect(chosen(), "Esc from the list clears").toBe("");
  });

  it("leaves the name link and copied text alone", () => {
    render(<List />);
    fireEvent.click(screen.getByRole("link", { name: "Ben" }));
    expect(chosen()).toBe("");
  });
});
