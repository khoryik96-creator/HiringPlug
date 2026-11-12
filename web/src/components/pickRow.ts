// Rows chosen by clicking them (no tick boxes): a click on the row chooses or
// unchooses it, unless it ends a text selection (copying an email or phone).
// Links and buttons keep doing their own thing: the name still opens the
// record, and Space on the focused name chooses the row (one tab stop per row).
import type React from "react";

const ownControl = (target: EventTarget) => (target as HTMLElement).closest("a, button, input, select, label, textarea") !== null;
const selectingText = (row: Element) => {
  const s = window.getSelection();
  return !!s && !s.isCollapsed && s.toString().trim() !== "" && !!s.anchorNode && row.contains(s.anchorNode);
};

export function pickRowProps(chosen: boolean, toggle: () => void) {
  return {
    className: chosen ? "pick-row chosen" : "pick-row",
    "data-chosen": chosen ? "true" : undefined,
    onClick: (e: React.MouseEvent) => { if (!ownControl(e.target) && !selectingText(e.currentTarget)) toggle(); },
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === " " && (e.target as HTMLElement).closest("a")) { e.preventDefault(); toggle(); }
    },
  };
}
