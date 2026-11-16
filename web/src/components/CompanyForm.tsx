import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type InputHTMLAttributes, useRef, useState } from "react";

import { api, ApiError, type Company, unwrap } from "../api/client";
import { companiesKey, companyKey } from "../api/keys";
import { COMPANY_SIZE, COMPANY_TYPE, keysOf } from "../labels";
import { COMPANY_PATHS, companyFields, type CompanyFormState, companyProblems, formFromCompany } from "./companyFormState";
import { companionCall, operationId, useCompanion } from "../companion";
import { guessCompanyName } from "./companyNameGuess";
import { companionCan, companyPage, type LinkedinAbout, sizeFromLinkedin } from "./linkedinCompany";
import { IndustryPicker } from "./IndustryPicker";
import { Field } from "./Field";
import { CountryList, useFavourites } from "./favourites";
import { fieldErrors, FormInvalid, sameBody } from "./forms";
import { SaveError } from "./SaveError";

type TextKey = "name" | "website" | "linkedinUrl" | "industry" | "phone" | "city" | "country" | "registrationNumber";

/**
 * Add a company (company = null) or edit one. Saving a new company calls
 * onSaved with it; an edit saves with the version it was opened with, so a
 * newer change by someone else comes back as a conflict.
 */
/** A company's fields but its industry (left as saved). */
function withoutIndustry(fields: ReturnType<typeof companyFields>): ReturnType<typeof companyFields> {
  const out = { ...fields };
  delete out.industry;
  delete out.industrySelections;
  return out;
}

export function CompanyForm({ company, onDone, onSaved }: {
  company: Company | null;
  onDone: () => void;
  onSaved?: (company: Company) => void;
}) {
  const queryClient = useQueryClient();
  const [opened] = useState(() => ({ id: company?.id, version: company?.version ?? 0, form: formFromCompany(company) }));
  const [form, setForm] = useState(opened.form);
  // A new company: its name is guessed from its addresses until typed, and
  // saving reads its website (filling what is left empty).
  const isNew = !opened.id;
  const [nameGuessed, setNameGuessed] = useState(false);
  // Or from its LinkedIn page, read by the LinkedIn companion in this browser.
  const companion = useCompanion();
  const favs = useFavourites();
  const linkedinPage = companyPage(form.linkedinUrl);
  const [fromLinkedin, setFromLinkedin] = useState<{ filled: string[]; industry: string } | null>(null);
  const [industryHint, setIndustryHint] = useState("");
  // Its own read only: Stop cancels this operation in the companion (a page
  // timeout alone does not stop it), and a late answer is then ignored.
  const [reading, setReading] = useState<string | null>(null);
  const current = useRef<string | null>(null);
  const readLinkedin = useMutation({
    mutationFn: async () => {
      const op = operationId();
      current.current = op;
      setReading(op);
      try {
        return { op, about: await companionCall<LinkedinAbout>("companyAbout", { company_url: linkedinPage, operation_id: op }) };
      } finally {
        setReading((r) => (r === op ? null : r));
      }
    },
    onSuccess: ({ op, about: a }) => {
      if (current.current !== op) return; // stopped: a late answer is ignored
      const filled: string[] = [];
      const next = { ...form };
      if (a.name && (!form.name.trim() || nameGuessed)) { next.name = a.name; filled.push("name"); }
      const size = sizeFromLinkedin(a.size);
      if (size && !form.size) { next.size = size; filled.push("size"); }
      if (a.website && !form.website.trim()) { next.website = a.website; filled.push("website"); }
      if (a.about && !form.about.trim()) { next.about = a.about; filled.push("about"); }
      // "Shah Alam, Selangor": its city is the first part.
      const city = a.headquarters.split(",")[0]?.trim() ?? "";
      if (city && !form.city.trim()) { next.city = city.slice(0, 100); filled.push("city"); }
      setForm(next);
      // LinkedIn's industry is only a hint: the AI picks the closest of yours once the company is saved.
      setIndustryHint(a.industry);
      if (filled.includes("name")) setNameGuessed(false);
      setFromLinkedin({ filled, industry: a.industry });
    },
  });
  const save = useMutation({
    mutationFn: async () => {
      const problems = companyProblems(form);
      if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
      const fields = companyFields(form);
      if (!opened.id) {
        const fill = {
          ...(form.website.trim() ? { fillFromWebsite: true, nameGuessed } : {}),
          ...(industryHint && !form.industrySelections?.ids.length ? { industryHint } : {}),
        };
        return unwrap(await api.POST("/v1/companies", { body: { ...fields, ...fill } }));
      }
      if (sameBody(fields, companyFields(opened.form))) return null; // nothing changed
      const id = opened.id;
      const patch = async (body: typeof fields, version: number) =>
        unwrap(await api.PATCH("/v1/companies/{companyId}", { params: { path: { companyId: id } }, body: { ...body, version } }));
      try {
        return await patch(fields, opened.version);
      } catch (err) {
        // The AI may have picked the industry since this form opened (a new
        // company): when that is all that changed, and this form leaves the
        // industry as it was, save over it, keeping both.
        if (!(err instanceof ApiError) || err.code !== "CONFLICT_STALE_VERSION") throw err;
        const latest = unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: id } } }));
        const industryUntouched = form.industry === opened.form.industry && sameBody(form.industrySelections ?? null, opened.form.industrySelections ?? null);
        if (!industryUntouched || !sameBody(withoutIndustry(companyFields(opened.form)), withoutIndustry(companyFields(formFromCompany(latest))))) throw err;
        return await patch(withoutIndustry(fields), latest.version);
      }
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
    if (isNew && (key === "website" || key === "linkedinUrl") && (!form.name.trim() || nameGuessed)) {
      const next = { ...form, [key]: value };
      const guess = guessCompanyName(next.website, next.linkedinUrl);
      setForm({ ...next, name: guess });
      setNameGuessed(guess !== "");
      return;
    }
    setForm((f) => ({ ...f, [key]: value }));
    if (key === "name") setNameGuessed(false); // typed: theirs
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
      {isNew && linkedinPage && (
        <div className="full row company-linkedin-fill">
          {companion.ready && !companionCan(companion.version) ? (
            <span className="muted small">Update the HiringPlug LinkedIn companion (version 1.1 or later) to fill this in from LinkedIn: <a href="#/companion">download it again</a>.</span>
          ) : companion.ready ? (
            <>
              <button type="button" className="pill-btn" disabled={readLinkedin.isPending} onClick={() => { readLinkedin.mutate(); }}>
                {readLinkedin.isPending ? "Reading LinkedIn…" : "Fill in from LinkedIn"}
              </button>
              {reading && (
                <button type="button" className="link small" onClick={() => {
                  void companionCall("cancel", { operation_id: reading }, 10_000).catch(() => undefined);
                  current.current = null;
                  setReading(null);
                  readLinkedin.reset();
                }}>Stop</button>
              )}
            </>
          ) : companion.ready === false && (
            <span className="muted small">With the HiringPlug LinkedIn companion in this browser, the name, size and industry fill in from LinkedIn. <a href="#/companion">Set it up</a></span>
          )}
          {readLinkedin.error && <span className="warning small" role="alert">{readLinkedin.error.message}</span>}
          {fromLinkedin && (
            <span className="muted small" role="status">
              {fromLinkedin.filled.length ? `From LinkedIn: ${fromLinkedin.filled.join(", ")}.` : "LinkedIn had nothing to add."}
              {fromLinkedin.industry && !form.industrySelections?.ids.length && ` LinkedIn says “${fromLinkedin.industry}”: once saved, the AI picks the closest of your industries (change it if it is wrong).`}
            </span>
          )}
        </div>
      )}
      {isNew && form.website.trim() && (
        <p className="muted small full company-fill-hint">
          Saving reads the website and fills in what is left empty (phone, address, size…){nameGuessed ? ", and the company’s own name" : ""}.
        </p>
      )}
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
      <CountryList id="company-countries" fav={favs.of("country")} />
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
