import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Candidate, unwrap } from "../api/client";
import { type CandidateFormState, formFromCandidate, toUpdateBody } from "./candidateForm";
import { ErrorMessage } from "./ErrorMessage";
import { Field } from "./Field";
import { COUNTRIES, fieldErrors, SOURCES } from "./forms";

/** Edit a candidate's profile. Salary is edited separately (its own permission). */
export function CandidateForm({ candidate, onDone }: { candidate: Candidate; onDone: () => void }) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CandidateFormState>(() => formFromCandidate(candidate));
  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PATCH("/v1/candidates/{candidateId}", {
          params: { path: { candidateId: candidate.id } },
          body: toUpdateBody(form, candidate.version),
        }),
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(["candidate", candidate.id], updated);
      void queryClient.invalidateQueries({ queryKey: ["candidates"] });
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof CandidateFormState>(key: K, value: CandidateFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const text = (key: keyof CandidateFormState, label: string, extra: Record<string, unknown> = {}) => (
    <Field label={label} error={errors[key]}>
      <input value={form[key]} onChange={(e) => { set(key, e.target.value); }} {...extra} />
    </Field>
  );
  const stale = save.error instanceof ApiError && save.error.code === "CONFLICT_STALE_VERSION";

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
      {text("email", "Email", { type: "email" })}
      {text("phone", "Phone", { placeholder: "+60 12 345 6789" })}
      {text("nationality", "Nationality (country code)", { list: "countries", maxLength: 2 })}
      <datalist id="countries">
        {COUNTRIES.map((c) => <option key={c} value={c} />)}
      </datalist>

      <Field label="Work authorization" error={errors["workAuthorization.status"]}>
        <select value={form.waStatus} onChange={(e) => { set("waStatus", e.target.value as CandidateFormState["waStatus"]); }}>
          <option value="">—</option>
          <option value="local">Local</option>
          <option value="expat_eligible">Expat, can work here</option>
          <option value="requires_visa">Requires a visa</option>
        </select>
      </Field>
      {form.waStatus === "expat_eligible" && (
        <Field label="Pass held" error={errors["workAuthorization.basis"]}>
          <select value={form.waBasis} onChange={(e) => { set("waBasis", e.target.value as CandidateFormState["waBasis"]); }}>
            <option value="">Choose…</option>
            <option value="rp_t">Residence Pass-Talent (RP-T)</option>
            <option value="permanent_resident">Permanent resident</option>
            <option value="spouse_visa">Spouse visa</option>
            <option value="other">Other</option>
          </select>
        </Field>
      )}
      {form.waStatus !== "" && text("waNote", "Work authorization note")}

      <Field label="Notice period (0 = immediate)" error={errors["noticePeriod.value"] ?? errors.noticePeriod}>
        <input type="number" min={0} max={365} value={form.noticeValue}
          onChange={(e) => { set("noticeValue", e.target.value); }} />
      </Field>
      <Field label="Notice period unit">
        <select value={form.noticeUnit} onChange={(e) => { set("noticeUnit", e.target.value as CandidateFormState["noticeUnit"]); }}>
          <option value="day">days</option>
          <option value="week">weeks</option>
          <option value="month">months</option>
        </select>
      </Field>
      {text("industry", "Industry")}
      {text("specialization", "Specialization")}
      {text("currentTitle", "Current title")}
      {text("currentCompany", "Current company")}
      {text("location", "Location")}
      <Field label="Source" error={errors.source}>
        <select value={form.source} onChange={(e) => { set("source", e.target.value as CandidateFormState["source"]); }}>
          <option value="">—</option>
          {SOURCES.map((s) => <option key={s} value={s}>{s.replace("_", " ")}</option>)}
        </select>
      </Field>

      <div className="row full">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      {stale ? (
        <p className="error full">
          Someone else changed this candidate while you were editing.{" "}
          <button type="button" className="link" onClick={() => { void queryClient.invalidateQueries({ queryKey: ["candidate", candidate.id] }); onDone(); }}>
            Reload their version
          </button>
        </p>
      ) : (
        Object.keys(errors).length === 0 && <div className="full"><ErrorMessage error={save.error} /></div>
      )}
    </form>
  );
}
