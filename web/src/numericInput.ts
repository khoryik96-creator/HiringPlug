// Fields that hold a number accept only what a number can contain: typing
// or pasting anything else does nothing (pasted text keeps its digits, so
// "RM 12,000" becomes "12,000"). One listener covers every such field in the
// app: an <input type="number">, or one with inputmode="numeric" (whole
// numbers) or inputmode="decimal" (amounts, percentages).

type Kind = "integer" | "decimal";

/** What kind of number a field holds, or null for any other field. */
export function numericKind(el: EventTarget | null): Kind | null {
  if (!(el instanceof HTMLInputElement)) return null;
  if (el.type === "number") return el.step === "" || /^\d+$/.test(el.step) ? "integer" : "decimal";
  if (el.type !== "text" && el.type !== "search" && el.type !== "tel") return null;
  if (el.inputMode === "numeric") return "integer";
  if (el.inputMode === "decimal") return "decimal";
  return null;
}

/**
 * The part of `text` such a field keeps: digits; for amounts also "." and
 * thousands commas (not in a number input, which takes neither); a minus
 * only where the field allows numbers below zero.
 */
export function numericText(text: string, kind: Kind, opts: { negative?: boolean; numberInput?: boolean } = {}): string {
  const allowed = kind === "integer" ? "0-9" : opts.numberInput ? "0-9." : "0-9.,";
  return text.replace(new RegExp(`[^${allowed}${opts.negative ? "-" : ""}]`, "g"), "");
}

/** Sets an input's value past React's own setter, so React sees the change (as when typed). */
function setValue(el: HTMLInputElement, value: string) {
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(el, value);
}

function onBeforeInput(e: Event) {
  if (!(e instanceof InputEvent) || !e.inputType.startsWith("insert")) return;
  const el = e.target;
  const kind = numericKind(el);
  if (!kind || !(el instanceof HTMLInputElement)) return;
  const text = e.data ?? e.dataTransfer?.getData("text/plain") ?? "";
  const kept = numericText(text, kind, { negative: el.min !== "" && Number(el.min) < 0, numberInput: el.type === "number" });
  if (kept === text) return;
  e.preventDefault();
  if (kept === "") return;
  // Insert what is kept as if typed, so React sees the change.
  if (el.type === "number") {
    // A number input cannot say where the cursor is: fill it only when empty, and only with a number.
    if (el.value !== "" || !Number.isFinite(Number(kept))) return;
    setValue(el, kept);
  }
  else {
    const start = el.selectionStart ?? el.value.length;
    const end = el.selectionEnd ?? start;
    setValue(el, el.value.slice(0, start) + kept + el.value.slice(end));
    el.setSelectionRange(start + kept.length, start + kept.length);
  }
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

/** Starts guarding number fields in `doc`; returns how to stop. */
export function guardNumberFields(doc: Document = document): () => void {
  doc.addEventListener("beforeinput", onBeforeInput, true);
  return () => { doc.removeEventListener("beforeinput", onBeforeInput, true); };
}
