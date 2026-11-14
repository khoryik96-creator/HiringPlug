// Saving with a field missing (or not valid: too short, out of range) does
// not go through: instead of a greyed-out button, every such field is
// outlined in red and the first one gets the focus. Fields say what they need
// with the usual attributes (required, min, minLength, pattern, ...) and the
// browser checks them when a form is submitted; buttons outside a form call
// filledIn(). The outline goes as soon as the field is filled.

import { useCallback } from "react";

const MARK = "data-missing";

type Control = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

function isControl(t: EventTarget | null): t is Control {
  return t instanceof HTMLInputElement || t instanceof HTMLSelectElement || t instanceof HTMLTextAreaElement;
}

function onInvalid(e: Event) {
  if (isControl(e.target)) e.target.setAttribute(MARK, "");
}

function onEdit(e: Event) {
  const t = e.target;
  if (!isControl(t)) return;
  // A group of checkboxes or radios is filled in when any one of them is
  // ticked (the page stops requiring the others when it next draws).
  const group = (t.type === "checkbox" || t.type === "radio") && t.name && t.form
    ? [...t.form.querySelectorAll<Control>(`[name="${CSS.escape(t.name)}"]`)] : [t];
  const ticked = group.some((c) => c instanceof HTMLInputElement && (c.type === "checkbox" || c.type === "radio") && c.checked);
  for (const c of group) if (ticked || c.validity.valid) c.removeAttribute(MARK);
}

/** Starts outlining missing fields in `doc`; returns how to stop. */
export function guardRequiredFields(doc: Document = document): () => void {
  doc.addEventListener("invalid", onInvalid, true);
  doc.addEventListener("input", onEdit, true);
  doc.addEventListener("change", onEdit, true);
  return () => {
    doc.removeEventListener("invalid", onInvalid, true);
    doc.removeEventListener("input", onEdit, true);
    doc.removeEventListener("change", onEdit, true);
  };
}

/**
 * Whether the field `el`, or every field inside it, is filled in; if not, outlines the missing
 * ones and focuses the first. For buttons that are not a form's submit button.
 */
export function filledIn(el: Element | null | undefined): boolean {
  if (!el) return true;
  const controls = isControl(el) ? [el] : [...el.querySelectorAll<Control>("input, select, textarea")];
  const missing = controls.filter((c) => !c.disabled && !c.checkValidity());
  missing[0]?.focus();
  return missing.length === 0;
}

/**
 * A file input that must have a file, held in state: put the returned ref on
 * the input, which is then missing (outlined) until `file` is set, and is
 * emptied when `file` is cleared (after a save, so the same file can be
 * chosen again).
 */
export function useRequiredFile(file: File | null): (el: HTMLInputElement | null) => void {
  return useCallback((el: HTMLInputElement | null) => {
    if (!el) return;
    el.setCustomValidity(file ? "" : "Choose a file.");
    if (!file) el.value = "";
  }, [file]);
}
