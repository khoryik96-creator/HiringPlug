import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useState } from "react";

import { api, type Candidate, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";
import { CANDIDATE_SOURCE, keysOf, NOTICE_UNIT, regionName, WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";
import {
  type CandidateFormState,
  candidateProblems,
  FIELD_PATHS,
  formFromCandidate,
  toUpdateBody,
} from "./candidateFormState";
import { Field } from "./Field";
import { COUNTRIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import { SaveError } from "./SaveError";

type TextKey = "firstName" | "lastName" | "email" | "phone" | "nationality" | "waNote" | "industry"
  | "specialization" | "currentTitle" | "currentCompany" | "location";

/** Edit a candidate's profile. Salary is edited separately (its own permission). */
export function CandidateForm({ candidate, onDone }: { candidate: Candidate; onDone: () => void }) {
  const queryClient = useQueryClient();
  // What the user started from. Saving sends this version even if the page
  // refreshes in the meantime, so a newer change by someone else comes back as
  // a conflict instead of being silently overwritten.
  const [opened] = useState(() => ({ id: candidate.id, version: candidate.version, form: formFromCandidate(candidate) }));
  const [form, setForm] = useState(opened.form);
  const save = useMutation({
    mutationFn: async () => {
      const problems = candidateProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const body = toUpdateBody(form, opened.version);
      if (sameBody(body, toUpdateBody(opened.form, opened.version))) return null; // nothing changed
      return unwrap(await api.PATCH("/v1/candidates/{candidateId}", { params: { path: { candidateId: opened.id } }, body }));
    },
    onSuccess: (updated) => {
      if (updated) {
        queryClient.setQueryData(candidateKey(opened.id), updated);
        void queryClient.invalidateQueries({ queryKey: candidatesKey });
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const errorFor = (key: keyof CandidateFormState) => errors[FIELD_PATHS[key]];
  const set = <K extends keyof CandidateFormState>(key: K, value: CandidateFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const text = (key: TextKey, label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
    <Field label={label} error={errorFor(key)}>
      <input value={form[key]} onChange={(e) => { set(key, e.target.value); }} {...extra} />
    </Field>
  );

  return (
    <form
      className="panel form-grid"
      aria-label="Edit profile"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      {text("firstName", "First name")}
      {text("lastName", "Last name")}
      {/* Not type="email": the browser's rule refuses addresses the API accepts (e.g. josé@…). */}
      {text("email", "Email", { inputMode: "email", autoComplete: "off", spellCheck: false })}
      {text("phone", "Phone", { placeholder: "+60 12 345 6789" })}
      {text("nationality", "Nationality (2-letter code)", { list: "countries", placeholder: "e.g. MY", className: "code" })}
      <datalist id="countries">
        {COUNTRIES.map((c) => <option key={c} value={c}>{regionName(c)}</option>)}
      </datalist>

      <Field label="Work authorization" error={errorFor("waStatus")}>
        <select value={form.waStatus} onChange={(e) => { set("waStatus", e.target.value as CandidateFormState["waStatus"]); }}>
          <option value="">—</option>
          {keysOf(WORK_AUTH_STATUS).map((s) => <option key={s} value={s}>{WORK_AUTH_STATUS[s]}</option>)}
        </select>
      </Field>
      {form.waStatus === "expat_eligible" && (
        <Field label="Pass held" error={errorFor("waBasis")}>
          <select value={form.waBasis} onChange={(e) => { set("waBasis", e.target.value as CandidateFormState["waBasis"]); }}>
            <option value="">Choose…</option>
            {keysOf(WORK_AUTH_BASIS).map((b) => <option key={b} value={b}>{WORK_AUTH_BASIS[b]}</option>)}
          </select>
        </Field>
      )}
      {form.waStatus !== "" && text("waNote", "Work authorization note")}

      <Field label="Notice period (0 = immediate)" error={errorFor("noticeValue")}>
        <input type="number" min={0} max={365} value={form.noticeValue}
          onChange={(e) => { set("noticeValue", e.target.value); }} />
      </Field>
      <Field label="Notice period unit" error={errorFor("noticeUnit")}>
        <select value={form.noticeUnit} onChange={(e) => { set("noticeUnit", e.target.value as CandidateFormState["noticeUnit"]); }}>
          {keysOf(NOTICE_UNIT).map((u) => <option key={u} value={u}>{NOTICE_UNIT[u].many}</option>)}
        </select>
      </Field>
      {text("industry", "Industry")}
      {text("specialization", "Specialization")}
      {text("currentTitle", "Current title")}
      {text("currentCompany", "Current company")}
      {text("location", "Location")}
      <Field label="Source" error={errorFor("source")}>
        <select value={form.source} onChange={(e) => { set("source", e.target.value as CandidateFormState["source"]); }}>
          <option value="">—</option>
          {keysOf(CANDIDATE_SOURCE).map((s) => <option key={s} value={s}>{CANDIDATE_SOURCE[s]}</option>)}
        </select>
      </Field>

      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(FIELD_PATHS)}
        onReload={() => { void queryClient.invalidateQueries({ queryKey: candidateKey(opened.id) }); onDone(); }}
      />
    </form>
  );
}
