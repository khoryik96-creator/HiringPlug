// What two CVs' details disagree on (job titles, skills, contacts...).
import { type Document } from "../api/client";
import { compareFields, compareJobs, compareSkills } from "./compareCvs";
import { SOURCE } from "./CvPane";

export function CvDifferences({ a, b }: { a: Document | undefined; b: Document | undefined }) {
  if (!a || !b) return null;
  const pa = a.parsedProfile ?? null;
  const pb = b.parsedProfile ?? null;
  if (!pa || !pb) {
    return <p className="muted">Read details from both CVs to compare them.</p>;
  }
  const fields = compareFields(pa, pb).filter((r) => r.differs);
  const jobs = compareJobs(pa, pb);
  const skills = compareSkills(pa, pb);
  const none = fields.length + jobs.onlyA.length + jobs.onlyB.length + jobs.changed.length + skills.onlyA.length + skills.onlyB.length === 0;
  return (
    <section className="panel" aria-label="Differences">
      <h3>Differences</h3>
      {none && <p>The details read from both CVs agree.</p>}
      {fields.length > 0 && (
        <table>
          <thead><tr><th>Field</th><th>{SOURCE[a.source]}</th><th>{SOURCE[b.source]}</th></tr></thead>
          <tbody>
            {fields.map((r) => <tr key={r.label}><td>{r.label}</td><td>{r.a || "—"}</td><td>{r.b || "—"}</td></tr>)}
          </tbody>
        </table>
      )}
      {jobs.changed.map((c) => <p key={c.a} className="warning">Changed: {c.a} → {c.b}</p>)}
      {jobs.onlyA.map((j) => <p key={j}>Only in the {SOURCE[a.source].toLowerCase()} CV: {j}</p>)}
      {jobs.onlyB.map((j) => <p key={j}>Only in the {SOURCE[b.source].toLowerCase()} CV: {j}</p>)}
      {skills.onlyA.length > 0 && <p>Skills only in the {SOURCE[a.source].toLowerCase()} CV: {skills.onlyA.join(", ")}</p>}
      {skills.onlyB.length > 0 && <p>Skills only in the {SOURCE[b.source].toLowerCase()} CV: {skills.onlyB.join(", ")}</p>}
    </section>
  );
}
