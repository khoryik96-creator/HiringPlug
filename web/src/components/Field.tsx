import { cloneElement, type ReactElement, useId } from "react";

/**
 * A label linked to its input (so screen readers and tests find the input by
 * its label), with the validation message underneath.
 */
export function Field({ label, error, children }: { label: string; error?: string | undefined; children: ReactElement<{ id?: string }> }) {
  const id = useId();
  return (
    <div className="field">
      <label htmlFor={id}>{label}</label>
      {cloneElement(children, { id })}
      {error && <small className="error">{error}</small>}
    </div>
  );
}
