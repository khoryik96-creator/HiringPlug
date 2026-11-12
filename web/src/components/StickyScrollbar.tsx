// A wide table's horizontal scrollbar, kept at the bottom of the window
// while the table is on screen (so it can be scrolled sideways without
// first scrolling down to the table's end). It moves with the table both
// ways and shows only when the table is wider than its box.
import { type ReactNode, type RefObject, useEffect, useRef, useState } from "react";

export function StickyScrollbar({ target }: { target: RefObject<HTMLDivElement | null> }) {
  const bar = useRef<HTMLDivElement>(null);
  const [widths, setWidths] = useState({ scroll: 0, client: 0 });
  useEffect(() => {
    const box = target.current;
    if (!box) return;
    const measure = () => { setWidths({ scroll: box.scrollWidth, client: box.clientWidth }); };
    measure();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    observer?.observe(box);
    if (box.firstElementChild) observer?.observe(box.firstElementChild);
    window.addEventListener("resize", measure);
    // Each follows the other; once they match, the echo changes nothing.
    const follow = (from: HTMLElement, to: HTMLElement) => () => {
      if (to.scrollLeft !== from.scrollLeft) to.scrollLeft = from.scrollLeft;
    };
    const strip = bar.current;
    const fromBox = strip ? follow(box, strip) : null;
    const fromBar = strip ? follow(strip, box) : null;
    if (fromBox) box.addEventListener("scroll", fromBox);
    if (strip && fromBar) strip.addEventListener("scroll", fromBar);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", measure);
      if (fromBox) box.removeEventListener("scroll", fromBox);
      if (strip && fromBar) strip.removeEventListener("scroll", fromBar);
    };
  }, [target]);
  const wide = widths.scroll > widths.client + 1;
  return (
    <div ref={bar} className="sticky-scrollbar" hidden={!wide} aria-hidden="true">
      <div style={{ width: widths.scroll }} />
    </div>
  );
}

/** A wide table in a box that scrolls sideways, with that scrollbar kept at the bottom of the window. */
export function PinnedScroll({ children }: { children: ReactNode }) {
  const box = useRef<HTMLDivElement>(null);
  return (
    <>
      <div className="table-scroll pinned-scroll" ref={box}>{children}</div>
      <StickyScrollbar target={box} />
    </>
  );
}
