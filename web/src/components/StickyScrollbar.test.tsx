import { cleanup, fireEvent, render } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, expect, it } from "vitest";

import { StickyScrollbar } from "./StickyScrollbar";

afterEach(() => { cleanup(); });

function Wide() {
  const box = useRef<HTMLDivElement>(null);
  return <><div data-testid="box" ref={box}><table><tbody><tr><td>wide</td></tr></tbody></table></div><StickyScrollbar target={box} /></>;
}

it("shows for a table wider than its box, and scrolls with it both ways", () => {
  // jsdom has no layout: give the box the widths a browser would.
  Object.defineProperty(HTMLElement.prototype, "scrollWidth", { configurable: true, get() { return (this as HTMLElement).dataset.testid === "box" ? 2000 : 0; } });
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return (this as HTMLElement).dataset.testid === "box" ? 800 : 0; } });
  const { getByTestId, container } = render(<Wide />);
  const bar = container.querySelector<HTMLElement>(".sticky-scrollbar");
  if (!bar) throw new Error("no scrollbar");
  expect(bar.hidden).toBe(false);
  expect((bar.firstElementChild as HTMLElement).style.width).toBe("2000px");
  bar.scrollLeft = 300;
  fireEvent.scroll(bar);
  expect(getByTestId("box").scrollLeft).toBe(300);
  const box = getByTestId("box");
  box.scrollLeft = 120;
  fireEvent.scroll(box);
  expect(bar.scrollLeft).toBe(120);
  Reflect.deleteProperty(HTMLElement.prototype, "scrollWidth");
  Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
});
