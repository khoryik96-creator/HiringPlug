import type { Document } from "../api/client";

const added = new Intl.DateTimeFormat("en-GB", {
  day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit", second: "2-digit",
});

export function replacementCount(documents: Document[], formatted: boolean): number {
  return Math.max(0, documents.length + 1 - (formatted ? 3 : 4));
}

export function CvReplacement({ documents, required, selected, onChange }: {
  documents: Document[]; required: number; selected: string[]; onChange: (ids: string[]) => void;
}) {
  if (!required) return null;
  const label = required === 1 ? "Replace a CV" : `Replace ${String(required)} CVs`;
  const title = (d: Document) => `${d.fileName} · ${added.format(new Date(d.createdAt))}`;
  return <fieldset className="field full cv-replacement">
    <legend>{label}</legend>
    {required === 1 ? <select aria-label={label} required value={selected[0] ?? ""} onChange={(e) => { onChange(e.target.value ? [e.target.value] : []); }}>
      <option value="">Choose a CV to replace</option>
      {documents.map((d) => <option key={d.id} value={d.id}>{title(d)}</option>)}
    </select> : documents.map((d) => <label className="row" key={d.id}>
      <input type="checkbox" checked={selected.includes(d.id)} disabled={!selected.includes(d.id) && selected.length >= required}
        onChange={(e) => { onChange(e.target.checked ? [...selected, d.id] : selected.filter((id) => id !== d.id)); }} />
      {title(d)}
    </label>)}
    <small className="muted">The selected CVs go to the Bin for 30 days after the new CV is saved.</small>
  </fieldset>;
}
