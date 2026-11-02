import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useState } from "react";

import { api, type JobOrder, unwrap } from "../api/client";
import { contactsKey, jobKey, jobsKey } from "../api/keys";
import { JOB_STATUS, keysOf, WORK_ARRANGEMENT, WORK_TYPE } from "../labels";
import { CompanyPicker, type PickedCompany } from "./CompanyPicker";
import { Field } from "./Field";
import { fieldErrors, FormInvalid, sameBody } from "./forms";
import { formFromJob, JOB_PATHS, jobFields, type JobFormState, jobProblems } from "./jobFormState";
import { SaveError } from "./SaveError";

/**
 * Add a job (job = null, optionally at `company`) or edit one. Edits save
 * with the version they were opened with. The salary budget is separate.
 */
export function JobForm({ job, company = null, onDone, onSaved }: {
  job: JobOrder | null;
  company?: PickedCompany | null;
  onDone: () => void;
  onSaved?: (job: JobOrder) => void;
}) {
  const queryClient = useQueryClient();
  const [opened] = useState(() => ({ id: job?.id, version: job?.version ?? 0, form: formFromJob(job, company) }));
  const [form, setForm] = useState(opened.form);
  const companyId = form.company?.id;
  const contacts = useQuery({
    queryKey: [...contactsKey, "company", companyId],
    queryFn: async () =>
      unwrap(await api.GET("/v1/contacts", { params: { query: { companyId: companyId ?? "", limit: 100 } } })).items,
    enabled: companyId !== undefined,
  });
  const save = useMutation({
    mutationFn: async () => {
      const problems = jobProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const fields = jobFields(form);
      if (!opened.id) return unwrap(await api.POST("/v1/jobs", { body: fields }));
      if (sameBody(fields, jobFields(opened.form))) return null; // nothing changed
      return unwrap(await api.PATCH("/v1/jobs/{jobId}", {
        params: { path: { jobId: opened.id } }, body: { ...fields, version: opened.version },
      }));
    },
    onSuccess: (saved) => {
      if (saved) {
        queryClient.setQueryData(jobKey(saved.id), saved);
        void queryClient.invalidateQueries({ queryKey: jobsKey });
        onSaved?.(saved);
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof JobFormState>(key: K, value: JobFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const text = (key: "title" | "location" | "openings", label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
    <Field label={label} error={errors[JOB_PATHS[key]]}>
      <input value={form[key]} onChange={(e) => { set(key, e.target.value); }} {...extra} />
    </Field>
  );

  return (
    <form
      className="panel form-grid"
      aria-label={opened.id ? "Edit job" : "Add job"}
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      {text("title", "Job title")}
      <Field label="Company" error={errors[JOB_PATHS.company]}>
        <CompanyPicker value={form.company} onChange={(c) => { setForm((f) => ({ ...f, company: c, contactId: "" })); }} />
      </Field>
      <Field label="Hiring contact" error={errors[JOB_PATHS.contactId]}>
        <select value={form.contactId} disabled={!companyId} onChange={(e) => { set("contactId", e.target.value); }}>
          <option value="">—</option>
          {contacts.data?.map((c) => <option key={c.id} value={c.id}>{c.fullName}{c.title ? `, ${c.title}` : ""}</option>)}
        </select>
      </Field>
      <Field label="Status" error={errors[JOB_PATHS.status]}>
        <select value={form.status} onChange={(e) => { set("status", e.target.value as JobFormState["status"]); }}>
          {keysOf(JOB_STATUS).map((s) => <option key={s} value={s}>{JOB_STATUS[s]}</option>)}
        </select>
      </Field>
      {text("location", "Location")}
      <Field label="Work type" error={errors[JOB_PATHS.workType]}>
        <select value={form.workType} onChange={(e) => { set("workType", e.target.value as JobFormState["workType"]); }}>
          <option value="">—</option>
          {keysOf(WORK_TYPE).map((w) => <option key={w} value={w}>{WORK_TYPE[w]}</option>)}
        </select>
      </Field>
      <Field label="Where" error={errors[JOB_PATHS.workArrangement]}>
        <select value={form.workArrangement} onChange={(e) => { set("workArrangement", e.target.value as JobFormState["workArrangement"]); }}>
          <option value="">—</option>
          {keysOf(WORK_ARRANGEMENT).map((w) => <option key={w} value={w}>{WORK_ARRANGEMENT[w]}</option>)}
        </select>
      </Field>
      {text("openings", "Openings", { inputMode: "numeric" })}
      <div className="full">
        <Field label="Description" error={errors[JOB_PATHS.description]}>
          <textarea rows={8} value={form.description} onChange={(e) => { set("description", e.target.value); }} />
        </Field>
      </div>

      <div className="row full">
        <button type="submit" disabled={save.isPending}>{opened.id ? "Save" : "Add job"}</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(JOB_PATHS)}
        onReload={() => {
          if (opened.id) void queryClient.invalidateQueries({ queryKey: jobKey(opened.id) });
          onDone();
        }}
      />
    </form>
  );
}
