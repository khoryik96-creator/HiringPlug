// The candidates an email to the client is about: each one's profile to
// attach, their order, more from the job, and their details put in the
// message as a block each or as a table.
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { loadApplications } from "../../api/applications";
import { jobApplicationsKey } from "../../api/keys";
import { EmailCandidateSearch } from "./EmailCandidateSearch";
import { salaryValues } from "./useFieldValues";
import { STAGE } from "../../labels";
import { ErrorMessage } from "../ErrorMessage";
import { blocksHtml, choiceKey, type CvChoice, DEFAULT_DETAILS, DETAILS, type Layout, missingDetails, type Person, type Profile, profilesOf, tableHtml } from "./shortlistHtml";

/** A candidate on the email; profile undefined: their newest profile (none if they have none). */
export type Shortlisted = {
  candidateId: string;
  applicationId?: string;
  name: string;
  title?: string | null;
  stage?: Schemas["Stage"];
  profile?: CvChoice | null;
  // The versions the details were read at: sending fails if they changed since.
  candidateVersion?: number;
  compensationVersion?: number;
};

const timeZone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

/**
 * Each candidate's profiles and values (as the sender may see them; their salary only when salary details
 * are put in: withSalary), and the CVs chosen.
 */
export function useShortlist(people: Shortlisted[], canSeeCvs: boolean) {
  const queryClient = useQueryClient();
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
  const personAt = (i: number, read = values[i]?.data): Person => ({
    id: people[i]?.candidateId,
    name: read?.fieldValues["[Full name]"] ?? people[i]?.name ?? "",
    values: read?.fieldValues ?? {},
  });
  /** As personAt, a salary field with a value not asked for yet counted as there (what is not on record). */
  const knownAt = (i: number): Person => {
    const p = personAt(i);
    const withheld = values[i]?.data?.withheldFields ?? [];
    return { ...p, values: { ...Object.fromEntries(withheld.map((t) => [t, t])), ...p.values } };
  };
  /**
   * Everyone's values with their salary's, asked for now (each counted as shown): salary details to put
   * in. The reads are kept for the versions their details were read at.
   */
  const withSalary = async (): Promise<{ people: Person[]; reads: Schemas["EmailPreview"][] }> => {
    const reads = await Promise.all(people.map((p) => queryClient.query({
      queryKey: ["email-context", p.applicationId ?? p.candidateId, "salary"], staleTime: 0,
      queryFn: () => salaryValues({ subject: "", bodyHtml: "", candidateId: p.candidateId, applicationId: p.applicationId, timeZone: timeZone() }),
    })));
    return { people: reads.map((r, i) => personAt(i, r)), reads };
  };
  // A candidate sent with "No attachment" needs no documents.
  const wantsDocs = (i: number) => people[i]?.profile !== null;
  const loading = values.some((v) => v.isPending) || (canSeeCvs && docs.some((d, i) => wantsDocs(i) && d.isPending));
  /** The people with the versions their details were read at (salary's only when shown: salaryReads). */
  const snapshot = (salaryReads?: Schemas["EmailPreview"][]): Shortlisted[] => people.map((p, i) => {
    const read = salaryReads?.[i] ?? values[i]?.data;
    return {
      ...p,
      ...(read?.candidateVersion !== undefined ? { candidateVersion: read.candidateVersion } : {}),
      compensationVersion: salaryReads ? read?.compensationVersion : undefined,
    };
  });
  const refresh = () => Promise.all([
    ...values.map((q) => q.refetch()),
    ...(canSeeCvs ? docs.map((q) => q.refetch()) : []),
  ]);
  const error = values.find((v) => v.error)?.error ?? docs.find((d, i) => wantsDocs(i) && d.error)?.error;
  return { profiles, profileOf, cvs, personAt, knownAt, withSalary, loading, snapshot, refresh, error };
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

export function ShortlistPanel({ jobId, people, onPeople, list, canSeeCvs, canSeeSalary = false, allowEmpty = false, attachProfiles = true, rowTool, onRefresh, onInsert }: {
  jobId?: string;
  people: Shortlisted[];
  onPeople: (people: Shortlisted[]) => void;
  list: ReturnType<typeof useShortlist>;
  canSeeCvs: boolean;
  canSeeSalary?: boolean;
  allowEmpty?: boolean;
  /** false: details only, no profile to attach (the email has its own attachments), and no Refresh link. */
  attachProfiles?: boolean;
  /** Without profiles to attach: something else on each candidate's row (Outlook: their CV to attach). */
  rowTool?: (p: Shortlisted, name: string) => ReactNode;
  onRefresh?: () => void;
  onInsert: (html: string, snapshot: Shortlisted[]) => void;
}) {
  const [layout, setLayout] = useState<Layout>(() => read("hiringplug.shortlist.layout", "blocks"));
  const [details, setDetails] = useState<string[]>(() => read("hiringplug.shortlist.details", DEFAULT_DETAILS));
  const onJob = useQuery({
    queryKey: jobApplicationsKey(jobId ?? ""),
    enabled: jobId !== undefined,
    queryFn: ({ signal }) => loadApplications("job", jobId ?? "", signal),
  });
  const more = (onJob.data?.items ?? []).filter((a) => a.stage !== "rejected" && a.stage !== "withdrawn" && !people.some((p) => p.candidateId === a.candidateId));
  const available = DETAILS.filter((d) => !d.salary || canSeeSalary);
  const chosen = available.filter((d) => details.includes(d.id));
  const shown = people.map((_, i) => list.personAt(i));
  const gaps = list.loading ? [] : missingDetails(people.map((_, i) => list.knownAt(i)), chosen);
  // Salary details are read when they are put in (counted as shown), not before.
  const [putting, setPutting] = useState(false);
  const [putError, setPutError] = useState<unknown>(null);
  const put = async () => {
    setPutting(true);
    setPutError(null);
    try {
      const salary = chosen.some((d) => d.salary) ? await list.withSalary() : null;
      const who = salary?.people ?? shown;
      onInsert(layout === "blocks" ? blocksHtml(who, chosen) : tableHtml(who, chosen), list.snapshot(salary?.reads));
    } catch (e) {
      setPutError(e);
    } finally {
      setPutting(false);
    }
  };
  const move = (i: number, by: number) => {
    const next = [...people];
    const [p] = next.splice(i, 1);
    if (p) next.splice(i + by, 0, p);
    onPeople(next);
  };

  return (
    <fieldset className="shortlist" aria-label="Candidates">
      <legend>Selected candidates ({people.length}/10)</legend>
      <EmailCandidateSearch people={people} onAdd={(p) => { onPeople([...people, p]); }} />
      <ol className="shortlist-rows">
        {people.map((p, i) => (
          <li key={p.candidateId} className="shortlist-row">
            <span className="shortlist-who">
              <strong>{shown[i]?.name || p.name}</strong>
              <span className="muted small">{[p.title, p.stage ? STAGE[p.stage] : null].filter(Boolean).join(" · ")}</span>
            </span>
            {!attachProfiles ? (rowTool?.(p, shown[i]?.name || p.name) ?? null) : canSeeCvs ? (
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
              onClick={() => { onRefresh?.(); setLayout(l); keep("hiringplug.shortlist.layout", l); }}>
              {l === "blocks" ? "A block each" : "One table"}
            </button>
          ))}
        </div>
        <span className="shortlist-details" role="group" aria-label="Details to show">
          {available.map((d) => (
            <label key={d.id} className="small">
              <input type="checkbox" checked={details.includes(d.id)} onChange={(e) => {
                const next = e.target.checked ? DETAILS.filter((x) => details.includes(x.id) || x.id === d.id).map((x) => x.id) : details.filter((x) => x !== d.id);
                onRefresh?.();
                setDetails(next);
                keep("hiringplug.shortlist.details", next);
              }} /> {d.label}
            </label>
          ))}
        </span>
        <button
          type="button"
          aria-label="Insert candidates into the message"
          disabled={list.loading || !!list.error || people.length === 0 || putting}
          onClick={() => { void put(); }}
        >
          Insert
        </button>
      </div>
      {/* Details only (Outlook): read again on adding a candidate and on coming back to the tab. */}
      {people.length > 0 && attachProfiles && (
        <button type="button" className="link" onClick={() => { onRefresh?.(); void list.refresh(); }}>
          Refresh candidate details and CVs
        </button>
      )}
      {gaps.length > 0 && <p className="warning small">Not on record (shown as —): {gaps.join("; ")}.</p>}
      <ErrorMessage error={putError} />
    </fieldset>
  );
}
