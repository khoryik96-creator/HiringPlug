// The Outlook editors' window: wide, and as wide as the person drags either
// side (kept in this browser); its content scrolls inside it.
import { type ReactNode, type RefObject, useEffect, useRef, useSyncExternalStore } from "react";

import { Splitter, useStoredWidth } from "../Splitter";

function watchWidth(change: () => void) {
  window.addEventListener("resize", change);
  return () => { window.removeEventListener("resize", change); };
}

export function OutlookDialog({ className, labelledBy, onCancel, dialogRef, children }: {
  className: string; labelledBy: string; onCancel: () => void; dialogRef?: RefObject<HTMLDialogElement | null>; children: ReactNode;
}) {
  const own = useRef<HTMLDialogElement>(null);
  const dialog = dialogRef ?? own;
  const screen = useSyncExternalStore(watchWidth, () => window.innerWidth);
  const max = Math.max(320, screen - 32);
  const min = Math.min(480, max);
  const [stored, setWidth] = useStoredWidth("hiringplug.outlook.editorWidth", 1000, 480, 2400);
  const width = Math.min(stored, max);
  useEffect(() => { if (dialog.current && !dialog.current.open) dialog.current.showModal(); }, [dialog]);
  return (
    <dialog ref={dialog} className={`panel outlook-dialog ${className}`} aria-labelledby={labelledBy} style={{ width: `${String(width)}px` }}
      onCancel={(event) => { event.preventDefault(); onCancel(); }}>
      <Splitter label="Editor width, left edge" className="outlook-dialog-edge start" edge="start" scale={2} value={width} min={min} max={max} onChange={setWidth} />
      <div className="outlook-dialog-body">{children}</div>
      <Splitter label="Editor width, right edge" className="outlook-dialog-edge end" scale={2} value={width} min={min} max={max} onChange={setWidth} />
    </dialog>
  );
}
