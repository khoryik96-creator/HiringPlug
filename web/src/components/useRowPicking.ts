// Choosing rows of a list (no tick boxes), with the usual shortcuts:
//   click (or Ctrl/⌘+click): choose or unchoose that row;
//   Shift+click: every row from the last one clicked to this one;
//   Ctrl/⌘+A in the list: every row shown; Esc: none;
//   ↑/↓ on a row: the row above or below (Shift: choosing it too);
//   Space on a row: choose or unchoose it.
// A click that ends a text selection (copying an email) chooses nothing, and
// links and buttons keep doing their own thing (the name opens the record).
import type React from "react";
import { useEffect, useRef } from "react";

const ownControl = (target: EventTarget) => (target as HTMLElement).closest("a, button, input, select, label, textarea") !== null;
const typing = (target: EventTarget | null) => !!target && (target as HTMLElement).closest("input, textarea, select, [contenteditable=true]") !== null;
const selectingText = (row: Element) => {
  const s = window.getSelection();
  return !!s && !s.isCollapsed && s.toString().trim() !== "" && !!s.anchorNode && row.contains(s.anchorNode);
};

/** The shortcuts, for a tooltip. */
export const PICK_SHORTCUTS = "Shift+click: a range · Ctrl/⌘+click: one more or less · Ctrl/⌘+A: all shown · Esc: none · ↑↓: move, Shift+↑↓: choose while moving";

export function useRowPicking({ ids, chosen, set, clear, enabled = true }: {
  /** The rows shown, in order. */
  ids: string[];
  chosen: ReadonlySet<string>;
  set: (ids: string[], on: boolean) => void;
  clear: () => void;
  enabled?: boolean;
}) {
  const anchor = useRef<string | null>(null); // the last row clicked: where Shift+click ranges from
  // Cleared by any means (Clear, another folder…): Shift+click starts afresh.
  useEffect(() => { if (chosen.size === 0) anchor.current = null; }, [chosen.size]);
  // Esc clears the chosen rows, when it is meant for the list: from a row,
  // the bar over it, or nowhere in particular (not while typing, nor when it
  // closes a menu or a dialog).
  useEffect(() => {
    if (!enabled || chosen.size === 0) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented || typing(e.target) || document.querySelector("dialog[open]")) return;
      const at = e.target as HTMLElement | null;
      const forList = !at || at === document.body || at === document.documentElement || at.closest("tr[data-pick-id], .pick-bar, table") !== null;
      if (!forList) return;
      clear();
      anchor.current = null;
    };
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("keydown", onKey); };
  }, [enabled, chosen.size, clear]);

  const range = (from: string, to: string) => {
    const a = ids.indexOf(from);
    const b = ids.indexOf(to);
    if (a < 0 || b < 0) return [to];
    return ids.slice(Math.min(a, b), Math.max(a, b) + 1);
  };
  const focusRow = (from: HTMLElement, step: 1 | -1) => {
    const row = from.closest("tr");
    let next = row ? (step > 0 ? row.nextElementSibling : row.previousElementSibling) : null;
    while (next && !(next as HTMLElement).dataset.pickId) next = step > 0 ? next.nextElementSibling : next.previousElementSibling;
    const target = next?.querySelector<HTMLElement>("a, [tabindex]") ?? (next as HTMLElement | null);
    target?.focus();
    return (next as HTMLElement | null)?.dataset.pickId ?? null;
  };

  return {
    /** Props for the list's table: Ctrl/⌘+A chooses every row shown. */
    tableProps: enabled ? {
      onKeyDown: (e: React.KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a" && !typing(e.target)) {
          e.preventDefault();
          set(ids, true);
        }
      },
    } : {},
    /** Props for one row. */
    rowProps: (id: string) => {
      if (!enabled) return {};
      const on = chosen.has(id);
      return {
        className: on ? "pick-row chosen" : "pick-row",
        "data-chosen": on ? "true" : undefined,
        "data-pick-id": id,
        // Shift+click would select the page's text: not on a row.
        onMouseDown: (e: React.MouseEvent) => { if (e.shiftKey && !ownControl(e.target)) e.preventDefault(); },
        onClick: (e: React.MouseEvent) => {
          if (ownControl(e.target) || selectingText(e.currentTarget)) return;
          if (e.shiftKey && anchor.current && anchor.current !== id) {
            set(range(anchor.current, id), chosen.has(anchor.current)); // the range takes the anchor's state
            return;
          }
          set([id], !on);
          anchor.current = id;
        },
        onKeyDown: (e: React.KeyboardEvent) => {
          const t = e.target as HTMLElement;
          if (typing(t)) return;
          if (e.key === " " && t.closest("a, tr")) {
            e.preventDefault();
            set([id], !on);
            anchor.current = id;
          } else if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            const next = focusRow(t, e.key === "ArrowDown" ? 1 : -1);
            if (e.shiftKey && next) {
              if (!chosen.has(id)) set([id], true);
              set([next], true);
              anchor.current = next;
            }
          }
        },
      };
    },
  };
}
