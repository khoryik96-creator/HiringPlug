import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as companion from "../companion";
import { renderWithClient } from "../test-utils";
import { LinkedinPageFinder } from "./LinkedinPageFinder";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("finding a company's LinkedIn page with the companion", () => {
  it("stops a search still running when the box goes away, so the companion is free again", async () => {
    vi.spyOn(companion, "useCompanion").mockReturnValue({ ready: true, version: "1.6.0" });
    vi.spyOn(companion, "operationId").mockReturnValueOnce("op_1").mockReturnValueOnce("op_2");
    const call = vi.spyOn(companion, "companionCall").mockImplementation(((action: string) =>
      action === "cancel" ? Promise.resolve({}) : new Promise(() => undefined)));
    const { rerender } = renderWithClient(<LinkedinPageFinder name="Nusantara Logistik" value="" onChange={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Find on LinkedIn" }));
    await waitFor(() => { expect(call).toHaveBeenCalledWith("companySearch", { operation_id: "op_1", name: "Nusantara Logistik" }); });
    expect(call.mock.calls.some((c) => c[0] === "cancel")).toBe(false);

    rerender(<p>Panel closed</p>);
    await waitFor(() => { expect(call).toHaveBeenCalledWith("cancel", { operation_id: "op_1" }, 5_000); });
    expect(call.mock.calls.filter((c) => c[0] === "cancel")).toHaveLength(1);
  });
});
