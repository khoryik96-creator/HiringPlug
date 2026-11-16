import { cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { api } from "../api/client";
import { ok, renderWithClient, testFieldSettings } from "../test-utils";
import { ContactTable } from "./ContactTable";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe("a contacts table's Esc", () => {
  it("clears every chosen contact, also those chosen under an earlier search", () => {
    vi.spyOn(api, "GET").mockResolvedValue(ok(testFieldSettings()));
    const onPick = vi.fn();
    const onClearPicked = vi.fn();
    renderWithClient(<ContactTable contacts={[]} empty="None" picked={new Set(["cont_not_shown"])} onPick={onPick} onClearPicked={onClearPicked} />);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(onClearPicked).toHaveBeenCalled();
    expect(onPick).not.toHaveBeenCalled();
  });
});
