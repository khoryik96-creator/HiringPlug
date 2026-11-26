// The Outlook editors' window: wide, and as wide as the person drags either
// side (kept in this browser); its content scrolls inside it. Docked, it sits
// on the right below the top bar, as wide as dragged from its left edge, and
// the mail beside it stays usable (the choice is kept in this browser).
import { type ReactNode, type RefObject, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";

import { Splitter, useStoredWidth } from "../Splitter";

function watchWidth(change: () => void) {
  window.addEventListener("resize", change);
  return () => { window.removeEventListener("resize", change); };
}

const DOCK_KEY = "hiringplug.outlook.docked";

/** Whether the editors open docked on the right: [docked, change it]. */
export function useOutlookDock(): [boolean, (docked: boolean) => void] {
  const [docked, setDocked] = useState(() => {
    try { return localStorage.getItem(DOCK_KEY) === "1"; } catch { return false; }
  });
  return [docked, (next) => {
    setDocked(next);
    try { if (next) localStorage.setItem(DOCK_KEY, "1"); else localStorage.removeItem(DOCK_KEY); } catch { /* private window: this visit only */ }
  }];
}

export function OutlookDialog({ className, labelledBy, onCancel, dialogRef, docked = false, children }: {
  className: string; labelledBy: string; onCancel: () => void; dialogRef?: RefObject<HTMLDialogElement | null>;
  /** On the right, not over the page: the mail beside it can still be read and opened. */
  docked?: boolean;
  children: ReactNode;
}) {
  const own = useRef<HTMLDialogElement>(null);
  const dialog = dialogRef ?? own;
  const screen = useSyncExternalStore(watchWidth, () => window.innerWidth);
  const max = Math.max(320, screen - 32);
  const min = Math.min(480, max);
  const [stored, setWidth] = useStoredWidth("hiringplug.outlook.editorWidth", 1000, 480, 2400);
  const [dockedStored, setDockedWidth] = useStoredWidth("hiringplug.outlook.dockedWidth", 640, 420, 1600);
  const width = docked ? Math.min(dockedStored, Math.max(320, screen - 360)) : Math.min(stored, max);
  // Open before the content's own effects run, so the message editor can take the
  // focus (opening focuses the first control in the window). Docked, it is not
  // modal: what is beside it stays usable.
  useLayoutEffect(() => {
    const node = dialog.current;
    if (!node) return;
    if (node.open) node.close();
    if (docked) node.show(); else node.showModal();
  }, [dialog, docked]);
  // Docked below the top bar (whose height changes as the window narrows), and the page leaves room for it.
  const top = useSyncExternalStore(watchWidth, () => (docked ? Math.max(0, Math.round(document.querySelector(".topbar")?.getBoundingClientRect().bottom ?? 0)) : 0));
  useLayoutEffect(() => {
    if (!docked) return;
    const root = document.documentElement;
    root.style.setProperty("--outlook-docked", `${String(width)}px`);
    return () => { root.style.removeProperty("--outlook-docked"); };
  }, [docked, width]);
  return (
    <dialog ref={dialog} className={`panel outlook-dialog ${className}${docked ? " docked" : ""}`} aria-labelledby={labelledBy}
      style={docked ? { width: `${String(width)}px`, top: `${String(top)}px`, height: `calc(100dvh - ${String(top)}px)` } : { width: `${String(width)}px` }}
      onCancel={(event) => { event.preventDefault(); onCancel(); }}>
      <div className="outlook-dialog-body">{children}</div>
      {/* After the content: never the control that opening the window focuses. */}
      {docked
        ? <Splitter label="Editor width" className="outlook-dialog-edge start" edge="start" value={width} min={420} max={Math.max(420, screen - 360)} onChange={setDockedWidth} />
        : <>
          <Splitter label="Editor width, left edge" className="outlook-dialog-edge start" edge="start" scale={2} value={width} min={min} max={max} onChange={setWidth} />
          <Splitter label="Editor width, right edge" className="outlook-dialog-edge end" scale={2} value={width} min={min} max={max} onChange={setWidth} />
        </>}
    </dialog>
  );
}
