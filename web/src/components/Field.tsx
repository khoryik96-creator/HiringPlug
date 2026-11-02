import { cloneElement, type ReactElement, useId } from "react";

type Control = ReactElement<{ id?: string; "aria-invalid"?: boolean; "aria-describedby"?: string }>;

/**
 * A label linked to its input (so screen readers and tests find the input by
 * its label), with the validation message underneath. The message is linked to
 * the input and marks it invalid, so screen readers read it out with the field.
 */
export function Field({ label, error, children }: { label: string; error?: string | undefined; children: Control }) {
  const id = useId();
  const errorId = `${id}-error`;
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {cloneElement(children, error ? { id, "aria-invalid": true, "aria-describedby": errorId } : { id })}
      {error && <small id={errorId} className="error">{error}</small>}
    </div>
  );
}
