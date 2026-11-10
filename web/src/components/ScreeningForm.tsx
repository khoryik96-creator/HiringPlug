// The screening call form: notes taken during the call, and what was asked
// (salary and currency, notice period, work authorization, nationality, and
// the source when the candidate has none), saved to the profile in one step.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Permission, type Schemas, unwrap } from "../api/client";
import { activityKey, candidateKey, candidatesKey, notesKey, screeningCallsKey, usersKey } from "../api/keys";
import { formatDay, regionName } from "../labels";
import { CallItems, type CallAnswers, callNote, useCallItems } from "./CallItems";
import { type CandidateFormState, formFromCandidate, toUpdateBody } from "./candidateFormState";
import { Field } from "./Field";
import { COUNTRIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import { ScreeningSalary } from "./ScreeningSalary";
import { formFromCompensation, SALARY_PATHS, salaryProblems, type SalaryFormState, toCompensationBody } from "./salaryFormState";
import { SaveError } from "./SaveError";
import { SourcePicker } from "./SourcePicker";
import { NoticeField, WorkAuthFields } from "./ProfileFields";
import { useFieldSettings } from "./useFieldSettings";

/** Today's date where the recruiter is (YYYY-MM-DD). */
const localToday = () => {
  const d = new Date();
  return `${String(d.getFullYear())}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};

interface Base {
  version: number;
  profile: CandidateFormState;
  salaryVersion: number;
  salary: SalaryFormState;
}
const baseOf = (c: Candidate): Base => ({
  version: c.version,
  profile: formFromCandidate(c),
  salaryVersion: c.compensation?.version ?? 0,
  salary: formFromCompensation(c.compensation),
});

/** The request: the profile fields asked, the salary when it changed, and the notes. */
export function screeningBody(
  base: Base, profile: CandidateFormState, salary: SalaryFormState, notes: string, sensitive: boolean, withSalary: boolean,
): Schemas["ScreeningCallInput"] {
  const p = toUpdateBody(profile, base.version);
  const was = toUpdateBody(base.profile, base.version);
  const body: Schemas["ScreeningCallInput"] = { version: base.version, notes, sensitive };
  // Only what changed, so a field someone else edited meanwhile is not sent back.
  if (!sameBody(p.nationality, was.nationality)) body.nationality = p.nationality;
  if (!sameBody(p.workAuthorization, was.workAuthorization)) body.workAuthorization = p.workAuthorization;
  if (!sameBody(p.noticePeriod, was.noticePeriod)) body.noticePeriod = p.noticePeriod;
  if (profile.source && !base.profile.source) body.source = profile.source;
  const comp = toCompensationBody(salary, base.salaryVersion);
  if (withSalary && !sameBody(comp, toCompensationBody(base.salary, base.salaryVersion))) body.compensation = comp;
  return body;
}

export function ScreeningForm({ candidate, permissions }: { candidate: Candidate; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const [base, setBase] = useState(() => baseOf(candidate));
  const [profile, setProfile] = useState(base.profile);
  const [salary, setSalary] = useState(base.salary);
  const [notes, setNotes] = useState(""); // other notes, after the items
  const [answers, setAnswers] = useState<CallAnswers>({});
  const items = useCallItems("screening_call");
  const [sensitive, setSensitive] = useState(false);
  const [saved, setSaved] = useState<string | null>(null);
  // Who made the call and when: you and today unless said otherwise (a call logged later).
  const [calledOn, setCalledOn] = useState(localToday);
  const [calledBy, setCalledBy] = useState("");
  const users = useQuery({ queryKey: usersKey, queryFn: async () => unwrap(await api.GET("/v1/users")).items, staleTime: 60_000 });
  const salaryShown = permissions.includes("candidates.compensation:update") && !candidate.restrictedFields.includes("compensation");
  const canNote = permissions.includes("notes:create");
  const startFrom = (c: Candidate) => {
    const next = baseOf(c);
    setBase(next);
    setProfile(next.profile);
    setSalary(next.salary);
  };

  const save = useMutation({
    mutationFn: async () => {
      const problems = salaryShown ? salaryProblems(salary) : {};
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const body = { ...screeningBody(base, profile, salary, callNote(items, answers, notes), sensitive, salaryShown), calledOn, ...(calledBy ? { calledBy } : {}) };
      return unwrap(await api.POST("/v1/candidates/{candidateId}/screening-calls", { params: { path: { candidateId: candidate.id } }, body }));
    },
    onSuccess: (res) => {
      queryClient.setQueryData(candidateKey(candidate.id), res.candidate);
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
      void queryClient.invalidateQueries({ queryKey: notesKey("candidate", candidate.id) });
      void queryClient.invalidateQueries({ queryKey: activityKey("candidate", candidate.id) });
      void queryClient.invalidateQueries({ queryKey: screeningCallsKey(candidate.id) });
      startFrom(res.candidate);
      setNotes("");
      setAnswers({});
      setSensitive(false); // each note is marked on its own
      const sent = !!screeningBody(base, profile, salary, "", false, salaryShown).compensation;
      setSaved(`Call of ${formatDay(res.call.calledOn)} saved to ${res.candidate.fullName}'s profile${res.noteId ? ", with the notes" : ""}.`
        + (res.sourceKept ? " Their source was kept." : "")
        + (sent && !res.salaryApplied ? " The salary was not changed: a later call already set it (the salary follows the latest call)." : ""));
      setCalledOn(localToday());
      setCalledBy("");
    },
  });
  const errors = fieldErrors(save.error);
  const change = (patch: Partial<CandidateFormState>) => {
    setSaved(null);
    setProfile((f) => ({ ...f, ...patch }));
  };
  const setSalaryField = <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => {
    setSaved(null);
    setSalary((f) => ({ ...f, [key]: value }));
  };

  return (
    <form className="panel form-grid" aria-label="Screening call" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h3 className="full">Screening call</h3>
      <Field label="Call date" error={errors.calledOn}>
        <input type="date" value={calledOn} max={localToday()} onChange={(e) => { setSaved(null); setCalledOn(e.target.value); }} />
      </Field>
      <Field label="Called by" error={errors.calledBy}>
        <select value={calledBy} onChange={(e) => { setSaved(null); setCalledBy(e.target.value); }}>
          <option value="">Me</option>
          {users.data?.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </select>
      </Field>
      {canNote && (
        <div className="full">
          <p className="muted small">What to ask is set in Settings &gt; Screening call. Each answer is saved in the notes after its label (&ldquo;RFL: &hellip;&rdquo;).</p>
          <CallItems list="screening_call" answers={answers} onChange={(a) => { setSaved(null); setAnswers(a); }} />
          <Field label="Other notes" error={errors.body ?? errors.notes}>
            <textarea rows={4} value={notes}
              onChange={(e) => { setSaved(null); setNotes(e.target.value); }} />
          </Field>
          {permissions.includes("notes.sensitive:read") && (
            <label className="row"><input type="checkbox" checked={sensitive} onChange={(e) => { setSensitive(e.target.checked); }} /> Sensitive note (only roles that see sensitive notes)</label>
          )}
        </div>
      )}
      <NoticeField form={profile} onChange={change} label="Notice period" error={errors["noticePeriod.value"] ?? errors["noticePeriod.unit"]} />
      <Field label="Nationality" error={errors.nationality}>
        <select value={profile.nationality.toUpperCase()} onChange={(e) => { change({ nationality: e.target.value }); }}>
          <option value="">—</option>
          {[...new Set([...COUNTRIES, ...(profile.nationality ? [profile.nationality.toUpperCase()] : [])])].map((c) => (
            <option key={c} value={c}>{regionName(c) ?? c}</option>
          ))}
        </select>
      </Field>
      <WorkAuthFields form={profile} onChange={change} label="Work authorization"
        errors={{ status: errors["workAuthorization.status"], basis: errors["workAuthorization.basis"] }} />
      <div className="full field">
        <span>Source</span>
        {base.profile.source ? (
          <p className="muted">{fs.label("candidate_source", base.profile.source)} (kept: they already have a source)</p>
        ) : (
          <SourcePicker value={profile.source} onChange={(v) => { change({ source: v }); }} label="Source" />
        )}
        {errors.source && <small className="error">{errors.source}</small>}
      </div>
      {salaryShown && <ScreeningSalary form={salary} set={setSalaryField} patch={(value) => { setSaved(null); setSalary((f) => ({ ...f, ...value })); }} errors={errors} />}
      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save the call</button>
        {saved && <span role="status" className="muted">{saved}</span>}
      </div>
      <SaveError
        error={save.error}
        shown={["body", "notes", "calledOn", "calledBy", "nationality", "noticePeriod.value", "noticePeriod.unit", "workAuthorization.status", "workAuthorization.basis", "source", ...Object.values(SALARY_PATHS)]}
        reloadLabel="Reload their version (your notes are kept; other changes here are lost)"
        onReload={() => {
          // Someone saved first: start again from their version, keeping the notes typed so far.
          void queryClient.invalidateQueries({ queryKey: candidateKey(candidate.id) }).then(() => {
            const fresh = queryClient.getQueryData<Candidate>(candidateKey(candidate.id));
            if (fresh) startFrom(fresh);
            save.reset();
          });
        }}
      />
    </form>
  );
}
