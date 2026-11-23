import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ExportButton } from "./ExportButton";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe("ExportButton", () => {
  it("offers Excel and CSV and runs the export in the chosen format", async () => {
    const run = vi.fn().mockResolvedValue(undefined);
    render(<ExportButton what="companies" run={run} />);
    fireEvent.click(screen.getByLabelText("Export companies"));
    fireEvent.click(screen.getByRole("menuitem", { name: "Excel (.xlsx)" }));
    await waitFor(() => { expect(run).toHaveBeenCalledWith("xlsx"); });
    fireEvent.click(screen.getByRole("menuitem", { name: "CSV" }));
    await waitFor(() => { expect(run).toHaveBeenCalledWith("csv"); });
  });

  it("says why an export failed (e.g. too many rows)", async () => {
    const run = vi.fn().mockRejectedValue(new Error("That finds more than 10000. Narrow the search and export again."));
    render(<ExportButton what="candidates" run={run} />);
    fireEvent.click(screen.getByRole("menuitem", { name: "CSV" }));
    expect(await screen.findByText(/Narrow the search/)).toBeTruthy();
  });
});
