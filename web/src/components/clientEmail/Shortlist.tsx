// The candidates an email to the client is about: each one's profile to
// attach, their order, more from the job, and their details put in the
// message as a block each or as a table.
import { useQueries, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { jobApplicationsKey } from "../../api/keys";
import { EmailCandidateSearch } from "./EmailCandidateSearch";
import { STAGE } from "../../labels";
import { blocksHtml, choiceKey, type CvChoice, DEFAULT_DETAILS, DETAILS, type Layout, missingDetails, type Person, type Profile, profilesOf, tableHtml } from "./shortlistHtml";

/** A candidate on the email; profile undefined: their newest profile (none if they have none). */
export type Shortlisted = { candidateId: string; applicationId?: string; name: string; title?: string | null; stage?: Schemas["Stage"]; profile?: CvChoice | null; candidateVersion?: number; compensationVersion?: number };

const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

/** Each candidate's profiles and values (as the sender may see them), and the CVs chosen. */
export function useShortlist(people: Shortlisted[], canSeeCvs: boolean) {
  const docs = useQueries({
    queries: people.map((p) => ({
      queryKey: ["documents", p.candidateId],
      enabled: canSeeCvs,
      queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId: p.candidateId } } })).items,
    })),
  });
  const values = useQueries({
    queries: people.map((p) => ({
      queryKey: ["email-context", p.applicationId ?? p.candidateId],
      queryFn: async () => unwrap(await api.POST("/v1/email-previews", {
        body: { subject: "", bodyHtml: "", candidateId: p.candidateId, applicationId: p.applicationId, timeZone: timeZone() },
      })),
    })),
  });
  const profiles: Profile[][] = docs.map((d) => profilesOf(d.data ?? []));
  const profileOf = (i: number): CvChoice | null => {
    if(!canSeeCvs) return null;
    const chosen = people[i]?.profile;
    return chosen === undefined ? profiles[i]?.[0]?.choice ?? null : chosen;
  };
  const cvs = people.map((_, i) => profileOf(i)).filter((c): c is CvChoice => c !== null);
  const personAt = (i: number): Person => ({ id:people[i]?.candidateId, name: values[i]?.data?.fieldValues["[Full name]"] ?? people[i]?.name ?? "", values: values[i]?.data?.fieldValues ?? {} });
  const loading = values.some((v) => v.isPending) || (canSeeCvs && docs.some((d,i)=>people[i]?.profile!==null && d.isPending));
  const snapshot = (includeSalary:boolean):Shortlisted[]=>people.map((p,i)=>({ ...p,
   ...(values[i]?.data?.candidateVersion !== undefined ? {candidateVersion:values[i].data.candidateVersion}:{}),
   ...(includeSalary && values[i]?.data?.compensationVersion !== undefined ? {compensationVersion:values[i].data.compensationVersion}:{compensationVersion:undefined})
  }));
  const refresh = () => Promise.all([...values.map((q)=>q.refetch()),...(canSeeCvs?docs.map((q)=>q.refetch()):[])]);
  return { profiles, profileOf, cvs, personAt, loading, snapshot, refresh, error: values.find((v) => v.error)?.error ?? docs.find((d,i) => people[i]?.profile!==null && d.error)?.error };
}

const read = <T,>(key: string, fallback: T): T => {
  try {
    const v = window.localStorage.getItem(key);
    return v ? (JSON.parse(v) as T) : fallback;
  } catch {
    return fallback;
  }
};
const keep = (key: string, value: unknown) => {
  try { window.localStorage.setItem(key, JSON.stringify(value)); } catch { /* only a convenience */ }
};

export function ShortlistPanel({ jobId, people, onPeople, list, canSeeCvs, canSeeSalary = false, allowEmpty = false, onRefresh, onInsert }: {
  jobId?: string;
  people: Shortlisted[];
  onPeople: (people: Shortlisted[]) => void;
  list: ReturnType<typeof useShortlist>;
  canSeeCvs: boolean;
  canSeeSalary?: boolean;allowEmpty?:boolean;onRefresh?:()=>void;
  onInsert: (html: string,snapshot:Shortlisted[]) => void;
}) {
  const [layout, setLayout] = useState<Layout>(() => read("hiringplug.shortlist.layout", "blocks"));
  const [details, setDetails] = useState<string[]>(() => read("hiringplug.shortlist.details", DEFAULT_DETAILS));
  const onJob = useQuery({
    queryKey: jobApplicationsKey(jobId ?? ""),
    enabled: jobId !== undefined,
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/applications", { params: { path: { jobId: jobId ?? "" } } })),
  });
  const more = (onJob.data?.items ?? []).filter((a) => a.stage !== "rejected" && a.stage !== "withdrawn" && !people.some((p) => p.candidateId === a.candidateId));
  const available=DETAILS.filter((d)=>!d.salary || canSeeSalary);
  const chosen = available.filter((d) => details.includes(d.id));
  const shown = people.map((_, i) => list.personAt(i));
  const gaps = list.loading ? [] : missingDetails(shown, chosen);
  const move = (i: number, by: number) => {
    const next = [...people];
    const [p] = next.splice(i, 1);
    if (p) next.splice(i + by, 0, p);
    onPeople(next);
  };

  return (
    <fieldset className="shortlist" aria-label="Candidates">
      <legend>Selected candidates ({people.length}/10)</legend>
      <EmailCandidateSearch people={people} onAdd={(p)=>{onPeople([...people,p]);}} />
      <ol className="shortlist-rows">
        {people.map((p, i) => (
          <li key={p.candidateId} className="shortlist-row">
            <span className="shortlist-who">
              <strong>{p.name}</strong>
              <span className="muted small">{[p.title, p.stage ? STAGE[p.stage] : null].filter(Boolean).join(" · ")}</span>
            </span>
            {canSeeCvs ? (
              <select aria-label={`Profile to attach for ${p.name}`} value={choiceKey(list.profileOf(i))}
                onChange={(e) => {
                  const picked = list.profiles[i]?.find((x) => choiceKey(x.choice) === e.target.value)?.choice ?? null;
                  onPeople(people.map((q, j) => (j === i ? { ...q, profile: picked } : q)));
                }}>
                {list.profiles[i]?.map((x) => <option key={choiceKey(x.choice)} value={choiceKey(x.choice)}>📎 {x.label}</option>)}
                <option value="">No attachment</option>
              </select>
            ) : <span className="muted small">Attaching profiles needs permission to see documents.</span>}
            <span className="row shortlist-buttons">
              <button type="button" className="link" aria-label={`Move ${p.name} up`} disabled={i === 0} onClick={() => { move(i, -1); }}>↑</button>
              <button type="button" className="link" aria-label={`Move ${p.name} down`} disabled={i === people.length - 1} onClick={() => { move(i, 1); }}>↓</button>
              <button type="button" className="link" disabled={people.length === 1 && !allowEmpty} onClick={() => { onPeople(people.filter((_, j) => j !== i)); }}>Remove</button>
            </span>
          </li>
        ))}
      </ol>
      {more.length > 0 && people.length < 10 && (
        <label className="row small">+ Add from this job
          <select aria-label="Add a candidate from this job" value="" onChange={(e) => {
            const a = more.find((x) => x.id === e.target.value);
            if (a) onPeople([...people, { candidateId: a.candidateId, applicationId: a.id, name: a.candidateName, title: a.candidateTitle, stage: a.stage }]);
          }}>
            <option value="">Choose…</option>
            {more.map((a) => <option key={a.id} value={a.id}>{a.candidateName} ({STAGE[a.stage]})</option>)}
          </select>
        </label>
      )}

      <div className="shortlist-insert">
        <div className="segmented" role="group" aria-label="Write them as">
          {(["blocks", "table"] as const).map((l) => (
            <button key={l} type="button" aria-pressed={layout === l} className={layout === l ? "on" : undefined}
              onClick={() => { onRefresh?.();setLayout(l); keep("hiringplug.shortlist.layout", l); }}>
              {l === "blocks" ? "A block each" : "One table"}
            </button>
          ))}
        </div>
        <span className="shortlist-details" role="group" aria-label="Details to show">
          {available.map((d) => (
            <label key={d.id} className="small">
              <input type="checkbox" checked={details.includes(d.id)} onChange={(e) => {
                const next = e.target.checked ? DETAILS.filter((x) => details.includes(x.id) || x.id === d.id).map((x) => x.id) : details.filter((x) => x !== d.id);
                onRefresh?.();setDetails(next);
                keep("hiringplug.shortlist.details", next);
              }} /> {d.label}
            </label>
          ))}
        </span>
        <button type="button" disabled={list.loading || !!list.error || people.length===0} onClick={() => { onInsert(layout === "blocks" ? blocksHtml(shown, chosen) : tableHtml(shown, chosen),list.snapshot(chosen.some((d)=>d.salary))); }}>
          Insert candidates into the message
        </button>
      </div>
      {people.length>0 && <button type="button" className="link" onClick={()=>{onRefresh?.();void list.refresh();}}>Refresh candidate details and CVs</button>}
      {gaps.length > 0 && <p className="warning small">Not on record (shown as —): {gaps.join("; ")}.</p>}
    </fieldset>
  );
}
