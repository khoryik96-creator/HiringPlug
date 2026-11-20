// A bar to drag (or move with the arrow keys) that resizes what is beside it;
// the width chosen is kept in this browser.
import { type KeyboardEvent, type PointerEvent, useState } from "react";

const clamp = (n: number, min: number, max: number) => Math.min(max, Math.max(min, n));

/** A width in pixels kept under key (localStorage), within min and max. */
export function useStoredWidth(key: string, initial: number, min: number, max: number): [number, (w: number) => void] {
  const [width, setWidth] = useState(() => {
    try {
      const kept = Number(window.localStorage.getItem(key));
      return kept ? clamp(kept, min, max) : initial;
    } catch {
      return initial;
    }
  });
  const set = (w: number) => {
    const next = Math.round(clamp(w, min, max));
    setWidth(next);
    try { window.localStorage.setItem(key, String(next)); } catch { /* only a convenience */ }
  };
  return [width, set];
}

/**
 * label names what it resizes. scale: how much the width grows per pixel
 * dragged (2 for something centred, which grows on both sides); edge "start"
 * for a bar on its left side (dragging left makes it wider).
 */
export function Splitter({ label, value, min, max, onChange, scale = 1, edge = "end", className = "" }: {
  label: string; value: number; min: number; max: number; onChange: (w: number) => void;
  scale?: number; edge?: "start" | "end"; className?: string;
}) {
  const [drag, setDrag] = useState<{ x: number; width: number } | null>(null);
  const sign = edge === "end" ? 1 : -1;
  const move = (e: PointerEvent<HTMLDivElement>) => {
    if (drag) onChange(drag.width + sign * scale * (e.clientX - drag.x));
  };
  const keys = (e: KeyboardEvent<HTMLDivElement>) => {
    const step = e.shiftKey ? 64 : 16;
    if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
      e.preventDefault();
      onChange(value + sign * (e.key === "ArrowRight" ? step : -step));
    } else if (e.key === "Home") { e.preventDefault(); onChange(min); } else if (e.key === "End") { e.preventDefault(); onChange(max); }
  };
  return (
    <div role="separator" aria-orientation="vertical" aria-label={label} aria-valuenow={value} aria-valuemin={min} aria-valuemax={max}
      tabIndex={0} title={`${label}: drag to resize`} className={`splitter${drag ? " dragging" : ""} ${className}`.trim()}
      onPointerDown={(e) => { e.preventDefault(); e.currentTarget.setPointerCapture(e.pointerId); setDrag({ x: e.clientX, width: value }); }}
      onPointerMove={move} onPointerUp={() => { setDrag(null); }} onPointerCancel={() => { setDrag(null); }} onKeyDown={keys} />
  );
}
