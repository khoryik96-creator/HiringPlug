import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Schemas, unwrap } from "../api/client";
import { candidateKey } from "../api/keys";
import { formatPeriod } from "../labels";
import { SaveError } from "./SaveError";

type Work = Schemas["WorkEntry"];
type Education = Schemas["EducationEntry"];

/** Work history and education on the candidate page, with an editor for those who may update. */
export function History({ candidate, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const [editing, setEditing] = useState(false);
  if (editing) return <HistoryForm candidate={candidate} onDone={() => { setEditing(false); }} />;
  const { experience, education } = candidate;
  return (
    <section className="panel" aria-label="Work history and education">
      <div className="row spread">
        <h3>Work history</h3>
        {canEdit && <button type="button" onClick={() => { setEditing(true); }}>Edit history</button>}
      </div>
      {experience.length === 0 ? <p className="muted">None recorded.</p> : (
        <ul>
          {experience.map((j, i) => (
            <li key={i}>
              <strong>{j.title ?? "—"}</strong>{j.company && <> at {j.company}</>}{" "}
              <span className="muted">{formatPeriod(j.startDate, j.endDate, j.current)}</span>
              {j.description && <p className="muted">{j.description}</p>}
            </li>
          ))}
        </ul>
      )}
      <h3>Education</h3>
      {education.length === 0 ? <p className="muted">None recorded.</p> : (
        <ul>
          {education.map((e, i) => (
            <li key={i}>
              {[e.qualification, e.institution].filter(Boolean).join(", ")}{" "}
              {e.endYear && <span className="muted">{e.endYear}</span>}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

const emptyJob: Work = { title: null, company: null, startDate: null, endDate: null, current: false, description: null };
const emptySchool: Education = { qualification: null, institution: null, endYear: null };
const orNull = (v: string) => (v.trim() === "" ? null : v);

function HistoryForm({ candidate, onDone }: { candidate: Candidate; onDone: () => void }) {
  const queryClient = useQueryClient();
  // The version the form was opened with: a save by someone else since then is a conflict.
  const [opened] = useState(() => ({ version: candidate.version }));
  const [work, setWork] = useState<Work[]>(candidate.experience);
  const [education, setEducation] = useState<Education[]>(candidate.education);
  const save = useMutation({
    mutationFn: async () =>
      unwrap(await api.PUT("/v1/candidates/{candidateId}/history", {
        params: { path: { candidateId: candidate.id } },
        body: { version: opened.version, experience: work, education },
      })),
    onSuccess: (saved) => {
      queryClient.setQueryData(candidateKey(saved.id), saved);
      onDone();
    },
  });
  const setJob = (i: number, patch: Partial<Work>) => { setWork(work.map((j, k) => (k === i ? { ...j, ...patch } : j))); };
  const setSchool = (i: number, patch: Partial<Education>) => { setEducation(education.map((e, k) => (k === i ? { ...e, ...patch } : e))); };

  return (
    <form className="panel" aria-label="Edit work history and education" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h3>Work history</h3>
      {work.map((j, i) => (
        <fieldset key={i} className="form-grid" aria-label={`Job ${String(i + 1)}`}>
          <label className="field">Job title<input value={j.title ?? ""} onChange={(e) => { setJob(i, { title: orNull(e.target.value) }); }} /></label>
          <label className="field">Company<input value={j.company ?? ""} onChange={(e) => { setJob(i, { company: orNull(e.target.value) }); }} /></label>
          <label className="field">From (YYYY-MM)<input value={j.startDate ?? ""} placeholder="2020-01" onChange={(e) => { setJob(i, { startDate: orNull(e.target.value) }); }} /></label>
          <label className="field">To (YYYY-MM)<input value={j.endDate ?? ""} disabled={j.current} placeholder="2023-06" onChange={(e) => { setJob(i, { endDate: orNull(e.target.value) }); }} /></label>
          <label className="row"><input type="checkbox" checked={j.current} onChange={(e) => { setJob(i, { current: e.target.checked, endDate: e.target.checked ? null : j.endDate }); }} /> Current job</label>
          <button type="button" className="link" onClick={() => { setWork(work.filter((_, k) => k !== i)); }}>Remove job</button>
        </fieldset>
      ))}
      <button type="button" onClick={() => { setWork([...work, emptyJob]); }}>Add job</button>
      <h3>Education</h3>
      {education.map((e, i) => (
        <fieldset key={i} className="form-grid" aria-label={`Qualification ${String(i + 1)}`}>
          <label className="field">Qualification<input value={e.qualification ?? ""} onChange={(ev) => { setSchool(i, { qualification: orNull(ev.target.value) }); }} /></label>
          <label className="field">Institution<input value={e.institution ?? ""} onChange={(ev) => { setSchool(i, { institution: orNull(ev.target.value) }); }} /></label>
          <label className="field">Year finished<input inputMode="numeric" value={e.endYear ?? ""} onChange={(ev) => { setSchool(i, { endYear: ev.target.value.trim() === "" ? null : Number(ev.target.value) }); }} /></label>
          <button type="button" className="link" onClick={() => { setEducation(education.filter((_, k) => k !== i)); }}>Remove</button>
        </fieldset>
      ))}
      <button type="button" onClick={() => { setEducation([...education, emptySchool]); }}>Add qualification</button>
      <div className="row">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError error={save.error} shown={[]} onReload={() => { void queryClient.invalidateQueries({ queryKey: candidateKey(candidate.id) }); onDone(); }} />
    </form>
  );
}
