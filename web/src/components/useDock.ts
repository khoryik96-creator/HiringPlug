// Where a side tool (the AI assistant) sits: in the page, docked to the
// right of the screen (the page makes room for it), or tucked away as a tab
// on the right edge. Remembered on this browser, with the dock's width.
import { useEffect, useState } from "react";

export type DockState = "inline" | "right" | "min";

const MIN_WIDTH = 320;
const MAX_WIDTH = 760;
export const clampWidth = (w: number) => Math.round(Math.min(MAX_WIDTH, Math.max(MIN_WIDTH, w)));

function read(key: string): string | null {
  try { return window.localStorage.getItem(key); } catch { return null; }
}
function write(key: string, value: string) {
  try { window.localStorage.setItem(key, value); } catch { /* only a convenience */ }
}

export function useDock(name: string) {
  const stateKey = `hiringplug.dock.${name}`;
  const widthKey = `hiringplug.dock.${name}.width`;
  const [state, setStateRaw] = useState<DockState>(() => {
    const v = read(stateKey);
    return v === "right" || v === "min" ? v : "inline";
  });
  const [width, setWidthRaw] = useState(() => clampWidth(Number(read(widthKey)) || 420));
  const setState = (s: DockState) => { setStateRaw(s); write(stateKey, s); };
  const setWidth = (w: number) => { const c = clampWidth(w); setWidthRaw(c); write(widthKey, String(c)); };

  // The page makes room for the dock (the topbar too), and gives it back when it goes.
  useEffect(() => {
    const body = document.body;
    body.classList.toggle("dock-right", state === "right");
    body.style.setProperty("--dock-w", `${String(width)}px`);
    return () => { body.classList.remove("dock-right"); };
  }, [state, width]);

  return { state, setState, width, setWidth };
}
