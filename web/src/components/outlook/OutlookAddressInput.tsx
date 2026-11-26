// To, Cc and Bcc as Outlook fills them: typing part of an address or a name
// suggests the people in your own mail (most written to first); choosing one
// puts it in, ready for the next. Several addresses are separated by commas.
import { useQuery } from "@tanstack/react-query";
import { type InputHTMLAttributes, useId, useState } from "react";

import { api, unwrap } from "../../api/client";

/** What is being typed: the part after the last comma or semicolon. */
export function typedAddress(value: string): string {
  return value.split(/[,;]/).pop()?.trim() ?? "";
}

/** The value with what is being typed replaced by the chosen address. */
export function withAddress(value: string, address: string): string {
  const cut = Math.max(value.lastIndexOf(","), value.lastIndexOf(";"));
  const before = cut < 0 ? "" : `${value.slice(0, cut + 1).trimEnd()} `;
  return `${before}${address}, `;
}

type Props = Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange"> & { value: string; onChange: (value: string) => void };

export function OutlookAddressInput({ value, onChange, ...input }: Props) {
  const listId = useId();
  const [open, setOpen] = useState(false);
  const [at, setAt] = useState(0);
  const typed = typedAddress(value);
  const already = new Set(value.split(/[,;\s]+/).map((a) => a.trim().toLowerCase()).filter(Boolean));
  const suggestions = useQuery({
    queryKey: ["outlook", "addresses", typed.toLowerCase()],
    queryFn: async ({ signal }) => unwrap(await api.GET("/v1/outlook/addresses", { params: { query: { q: typed } }, signal })).items,
    enabled: open && typed.length > 0,
    staleTime: 5 * 60_000,
  });
  const items = (suggestions.data ?? []).filter((s) => !already.has(s.address) || s.address === typed.toLowerCase());
  const shown = open && typed.length > 0 && items.length > 0;
  const choose = (address: string) => { onChange(withAddress(value, address)); setAt(0); };
  return (
    <span className="address-input">
      <input {...input} value={value} autoComplete="off" role="combobox" aria-expanded={shown} aria-controls={listId} aria-autocomplete="list"
        aria-activedescendant={shown ? `${listId}-${String(Math.min(at, items.length - 1))}` : undefined}
        onChange={(e) => { onChange(e.target.value); setOpen(true); setAt(0); }}
        onBlur={() => { setOpen(false); }}
        onKeyDown={(e) => {
          if (!shown) return;
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            setAt((i) => (i + (e.key === "ArrowDown" ? 1 : items.length - 1)) % items.length);
          } else if (e.key === "Enter" || e.key === "Tab") {
            const pick = items[Math.min(at, items.length - 1)];
            if (pick) { e.preventDefault(); choose(pick.address); }
          } else if (e.key === "Escape") {
            e.preventDefault(); e.stopPropagation(); setOpen(false); // the list closes, not the window
          }
        }} />
      {shown && (
        <ul id={listId} className="address-suggestions" role="listbox" aria-label="Addresses you have emailed">
          {items.map((s, i) => (
            <li key={s.address} id={`${listId}-${String(i)}`} role="option" aria-selected={i === Math.min(at, items.length - 1)}
              // Before the box loses focus, so the choice is not lost.
              onMouseDown={(e) => { e.preventDefault(); choose(s.address); }}>
              {s.name ? <><strong>{s.name}</strong> <span className="muted">{s.address}</span></> : s.address}
            </li>
          ))}
        </ul>
      )}
    </span>
  );
}
