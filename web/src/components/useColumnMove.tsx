// Columns a person puts in their own order, like a spreadsheet: drag a
// column's heading left or right and let go where it should be. With the
// heading focused, Alt+Shift+← or → moves it one place. A short click still
// sorts; only a press that moves becomes a drag.
import { type KeyboardEvent, type PointerEvent as ReactPointerEvent, type RefObject, useState } from "react";

const KEY = "hiringplug.columnOrder.";
const DRAG_FROM = 6; // pixels a press moves before it is a drag

/** The order once column is put before (or after) target. */
export function moved<T extends string>(order: T[], column: T, target: T, after: boolean): T[] {
  if (column === target || !order.includes(column)) return order;
  const rest = order.filter((c) => c !== column);
  const at = rest.indexOf(target);
  if (at < 0) return order;
  rest.splice(at + (after ? 1 : 0), 0, column);
  return rest;
}

const same = (a: readonly string[], b: readonly string[]) => a.length === b.length && a.every((c, i) => c === b[i]);

export interface ColumnMove<T extends string = string> {
  /** What a movable heading needs, spread on its <th>. */
  props: (column: T) => {
    className: string;
    title: string;
    onPointerDown: (e: ReactPointerEvent<HTMLTableCellElement>) => void;
    onKeyDown: (e: KeyboardEvent<HTMLTableCellElement>) => void;
  };
}

/**
 * Moving the columns in order (the movable ones, in the order shown) of the
 * table in ref; onMove gets the new order.
 */
export function useColumnMove<T extends string>(ref: RefObject<HTMLTableElement | null>, order: T[], onMove: (next: T[]) => void): ColumnMove<T> {
  const [drag, setDrag] = useState<{ column: T; target: T | null; after: boolean } | null>(null);

  const heads = () => [...(ref.current?.querySelectorAll<HTMLTableCellElement>("thead th[data-column]") ?? [])]
    .filter((th) => order.includes((th.dataset.column ?? "") as T));

  /** The heading the pointer is over (the first or last when beyond them), and which half. */
  const spot = (x: number): { target: T; after: boolean } | null => {
    const hs = heads();
    const col = (th: HTMLTableCellElement) => (th.dataset.column ?? "") as T;
    for (const th of hs) {
      const r = th.getBoundingClientRect();
      if (x >= r.left && x < r.right) return { target: col(th), after: x >= r.left + r.width / 2 };
    }
    const first = hs[0];
    const last = hs[hs.length - 1];
    if (!first || !last) return null;
    if (x < first.getBoundingClientRect().left) return { target: col(first), after: false };
    return { target: col(last), after: true };
  };

  const start = (column: T, e: ReactPointerEvent<HTMLTableCellElement>) => {
    if (e.button !== 0 || (e.target as Element).closest(".col-resize")) return;
    const from = e.clientX;
    const was = order;
    let dragging = false;
    let at: { target: T; after: boolean } | null = null;
    const move = (ev: PointerEvent) => {
      if (!dragging && Math.abs(ev.clientX - from) < DRAG_FROM) return;
      if (!dragging) {
        dragging = true;
        document.body.classList.add("moving-column");
        window.getSelection()?.removeAllRanges();
      }
      at = spot(ev.clientX);
      setDrag({ column, target: at?.target ?? null, after: at?.after ?? false });
    };
    const end = (ev: PointerEvent) => {
      document.removeEventListener("pointermove", move);
      document.removeEventListener("pointerup", end);
      document.removeEventListener("pointercancel", end);
      if (!dragging) return;
      document.body.classList.remove("moving-column");
      setDrag(null);
      // Letting go ends the drag: that click does not also sort the list.
      const swallow = (c: MouseEvent) => { c.stopPropagation(); c.preventDefault(); };
      window.addEventListener("click", swallow, { capture: true, once: true });
      window.setTimeout(() => { window.removeEventListener("click", swallow, { capture: true }); }, 0);
      if (ev.type !== "pointerup" || !at) return;
      const next = moved(was, column, at.target, at.after);
      if (!same(next, was)) onMove(next);
    };
    document.addEventListener("pointermove", move);
    document.addEventListener("pointerup", end);
    document.addEventListener("pointercancel", end);
  };

  const byKeys = (column: T, e: KeyboardEvent<HTMLTableCellElement>) => {
    if (!e.altKey || !e.shiftKey || (e.key !== "ArrowLeft" && e.key !== "ArrowRight")) return;
    e.preventDefault();
    const i = order.indexOf(column);
    const j = i + (e.key === "ArrowRight" ? 1 : -1);
    const target = order[j];
    if (i < 0 || target === undefined) return;
    onMove(moved(order, column, target, j > i));
    // Keep the focus on the heading moved, in its new place.
    const table = ref.current;
    window.requestAnimationFrame(() => {
      table?.querySelector<HTMLElement>(`thead th[data-column="${column}"] :is(button, [tabindex])`)?.focus();
    });
  };

  return {
    props: (column) => {
      const cls = ["movable"];
      if (drag?.column === column) cls.push("moving");
      if (drag && drag.target === column && drag.column !== column) cls.push(drag.after ? "drop-after" : "drop-before");
      return {
        className: cls.join(" "),
        title: "Drag to move this column (or Alt+Shift+← →)",
        onPointerDown: (e) => { start(column, e); },
        onKeyDown: (e) => { byKeys(column, e); },
      };
    },
  };
}

function read(table: string): string[] {
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(KEY + table) ?? "null");
    return Array.isArray(parsed) ? parsed.filter((c): c is string => typeof c === "string") : [];
  } catch {
    return [];
  }
}

/**
 * The order of a list's movable columns, kept on this device (`table` names
 * the list). Columns not in the saved order (new ones) go at the end.
 */
export function useStoredOrder<T extends string>(table: string, columns: T[]) {
  const [saved, setSaved] = useState<string[]>(() => read(table));
  const kept = saved.filter((c): c is T => columns.includes(c as T));
  const order = [...kept, ...columns.filter((c) => !kept.includes(c))];
  const keep = (next: T[] | null) => {
    setSaved(next ?? []);
    try {
      if (next) window.localStorage.setItem(KEY + table, JSON.stringify(next));
      else window.localStorage.removeItem(KEY + table);
    } catch { /* only a convenience */ }
  };
  return {
    order,
    /** Not the order the list comes in. */
    changed: !same(order, columns),
    save: (next: T[]) => { keep(next); },
    reset: () => { keep(null); },
  };
}
