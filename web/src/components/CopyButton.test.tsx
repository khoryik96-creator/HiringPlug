import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";

import { CopyButton } from "./CopyButton";

afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.useRealTimers(); });

it("copies the text in one click and says so for a moment", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
  vi.useFakeTimers();
  render(<CopyButton text="sy4zw4n@gmail.com" what="email" />);
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Copy email" })); await Promise.resolve(); });
  expect(writeText).toHaveBeenCalledWith("sy4zw4n@gmail.com");
  expect(screen.getByRole("button", { name: "email copied" }).textContent).toContain("Copied");
  act(() => { vi.advanceTimersByTime(1600); });
  expect(screen.getByRole("button", { name: "Copy email" })).toBeTruthy();
});

it("still copies where the browser has no clipboard API (the app opened over plain http)", async () => {
  Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
  const exec = vi.fn().mockReturnValue(true);
  Object.defineProperty(document, "execCommand", { value: exec, configurable: true });
  render(<CopyButton text="+60102215849" what="phone number" />);
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Copy phone number" })); await Promise.resolve(); });
  expect(exec).toHaveBeenCalledWith("copy");
  expect(screen.getByRole("button", { name: "phone number copied" })).toBeTruthy();
  expect(document.querySelector("textarea")).toBeNull(); // the helper is removed
});
