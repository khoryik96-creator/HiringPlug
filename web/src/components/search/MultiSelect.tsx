// A dropdown of choices to tick: "Status · Any", or "Status · 2" once some
// are chosen. Long lists get a box to narrow them.
import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";

export type Choice = { value: string; label: string };

export function MultiSelect({ label, choices, value, onChange, searchable = choices.length > 8, align = "start" }: {
  label: string;
  choices: Choice[];
  value: string[];
  onChange: (v: string[]) => void;
  searchable?: boolean;
  align?: "start" | "end";
}) {
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const box = useRef<HTMLDivElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [side, setSide] = useState(align);
  const id = useId();
  // Opened on its preferred side, unless that runs off the window: then the other.
  useLayoutEffect(() => {
    if (!open || !menu.current) return;
    const r = menu.current.getBoundingClientRect();
    const width = document.documentElement.clientWidth;
    if (align === "end" && r.left < 0) setSide("start");
    else if (align === "start" && r.right > width) setSide("end");
  }, [open, align]);
  // A click elsewhere closes it.
  useEffect(() => {
    if (!open) return;
    const away = (e: MouseEvent) => { if (box.current && !box.current.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("mousedown", away);
    return () => { document.removeEventListener("mousedown", away); };
  }, [open]);
  const shown = choices.filter((c) => !filter.trim() || c.label.toLowerCase().includes(filter.trim().toLowerCase()));
  const summary = value.length === 0 ? "Any"
    : value.length === 1 ? (choices.find((c) => c.value === value[0])?.label ?? value[0])
      : `${String(value.length)} chosen`;
  return (
    <div className="multi" ref={box}>
      <span className="multi-label" id={id}>{label}</span>
      <button type="button" className={`multi-button${value.length > 0 ? " set" : ""}`} aria-haspopup="listbox" aria-expanded={open}
        aria-labelledby={id} aria-describedby={`${id}-v`} onClick={() => { setSide(align); setOpen(!open); }}>
        <span id={`${id}-v`}>{summary}</span><span aria-hidden="true" className="multi-caret">▾</span>
      </button>
      {open && (
        <div ref={menu} className={`multi-menu${side === "end" ? " align-end" : ""}`} role="listbox" aria-multiselectable="true" aria-label={label}>
          {searchable && (
            <input className="multi-filter" placeholder="Type to narrow" aria-label={`Narrow ${label}`} value={filter} autoFocus
              onChange={(e) => { setFilter(e.target.value); }} />
          )}
          <div className="multi-options">
            {shown.length === 0 && <p className="muted small">None match.</p>}
            {shown.map((c) => {
              const on = value.includes(c.value);
              return (
                <label key={c.value} className={on ? "on" : undefined} role="option" aria-selected={on}>
                  <input type="checkbox" checked={on} onChange={() => { onChange(on ? value.filter((v) => v !== c.value) : [...value, c.value]); }} />
                  {c.label}
                </label>
              );
            })}
          </div>
          {value.length > 0 && <button type="button" className="link small" onClick={() => { onChange([]); }}>Clear</button>}
        </div>
      )}
    </div>
  );
}
