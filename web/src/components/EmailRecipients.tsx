import { useId, useState } from "react";

type Props = {
  to: string;
  cc: string;
  bcc: string;
  errors: Record<string, string>;
  onChange: (field: "to" | "cc" | "bcc", value: string) => void;
};

/** Keep optional recipient rows out of the way until needed. */
export function EmailRecipients({ to, cc, bcc, errors, onChange }: Props) {
  const id = useId();
  const [opened, setOpened] = useState({ cc: false, bcc: false });
  const showCc = opened.cc || !!cc || !!errors.cc;
  const showBcc = opened.bcc || !!bcc || !!errors.bcc;
  const copy = (field: "cc" | "bcc", label: string, value: string) => (
    <div className="field">
      <label htmlFor={`${id}-${field}`}>{label}</label>
      <input id={`${id}-${field}`} autoFocus={opened[field]} value={value} aria-invalid={!!errors[field] || undefined}
        aria-describedby={errors[field] ? `${id}-${field}-error` : undefined}
        onChange={(event) => { onChange(field, event.target.value); }} />
      {errors[field] && <small id={`${id}-${field}-error`} className="error">{errors[field]}</small>}
    </div>
  );
  return <div className="email-recipients full">
    <div className="field">
      <div className="row spread">
        <label htmlFor={`${id}-to`}>To</label>
        {(!showCc || !showBcc) && <div className="row">
          {!showCc && <button type="button" className="link small" onClick={() => { setOpened((current) => ({ ...current, cc: true })); }}>Cc</button>}
          {!showBcc && <button type="button" className="link small" onClick={() => { setOpened((current) => ({ ...current, bcc: true })); }}>Bcc</button>}
        </div>}
      </div>
      <input id={`${id}-to`} required value={to} aria-invalid={!!errors.to || undefined}
        aria-describedby={errors.to ? `${id}-to-error` : undefined}
        onChange={(event) => { onChange("to", event.target.value); }} />
      {errors.to && <small id={`${id}-to-error`} className="error">{errors.to}</small>}
    </div>
    {(showCc || showBcc) && <div className="two">
      {showCc && copy("cc", "Cc", cc)}
      {showBcc && copy("bcc", "Bcc", bcc)}
    </div>}
  </div>;
}
