import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useState } from "react";

import { api, type Candidate, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";
import { keysOf, regionName, WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";
import {
  type CandidateFormState,
  candidateProblems,
  FIELD_PATHS,
  formFromCandidate,
  toUpdateBody,
} from "./candidateFormState";
import { Field } from "./Field";
import { LabelInput } from "./LabelInput";
import { labelFor, noticeKey, useFieldSettings } from "./useFieldSettings";
import { useUsedValues } from "./useUsedValues";
import { COUNTRIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import { DuplicateEmail } from "./DuplicateEmail";
import { SaveError } from "./SaveError";
import { SourcePicker } from "./SourcePicker";

type TextKey = "firstName" | "lastName" | "email" | "phone" | "linkedinUrl" | "nationality" | "waNote" | "industry"
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
  const used = useUsedValues();
  const fs = useFieldSettings();
  const req = (label: string, field: string) => labelFor(label, fs.mandatory("candidate", field));
  const errors = fieldErrors(save.error);
  const errorFor = (key: keyof CandidateFormState) => errors[FIELD_PATHS[key]];
  const set = <K extends keyof CandidateFormState>(key: K, value: CandidateFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const notice = form.noticeValue.trim() === "" ? "" : noticeKey(form.noticeValue.trim(), form.noticeUnit);
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
      {text("firstName", req("First name", "firstName"))}
      {text("lastName", "Last name")}
      {/* Not type="email": the browser's rule refuses addresses the API accepts (e.g. josé@…). */}
      {text("email", req("Email", "email"), { inputMode: "email", autoComplete: "off", spellCheck: false })}
      {text("phone", req("Phone", "phone"), { placeholder: "+60 12 345 6789" })}
      {text("linkedinUrl", req("LinkedIn profile", "linkedinUrl"), { placeholder: "linkedin.com/in/…", inputMode: "url", spellCheck: false })}
      {text("nationality", req("Nationality (2-letter code)", "nationality"), { list: "countries", placeholder: "e.g. MY", className: "code" })}
      <datalist id="countries">
        {COUNTRIES.map((c) => <option key={c} value={c}>{regionName(c)}</option>)}
      </datalist>

      <Field label={req("Country of residence", "residenceCountry")} error={errorFor("residenceCountry")}>
        <select value={form.residenceCountry} onChange={(e) => { set("residenceCountry", e.target.value); }}>
          <option value="">NA (not known)</option>
          {[...new Set([...COUNTRIES, ...(form.residenceCountry ? [form.residenceCountry] : [])])].map((c) => (
            <option key={c} value={c}>{regionName(c) ?? c}</option>
          ))}
        </select>
      </Field>
      <Field label={req("Work authorization", "workAuthorization")} error={errorFor("waStatus")}>
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

      <Field label={req("Notice period", "noticePeriod")} error={errorFor("noticeValue") ?? errorFor("noticeUnit")}>
        <select value={notice} onChange={(e) => {
          const [value = "", unit = "month"] = e.target.value.split("_");
          setForm((f) => ({ ...f, noticeValue: value, noticeUnit: unit as CandidateFormState["noticeUnit"] }));
        }}>
          <option value="">—</option>
          {fs.options("notice_period", notice).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
        </select>
      </Field>
      <Field label={req("Industry", "industry")} error={errorFor("industry")}>
        <select value={form.industry} onChange={(e) => { set("industry", e.target.value); }}>
          <option value="">—</option>
          {fs.options("industry", form.industry).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
        </select>
      </Field>
      {text("specialization", "Specialization", { list: "specializations" })}
      <datalist id="specializations">{used.specializations.map((v) => <option key={v} value={v} />)}</datalist>
      {text("currentTitle", req("Current title", "currentTitle"))}
      {text("currentCompany", req("Current company", "currentCompany"))}
      {text("location", req("Location", "location"))}
      <div className="full field">
        <span>{req("Source", "source")}</span>
        <SourcePicker value={form.source} onChange={(v) => { set("source", v); }} label={req("Source", "source")} />
        {errorFor("source") && <small className="error">{errorFor("source")}</small>}
      </div>

      <div className="full">
        <Field label="Skills" error={errorFor("skills")}>
          <LabelInput value={form.skills} onChange={(v) => { set("skills", v); }} suggestions={used.skills} placeholder="Type a skill, then Enter" />
        </Field>
      </div>
      <div className="full">
        <Field label="Tags" error={errorFor("tags")}>
          <LabelInput value={form.tags} onChange={(v) => { set("tags", v); }} suggestions={used.tags} placeholder="e.g. relocate, then Enter" />
        </Field>
      </div>

      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(FIELD_PATHS)}
        onReload={() => { void queryClient.invalidateQueries({ queryKey: candidateKey(opened.id) }); onDone(); }}
      />
      <div className="full"><DuplicateEmail error={save.error} /></div>
    </form>
  );
}
