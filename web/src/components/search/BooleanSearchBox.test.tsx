import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { BooleanSearchBox, booleanParts, looksBoolean } from "./BooleanSearchBox";

afterEach(() => { cleanup(); });

describe("the boolean search box", () => {
  it("knows a boolean search from a name, email or phone search", () => {
    for (const q of ['SAP AND (BW OR BPC) NOT intern', '"project manager"', "manag*", "finance -intern", "sap NEAR/3 bw", "java W/2 spring", "a && b", "(sap)"]) {
      expect(looksBoolean(q), q).toBe(true);
    }
    for (const q of ["Tan - Maju Foods", "Johnson and Johnson", "+60 12 345 6789", "siti@example.com", "Ali", "near the office", "SAP-BW", "-5"]) {
      expect(looksBoolean(q), q).toBe(false);
    }
  });

  it("marks the rules, not the words looked for", () => {
    const rules = (q: string) => booleanParts(q).filter((p) => p.rule).map((p) => p.text);
    expect(rules('SAP AND (BW OR "planning") NOT intern* -sales NEAR/2 x')).toEqual(["AND", "(", "OR", '"planning"', ")", "NOT", "*", "-", "NEAR/2"]);
    expect(booleanParts("manag* AND x").map((p) => p.text).join(""), "nothing lost").toBe("manag* AND x");
  });

  it("shows the rules in blue under the typed text, with a Boolean badge", () => {
    const { rerender } = render(<BooleanSearchBox value="Siti" onChange={() => undefined} label="Search candidates" placeholder="Search" />);
    expect(document.querySelector(".bool-mirror"), "a plain search is shown as typed").toBeNull();
    expect(screen.queryByText("Boolean")).toBeNull();
    rerender(<BooleanSearchBox value="SAP AND BW" onChange={() => undefined} label="Search candidates" placeholder="Search" />);
    expect([...document.querySelectorAll(".bool-rule")].map((e) => e.textContent)).toEqual(["AND"]);
    expect(document.querySelector(".bool-mirror")?.textContent).toBe("SAP AND BW");
    expect(screen.getByLabelText("Search candidates").getAttribute("aria-describedby"), "says it is read as a boolean").toBe(screen.getByText("Boolean").id);
  });
});

describe("a boolean search that cannot be read", () => {
  it("says what is wrong with it, not just that something is", async () => {
    const { ApiError } = await import("../../api/client");
    const err = new ApiError({ error: { code: "VALIDATION_FAILED", message: "Some fields are invalid.", requestId: "r", details: { fields: [{ field: "boolean", message: "has a ( without its )" }] } } });
    const { ListSearchError } = await import("./BooleanSearchBox");
    render(<ListSearchError error={err} />);
    expect(screen.getByRole("alert").textContent).toBe("This boolean search has a ( without its ).");
  });
});
