// "Insert field" in the WhatsApp window: the person's own value (their first
// name, the job's title...) goes in at the cursor, as plain text, since the
// message is sent as typed. Fields they have no value for cannot be picked.
import { useRef, useState } from "react";

import type { Schemas } from "../api/client";

export function WhatsAppInsertField({ fields, disabled, onInsert }: {
  fields: Schemas["WhatsAppFieldValue"][]; disabled?: boolean; onInsert: (value: string) => void;
}) {
  // A choice made with the mouse (or in the open list) goes in at once. The
  // arrow and letter keys on the closed list change it straight away in some
  // browsers, so a field reached that way only goes in on Enter.
  const [browsed, setBrowsed] = useState("");
  const byKey = useRef(false);
  const put = (key: string) => {
    setBrowsed("");
    const field = fields.find((f) => f.key === key);
    if (field?.value) onInsert(field.value);
  };
  return (
    <select aria-label="Insert field" title="Choose a field; with the keyboard, press Enter to put it in" value={browsed}
      disabled={disabled || fields.length === 0}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          if (browsed) { e.preventDefault(); put(browsed); }
          return;
        }
        // A change in this same turn comes from the key itself.
        byKey.current = true;
        setTimeout(() => { byKey.current = false; }, 0);
      }}
      onChange={(e) => { if (byKey.current) setBrowsed(e.target.value); else put(e.target.value); }}
      onBlur={() => { setBrowsed(""); }}>
      <option value="">Insert field…</option>
      {fields.map((f) => (
        <option key={f.key} value={f.key} disabled={!f.value}>{f.label}{f.value ? "" : " (empty)"}</option>
      ))}
    </select>
  );
}
