// Rows chosen by clicking them (no tick boxes): a click on the row, or Space
// when it has the focus, chooses or unchooses it; links and buttons in the
// row keep doing their own thing (the name still opens the record).
import type React from "react";

export function pickRowProps(chosen: boolean, toggle: () => void, name: string) {
  const ownControl = (target: EventTarget) => (target as HTMLElement).closest("a, button, input, select, label, textarea") !== null;
  return {
    className: chosen ? "pick-row chosen" : "pick-row",
    tabIndex: 0,
    "aria-label": `${name}${chosen ? " (chosen)" : ""}`,
    "data-chosen": chosen ? "true" : undefined,
    // No focus on a mouse click: focusing a wide row would scroll the table sideways.
    onMouseDown: (e: React.MouseEvent) => { if (!ownControl(e.target)) e.preventDefault(); },
    onClick: (e: React.MouseEvent) => { if (!ownControl(e.target)) toggle(); },
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === " " && !ownControl(e.target)) { e.preventDefault(); toggle(); }
    },
  };
}
