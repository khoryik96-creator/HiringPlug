import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PageErrorBoundary } from "./PageErrorBoundary";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function Broken(): never {
  throw new Error("Cannot read properties of null (reading 'redacted')");
}

describe("page error boundary", () => {
  it("shows what went wrong instead of a blank page", () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    render(<PageErrorBoundary><Broken /></PageErrorBoundary>);
    expect(screen.getByRole("alert").textContent).toContain("This page could not be shown");
    expect(screen.getByText(/reading 'redacted'/)).toBeTruthy();
  });

  it("shows the page when nothing goes wrong", () => {
    render(<PageErrorBoundary><p>Fine</p></PageErrorBoundary>);
    expect(screen.getByText("Fine")).toBeTruthy();
  });
});
