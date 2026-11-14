import { renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { filledIn, guardRequiredFields, useRequiredFile } from "./requiredFields";

let stop = () => {};
beforeEach(() => { stop = guardRequiredFields(); });
afterEach(() => { stop(); document.body.innerHTML = ""; });

const $ = (sel: string) => {
  const el = document.querySelector<HTMLInputElement>(sel);
  if (!el) throw new Error(`no ${sel}`);
  return el;
};

function type(el: HTMLInputElement, value: string) {
  el.value = value;
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

describe("missing fields", () => {
  it("stop a form being saved and are outlined, until they are filled in", () => {
    document.body.innerHTML = `<form><input id="start" type="date" required><input id="po"><input id="days" type="number" max="365"><button>Save</button></form>`;
    let saved = 0;
    $("form").addEventListener("submit", (e) => { e.preventDefault(); saved++; });
    $("#days").value = "400";
    $("button").click();
    expect(saved).toBe(0);
    expect($("#start").hasAttribute("data-missing")).toBe(true);
    expect($("#days").hasAttribute("data-missing")).toBe(true);
    expect($("#po").hasAttribute("data-missing")).toBe(false);
    type($("#start"), "2026-12-01");
    expect($("#start").hasAttribute("data-missing")).toBe(false);
    type($("#days"), "90");
    $("button").click();
    expect(saved).toBe(1);
  });

  it("in a group of checkboxes, are all filled in once one is ticked", () => {
    document.body.innerHTML = `<form><input type="checkbox" name="ev" value="a" required><input type="checkbox" name="ev" value="b" required></form>`;
    expect(filledIn($("form"))).toBe(false);
    const [a, b] = document.querySelectorAll<HTMLInputElement>("[name=ev]");
    expect(a?.hasAttribute("data-missing") && b?.hasAttribute("data-missing")).toBe(true);
    // The page then stops requiring the others (required={none ticked}).
    if (!a || !b) throw new Error("no boxes");
    a.checked = true;
    b.required = false;
    a.dispatchEvent(new Event("change", { bubbles: true }));
    expect(a.hasAttribute("data-missing") || b.hasAttribute("data-missing")).toBe(false);
  });

  it("in a group of checkboxes, are cleared on ticking even before the page stops requiring the others", () => {
    document.body.innerHTML = `<form><input type="checkbox" name="days" required><input type="checkbox" name="days" required></form>`;
    filledIn($("form"));
    const [mon, tue] = document.querySelectorAll<HTMLInputElement>("[name=days]");
    if (!mon || !tue) throw new Error("no boxes");
    mon.checked = true;
    mon.dispatchEvent(new Event("change", { bubbles: true })); // tue is still required here
    expect(tue.hasAttribute("data-missing")).toBe(false); // so unticking later does not outline them unasked
  });

  it("outside a form, are found by filledIn, which focuses the first", () => {
    document.body.innerHTML = `<div id="box"><input id="title" required><input id="notes"></div>`;
    expect(filledIn($("#box"))).toBe(false);
    expect(document.activeElement).toBe($("#title"));
    expect($("#title").hasAttribute("data-missing")).toBe(true);
    type($("#title"), "Intro call");
    expect(filledIn($("#box"))).toBe(true);
    expect(filledIn(null)).toBe(true);
  });
});

describe("a required file", () => {
  it("is missing until chosen, and emptied when cleared", () => {
    const { result, rerender } = renderHook(({ file }: { file: File | null }) => useRequiredFile(file), { initialProps: { file: null as File | null } });
    const input = document.createElement("input");
    input.type = "file";
    result.current(input);
    expect(input.validity.valid).toBe(false);
    rerender({ file: new File(["x"], "cv.docx") });
    result.current(input);
    expect(input.validity.valid).toBe(true);
  });
});
