import { useState, type ReactNode } from "react";

export type Section<T extends string> = { id: T; label: string; content: ReactNode };

/**
 * A record's sections as tabs (a candidate's, a company's, a contact's). Tabs
 * load when first opened, then stay mounted so forms and drafts survive a
 * switch to another tab. `name` keeps ids apart ("candidate-tab-details").
 */
export function SectionTabs<T extends string>({ name, label, sections, active, onChange }: {
  name: string; label: string; sections: Section<T>[]; active: T; onChange: (tab: T) => void;
}) {
  const current = sections.some(s => s.id === active) ? active : sections[0]?.id;
  const [visited, setVisited] = useState<Set<T>>(new Set<T>(current ? [current] : []));
  if (current && !visited.has(current)) setVisited(new Set([...visited, current]));
  const select = (id: T) => { setVisited(old => new Set([...old, id])); onChange(id); };
  return <div className="candidate-tabs">
    <div role="tablist" aria-label={label} className="candidate-tablist">
      {sections.map((section, index) => <button key={section.id} type="button" role="tab" id={`${name}-tab-${section.id}`}
        aria-controls={`${name}-panel-${section.id}`} aria-selected={current === section.id} tabIndex={current === section.id ? 0 : -1}
        onClick={() => { select(section.id); }} onKeyDown={event => {
          const offset = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
          const next = event.key === "Home" ? sections[0] : event.key === "End" ? sections.at(-1) : offset ? sections[(index + offset + sections.length) % sections.length] : undefined;
          if (next) { event.preventDefault(); select(next.id); document.getElementById(`${name}-tab-${next.id}`)?.focus(); }
        }}>{section.label}</button>)}
    </div>
    {sections.filter(s => s.id === current || visited.has(s.id)).map(s => <div key={s.id} role="tabpanel" id={`${name}-panel-${s.id}`}
      aria-labelledby={`${name}-tab-${s.id}`} hidden={s.id !== current} tabIndex={0}>{s.content}</div>)}
  </div>;
}
