import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { Splitter, useStoredWidth } from "./Splitter";

afterEach(() => { cleanup(); window.localStorage.clear(); });

function Pane() {
  const [width, setWidth] = useStoredWidth("test.width", 300, 200, 500);
  return <><output>{width}</output><Splitter label="List width" value={width} min={200} max={500} onChange={setWidth} /></>;
}

it("resizes by dragging or with the arrow keys, within its limits, and keeps the width", () => {
  render(<Pane />);
  const bar = screen.getByRole("separator", { name: "List width" });
  fireEvent.keyDown(bar, { key: "ArrowRight" });
  expect(screen.getByRole("status").textContent).toBe("316");
  fireEvent.keyDown(bar, { key: "End" });
  expect(screen.getByRole("status").textContent).toBe("500");
  bar.setPointerCapture = () => undefined;
  fireEvent.pointerDown(bar, { clientX: 500, pointerId: 1 });
  fireEvent.pointerMove(bar, { clientX: 380, pointerId: 1 });
  fireEvent.pointerUp(bar, { pointerId: 1 });
  expect(screen.getByRole("status").textContent).toBe("380");
  fireEvent.pointerMove(bar, { clientX: 100, pointerId: 1 });
  expect(screen.getByRole("status").textContent, "no longer dragged").toBe("380");
  expect(window.localStorage.getItem("test.width")).toBe("380");
  cleanup();
  render(<Pane />);
  expect(screen.getByRole("status").textContent, "kept for next time").toBe("380");
});
