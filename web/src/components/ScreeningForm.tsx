// The screening call form: notes taken during the call, and what was asked
// (salary and currency, notice period, work authorization, nationality, and
// the source when the candidate has none), saved to the profile in one step.
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Candidate, type Permission, type Schemas, unwrap } from "../api/client";
import { activityKey, candidateKey, candidatesKey, notesKey } from "../api/keys";
import { keysOf, regionName, WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";
import { type CandidateFormState, formFromCandidate, toUpdateBody } from "./candidateFormState";
import { Field } from "./Field";
import { COUNTRIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import { SalaryFields } from "./SalaryForm";
import { formFromCompensation, SALARY_PATHS, salaryProblems, type SalaryFormState, toCompensationBody } from "./salaryFormState";
import { SaveError } from "./SaveError";
import { SourcePicker } from "./SourcePicker";
import { noticeKey, useFieldSettings } from "./useFieldSettings";

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
  const body: Schemas["ScreeningCallInput"] = {
    version: base.version, notes, sensitive,
    nationality: p.nationality, workAuthorization: p.workAuthorization, noticePeriod: p.noticePeriod,
  };
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
  const [notes, setNotes] = useState("");
  const [sensitive, setSensitive] = useState(false);
  const [saved, setSaved] = useState<string | null>(null);
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
      const body = screeningBody(base, profile, salary, notes.trim(), sensitive, salaryShown);
      return unwrap(await api.POST("/v1/candidates/{candidateId}/screening-calls", { params: { path: { candidateId: candidate.id } }, body }));
    },
    onSuccess: (res) => {
      queryClient.setQueryData(candidateKey(candidate.id), res.candidate);
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
      void queryClient.invalidateQueries({ queryKey: notesKey("candidate", candidate.id) });
      void queryClient.invalidateQueries({ queryKey: activityKey("candidate", candidate.id) });
      startFrom(res.candidate);
      setNotes("");
      setSaved(`Saved to ${res.candidate.fullName}'s profile${res.noteId ? ", with the notes" : ""}.${res.sourceKept ? " Their source was kept." : ""}`);
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof CandidateFormState>(key: K, value: CandidateFormState[K]) => {
    setSaved(null);
    setProfile((f) => ({ ...f, [key]: value }));
  };
  const setSalaryField = <K extends keyof SalaryFormState>(key: K, value: SalaryFormState[K]) => {
    setSaved(null);
    setSalary((f) => ({ ...f, [key]: value }));
  };
  const notice = profile.noticeValue.trim() === "" ? "" : noticeKey(profile.noticeValue.trim(), profile.noticeUnit);

  return (
    <form className="panel form-grid" aria-label="Screening call" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h3 className="full">Screening call</h3>
      {canNote && (
        <div className="full">
          <Field label="Notes" error={errors.body ?? errors.notes}>
            <textarea rows={8} value={notes} placeholder="What they said: motivation, availability, salary talk…"
              onChange={(e) => { setSaved(null); setNotes(e.target.value); }} />
          </Field>
          {permissions.includes("notes.sensitive:read") && (
            <label className="row"><input type="checkbox" checked={sensitive} onChange={(e) => { setSensitive(e.target.checked); }} /> Sensitive note (only roles that see sensitive notes)</label>
          )}
        </div>
      )}
      <Field label="Notice period" error={errors["noticePeriod.value"] ?? errors["noticePeriod.unit"]}>
        <select value={notice} onChange={(e) => {
          const [value = "", unit = "month"] = e.target.value.split("_");
          set("noticeValue", value);
          set("noticeUnit", unit as CandidateFormState["noticeUnit"]);
        }}>
          <option value="">—</option>
          {fs.options("notice_period", notice).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
        </select>
      </Field>
      <Field label="Nationality" error={errors.nationality}>
        <select value={profile.nationality.toUpperCase()} onChange={(e) => { set("nationality", e.target.value); }}>
          <option value="">—</option>
          {[...new Set([...COUNTRIES, ...(profile.nationality ? [profile.nationality.toUpperCase()] : [])])].map((c) => (
            <option key={c} value={c}>{regionName(c) ?? c}</option>
          ))}
        </select>
      </Field>
      <Field label="Work authorization" error={errors["workAuthorization.status"]}>
        <select value={profile.waStatus} onChange={(e) => { set("waStatus", e.target.value as CandidateFormState["waStatus"]); }}>
          <option value="">—</option>
          {keysOf(WORK_AUTH_STATUS).map((s) => <option key={s} value={s}>{WORK_AUTH_STATUS[s]}</option>)}
        </select>
      </Field>
      {profile.waStatus === "expat_eligible" && (
        <Field label="Pass held" error={errors["workAuthorization.basis"]}>
          <select value={profile.waBasis} onChange={(e) => { set("waBasis", e.target.value as CandidateFormState["waBasis"]); }}>
            <option value="">Choose…</option>
            {keysOf(WORK_AUTH_BASIS).map((b) => <option key={b} value={b}>{WORK_AUTH_BASIS[b]}</option>)}
          </select>
        </Field>
      )}
      <div className="full field">
        <span>Source</span>
        {base.profile.source ? (
          <p className="muted">{fs.label("candidate_source", base.profile.source)} (kept: they already have a source)</p>
        ) : (
          <SourcePicker value={profile.source} onChange={(v) => { set("source", v); }} label="Source" />
        )}
        {errors.source && <small className="error">{errors.source}</small>}
      </div>
      {salaryShown && <SalaryFields form={salary} set={setSalaryField} errors={errors} />}
      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save the call</button>
        {saved && <span role="status" className="muted">{saved}</span>}
      </div>
      <SaveError
        error={save.error}
        shown={["body", "notes", "nationality", "noticePeriod.value", "noticePeriod.unit", "workAuthorization.status", "workAuthorization.basis", "source", ...Object.values(SALARY_PATHS)]}
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
