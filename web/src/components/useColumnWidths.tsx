// Column widths a person sets on a list, like a spreadsheet: drag a header's
// right edge to make the column wider or narrower, double-click it to fit the
// longest entry. Once one column is set, every column keeps the width it has
// (the table no longer shares out the space by itself). Kept on this device.
import { type CSSProperties, type ReactNode, type RefObject, useState } from "react";

const MIN = 48; // narrowest a column goes, in pixels
const KEY = "hiringplug.columnWidths.";

type Widths = Record<string, number>;

function read(table: string): Widths {
  try {
    const raw = window.localStorage.getItem(KEY + table);
    const parsed: unknown = raw ? JSON.parse(raw) : null;
    return parsed && typeof parsed === "object" ? (parsed as Widths) : {};
  } catch {
    return {};
  }
}

function keep(table: string, widths: Widths) {
  try {
    if (Object.keys(widths).length === 0) window.localStorage.removeItem(KEY + table);
    else window.localStorage.setItem(KEY + table, JSON.stringify(widths));
  } catch { /* only a convenience */ }
}

export interface ColumnWidths {
  /** Some width is set: the table lays out by them (column-widths.css). */
  set: boolean;
  /** A header's style (its width, once set). */
  style: (column: string) => CSSProperties | undefined;
  /**
   * What a header (the column's <th>) needs: its column, width, and its name
   * (just the label, not the drag edge's instructions).
   */
  head: (column: string, label: string) => { "data-column": string; style: CSSProperties | undefined; "aria-label": string };
  /** The table's style: as wide as its columns, once set. */
  tableStyle: CSSProperties | undefined;
  /** The drag edge, inside a header (which must be the column's <th>). */
  handle: (column: string, label: string) => ReactNode;
  reset: () => void;
}

/**
 * Widths for the columns of one list (`table` names it, e.g. "candidates").
 * columns are the keys of the columns shown, in order.
 */
export function useColumnWidths(table: string, columns: string[], ref: RefObject<HTMLTableElement | null>): ColumnWidths {
  const [widths, setWidths] = useState<Widths>(() => read(table));
  const set = columns.some((c) => widths[c] !== undefined);
  const save = (next: Widths) => { setWidths(next); keep(table, next); };

  /** The headers' widths as drawn now, for columns not set yet (so nothing jumps). */
  const drawn = (): Widths => {
    const out: Widths = { ...widths };
    ref.current?.querySelectorAll<HTMLTableCellElement>("thead th[data-column]").forEach((th) => {
      const c = th.dataset.column ?? "";
      if (out[c] === undefined) out[c] = Math.max(MIN, Math.round(th.getBoundingClientRect().width));
    });
    return out;
  };
  const resize = (column: string, width: number) => { save({ ...drawn(), [column]: Math.max(MIN, Math.round(width)) }); };

  /** The widest entry in a column, header included (as if nothing wrapped). */
  const fit = (column: string) => {
    const t = ref.current;
    const i = columns.indexOf(column);
    if (!t || i < 0) return;
    const probe = document.createElement("div");
    probe.style.cssText = "position:absolute;visibility:hidden;left:-9999px;top:0;white-space:nowrap;width:max-content";
    t.parentElement?.appendChild(probe);
    let widest = MIN;
    for (const row of t.querySelectorAll("tr")) {
      const cell = row.children[i] as HTMLElement | undefined;
      if (!cell || cell.getAttribute("colspan")) continue;
      const box = document.createElement("div");
      box.className = cell.className;
      box.style.cssText = `display:inline-block;font:${getComputedStyle(cell).font}`;
      for (const n of cell.childNodes) {
        if (!(n instanceof HTMLElement && n.classList.contains("col-resize"))) box.appendChild(n.cloneNode(true));
      }
      probe.appendChild(box);
      const pad = parseFloat(getComputedStyle(cell).paddingLeft) + parseFloat(getComputedStyle(cell).paddingRight);
      widest = Math.max(widest, box.getBoundingClientRect().width + pad + 14); // room for the sort arrow and edge
      probe.removeChild(box);
    }
    probe.remove();
    resize(column, widest);
  };

  const style = (column: string) => (widths[column] !== undefined && set ? { width: widths[column], minWidth: widths[column], maxWidth: widths[column] } : undefined);
  return {
    set,
    style,
    head: (column, label) => ({ "data-column": column, style: style(column), "aria-label": label }),
    tableStyle: set ? { width: columns.reduce((sum, c) => sum + (widths[c] ?? 120), 0) } : undefined,
    reset: () => { save({}); },
    handle: (column, label) => (
      <span role="separator" aria-orientation="vertical" tabIndex={0} className="col-resize"
        aria-label={`Width of ${label}: drag, or ← →; double-click to fit`} title="Drag to resize · double-click to fit"
        onClick={(e) => { e.stopPropagation(); }}
        onDoubleClick={(e) => { e.stopPropagation(); fit(column); }}
        onKeyDown={(e) => {
          if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
          e.preventDefault();
          const th = e.currentTarget.parentElement;
          resize(column, (widths[column] ?? th?.getBoundingClientRect().width ?? 120) + (e.key === "ArrowRight" ? 16 : -16));
        }}
        onPointerDown={(e) => {
          if (e.button !== 0) return;
          e.preventDefault();
          e.stopPropagation();
          const th = e.currentTarget.parentElement;
          const start = e.clientX;
          const from = widths[column] ?? th?.getBoundingClientRect().width ?? 120;
          const base = drawn();
          const handle = e.currentTarget;
          handle.setPointerCapture(e.pointerId);
          document.body.classList.add("resizing-columns");
          let width = Math.max(MIN, Math.round(from));
          const move = (ev: PointerEvent) => {
            width = Math.max(MIN, Math.round(from + ev.clientX - start));
            setWidths({ ...base, [column]: width });
          };
          // The drag ends when the button is let go, and also when the browser
          // takes the pointer away (a touch turned into a scroll, the window
          // lost focus): either way the width reached is kept.
          const end = (ev: PointerEvent) => {
            if (ev.type === "pointerup") width = Math.max(MIN, Math.round(from + ev.clientX - start));
            handle.removeEventListener("pointermove", move);
            for (const type of ["pointerup", "pointercancel", "lostpointercapture"] as const) handle.removeEventListener(type, end);
            document.body.classList.remove("resizing-columns");
            save({ ...base, [column]: width });
          };
          handle.addEventListener("pointermove", move);
          for (const type of ["pointerup", "pointercancel", "lostpointercapture"] as const) handle.addEventListener(type, end);
        }} />
    ),
  };
}
