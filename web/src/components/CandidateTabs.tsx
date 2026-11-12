import { useState, type ReactNode } from "react";

export type CandidateTab = "details" | "cv" | "jobs" | "activity" | "salary" | "email";
export type CandidateSection = { id: CandidateTab; label: string; content: ReactNode };

/** Visit lazily, then retain mounted forms and drafts while another tab is open. */
export function CandidateTabs({ sections, active, onChange }: {
  sections: CandidateSection[]; active: CandidateTab; onChange: (tab: CandidateTab) => void;
}) {
  const current = sections.some(s => s.id === active) ? active : sections[0]?.id;
  const [visited, setVisited] = useState<Set<CandidateTab>>(new Set<CandidateTab>(current ? [current] : []));
  if (current && !visited.has(current)) setVisited(new Set([...visited, current]));
  const select = (id: CandidateTab) => { setVisited(old => new Set([...old, id])); onChange(id); };
  return <div className="candidate-tabs">
    <div role="tablist" aria-label="Candidate sections" className="candidate-tablist">
      {sections.map((section, index) => <button key={section.id} type="button" role="tab" id={`candidate-tab-${section.id}`}
        aria-controls={`candidate-panel-${section.id}`} aria-selected={current === section.id} tabIndex={current === section.id ? 0 : -1}
        onClick={() => { select(section.id); }} onKeyDown={event => {
          const offset = event.key === "ArrowRight" ? 1 : event.key === "ArrowLeft" ? -1 : 0;
          const next = event.key === "Home" ? sections[0] : event.key === "End" ? sections.at(-1) : offset ? sections[(index + offset + sections.length) % sections.length] : undefined;
          if (next) { event.preventDefault(); select(next.id); document.getElementById(`candidate-tab-${next.id}`)?.focus(); }
        }}>{section.label}</button>)}
    </div>
    {sections.filter(s => s.id === current || visited.has(s.id)).map(s => <div key={s.id} role="tabpanel" id={`candidate-panel-${s.id}`}
      aria-labelledby={`candidate-tab-${s.id}`} hidden={s.id !== current} tabIndex={0}>{s.content}</div>)}
  </div>;
}
