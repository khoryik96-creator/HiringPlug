// A panel opened from an icon in the top bar (the bell, CV imports): while
// open its icon is marked as the one clicked; a click elsewhere closes it,
// and so does leaving it open for 30 seconds.
import { type RefObject, type SyntheticEvent, useEffect, useState } from "react";

/** How long a panel opened from the top bar stays open by itself. */
export const AUTO_CLOSE_MS = 30_000;

export function useTopbarPanel(panel: RefObject<HTMLDetailsElement | null>) {
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (!open) return;
    const shut = () => { if (panel.current) panel.current.open = false; };
    const timer = window.setTimeout(shut, AUTO_CLOSE_MS);
    const outside = (e: MouseEvent) => { if (panel.current && !panel.current.contains(e.target as Node)) shut(); };
    document.addEventListener("click", outside);
    return () => { window.clearTimeout(timer); document.removeEventListener("click", outside); };
  }, [open, panel]);
  return { open, track: (e: SyntheticEvent<HTMLDetailsElement>) => { setOpen(e.currentTarget.open); } };
}
