import { useEffect, useRef, useState } from "react";

/**
 * A small dropdown of actions. The list is placed against the window, so a
 * table or panel that scrolls or clips does not cut it off. Choosing an item
 * closes it (call the given close() from the item's handler).
 */
export function Menu({ label, ariaLabel, className, children }: {
  label: React.ReactNode; ariaLabel?: string; className?: string;
  children: (close: () => void) => React.ReactNode;
}) {
  const [at, setAt] = useState<{ top: number; right: number } | null>(null);
  const root = useRef<HTMLDetailsElement>(null);
  const close = () => { setAt(null); };
  const open = at !== null;
  useEffect(() => { // placed against the window: scrolling would leave it behind
    if (!open) return;
    const shut = () => { setAt(null); };
    const outside = (e: PointerEvent) => { if (!root.current?.contains(e.target as Node)) shut(); };
    window.addEventListener("scroll", shut, { capture: true, passive: true });
    window.addEventListener("resize", shut);
    document.addEventListener("pointerdown", outside);
    return () => {
      window.removeEventListener("scroll", shut, { capture: true });
      window.removeEventListener("resize", shut);
      document.removeEventListener("pointerdown", outside);
    };
  }, [open]);
  return (
    <details className="menu" ref={root} open={open} onToggle={(e) => {
      const d = e.currentTarget;
      if (!d.open) { setAt(null); return; }
      const r = d.getBoundingClientRect();
      // clientWidth: the viewport a fixed element is placed in (without the scrollbar)
      setAt({ top: r.bottom + 4, right: Math.max(8, document.documentElement.clientWidth - r.right) });
    }}>
      <summary className={className} aria-label={ariaLabel}>{label}</summary>
      <div className="menu-list" style={at ? { position: "fixed", top: at.top, right: at.right } : undefined}
        onKeyDown={(e) => {
          if (e.key !== "Escape") return;
          close();
          e.currentTarget.parentElement?.querySelector("summary")?.focus();
        }}>
        {children(close)}
      </div>
    </details>
  );
}
