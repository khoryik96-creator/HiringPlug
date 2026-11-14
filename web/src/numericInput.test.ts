import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { guardNumberFields, numericKind, numericText } from "./numericInput";

let stop = () => {};
beforeEach(() => { stop = guardNumberFields(); });
afterEach(() => { stop(); document.body.innerHTML = ""; });

function field(html: string): HTMLInputElement {
  document.body.innerHTML = html;
  const el = document.body.querySelector("input");
  if (!el) throw new Error("no input");
  return el;
}

/** Types (or pastes) text the way a browser does: beforeinput first, and only if it is not cancelled the text goes in. */
function insert(el: HTMLInputElement, text: string, inputType = "insertText") {
  const e = new InputEvent("beforeinput", { data: text, inputType, bubbles: true, cancelable: true });
  if (el.dispatchEvent(e)) el.value += text;
}

describe("number fields", () => {
  it("know whether they hold whole numbers or amounts", () => {
    expect(numericKind(field('<input type="number">'))).toBe("integer");
    expect(numericKind(field('<input type="number" step="0.01">'))).toBe("decimal");
    expect(numericKind(field('<input inputmode="numeric">'))).toBe("integer");
    expect(numericKind(field('<input inputmode="decimal">'))).toBe("decimal");
    expect(numericKind(field("<input>"))).toBeNull();
    expect(numericKind(field('<input type="date" inputmode="numeric">'))).toBeNull();
  });

  it("keep only what a number can contain", () => {
    expect(numericText("RM 12,000.50 / month", "decimal")).toBe("12,000.50");
    expect(numericText("12,000.5", "decimal", { numberInput: true })).toBe("12000.5");
    expect(numericText("90 days", "integer")).toBe("90");
    expect(numericText("-7e", "integer")).toBe("7");
    expect(numericText("-7", "integer", { negative: true })).toBe("-7");
  });

  it("ignore letters and symbols as they are typed", () => {
    const fee = field('<input inputmode="decimal">');
    for (const c of "18.5%abc") insert(fee, c);
    expect(fee.value).toBe("18.5");
    const days = field('<input type="number" min="0">');
    for (const c of "3e-0") insert(days, c);
    expect(days.value).toBe("30");
  });

  it("keep the digits of pasted text", () => {
    const salary = field('<input inputmode="decimal">');
    let input = 0;
    salary.addEventListener("input", () => { input++; });
    insert(salary, "RM 12,000", "insertFromPaste");
    expect(salary.value).toBe("12,000");
    expect(input).toBe(1);
  });

  it("paste into a number box only when it is empty, and only a number", () => {
    const days = field('<input type="number" min="0" value="90">');
    insert(days, "v1.2.3", "insertFromPaste");
    expect(days.value).toBe("90"); // kept, not emptied
    days.value = "";
    insert(days, "60 days", "insertFromPaste");
    expect(days.value).toBe("60");
  });

  it("leave other fields alone", () => {
    const name = field("<input>");
    for (const c of "Ana 2") insert(name, c);
    expect(name.value).toBe("Ana 2");
  });
});
