import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useState } from "react";

import { api, type Company, unwrap } from "../api/client";
import { companiesKey, companyKey } from "../api/keys";
import { COMPANY_SIZE, COMPANY_TYPE, keysOf, regionName } from "../labels";
import { COMPANY_PATHS, companyFields, type CompanyFormState, companyProblems, formFromCompany } from "./companyFormState";
import { IndustryPicker } from "./IndustryPicker";
import { Field } from "./Field";
import { COUNTRIES, fieldErrors, FormInvalid, sameBody } from "./forms";
import { SaveError } from "./SaveError";

type TextKey = "name" | "website" | "linkedinUrl" | "industry" | "phone" | "city" | "country" | "registrationNumber";

/**
 * Add a company (company = null) or edit one. Saving a new company calls
 * onSaved with it; an edit saves with the version it was opened with, so a
 * newer change by someone else comes back as a conflict.
 */
export function CompanyForm({ company, onDone, onSaved }: {
  company: Company | null;
  onDone: () => void;
  onSaved?: (company: Company) => void;
}) {
  const queryClient = useQueryClient();
  const [opened] = useState(() => ({ id: company?.id, version: company?.version ?? 0, form: formFromCompany(company) }));
  const [form, setForm] = useState(opened.form);
  const save = useMutation({
    mutationFn: async () => {
      const problems = companyProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const fields = companyFields(form);
      if (!opened.id) return unwrap(await api.POST("/v1/companies", { body: fields }));
      if (sameBody(fields, companyFields(opened.form))) return null; // nothing changed
      return unwrap(await api.PATCH("/v1/companies/{companyId}", {
        params: { path: { companyId: opened.id } }, body: { ...fields, version: opened.version },
      }));
    },
    onSuccess: (saved) => {
      if (saved) {
        queryClient.setQueryData(companyKey(saved.id), saved);
        void queryClient.invalidateQueries({ queryKey: companiesKey });
        onSaved?.(saved);
      }
      onDone();
    },
  });
  const errors = fieldErrors(save.error);
  const set = <K extends keyof CompanyFormState>(key: K, value: CompanyFormState[K]) => {
    setForm((f) => ({ ...f, [key]: value }));
  };
  const text = (key: TextKey, label: string, extra: InputHTMLAttributes<HTMLInputElement> = {}) => (
    <Field label={label} error={errors[COMPANY_PATHS[key]]}>
      <input value={form[key]} onChange={(e) => { set(key, e.target.value); }} {...extra} />
    </Field>
  );
  const area = (key: "address" | "about", label: string) => (
    <div className="full">
      <Field label={label} error={errors[COMPANY_PATHS[key]]}>
        <textarea rows={key === "about" ? 4 : 2} value={form[key]} onChange={(e) => { set(key, e.target.value); }} />
      </Field>
    </div>
  );

  return (
    <form
      className="panel form-grid"
      aria-label={opened.id ? "Edit company" : "Add company"}
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      {text("name", "Name")}
      <Field label="Type" error={errors[COMPANY_PATHS.type]}>
        <select value={form.type} onChange={(e) => { set("type", e.target.value as CompanyFormState["type"]); }}>
          {keysOf(COMPANY_TYPE).map((t) => <option key={t} value={t}>{COMPANY_TYPE[t]}</option>)}
        </select>
      </Field>
      {text("website", "Website", { placeholder: "example.com", inputMode: "url" })}
      {text("linkedinUrl", "LinkedIn page", { placeholder: "linkedin.com/company/…", inputMode: "url" })}
      <IndustryPicker value={form.industrySelections} legacy={form.industry} onChange={(industrySelections, industry) => { setForm((f) => ({ ...f, industrySelections, industry })); }} />
      <Field label="Size" error={errors[COMPANY_PATHS.size]}>
        <select value={form.size} onChange={(e) => { set("size", e.target.value as CompanyFormState["size"]); }}>
          <option value="">—</option>
          {keysOf(COMPANY_SIZE).map((s) => <option key={s} value={s}>{COMPANY_SIZE[s]}</option>)}
        </select>
      </Field>
      {text("phone", "Phone")}
      {text("city", "City")}
      {text("country", "Country (2-letter code)", { list: "company-countries", placeholder: "e.g. MY", className: "code" })}
      <datalist id="company-countries">
        {COUNTRIES.map((c) => <option key={c} value={c}>{regionName(c)}</option>)}
      </datalist>
      {text("registrationNumber", "Registration no. (e.g. SSM)")}
      {area("address", "Address")}
      {area("about", "About")}

      <div className="row full">
        <button type="submit" disabled={save.isPending}>{opened.id ? "Save" : "Add company"}</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <SaveError
        error={save.error}
        shown={Object.values(COMPANY_PATHS)}
        onReload={() => {
          if (opened.id) void queryClient.invalidateQueries({ queryKey: companyKey(opened.id) });
          onDone();
        }}
      />
    </form>
  );
}
