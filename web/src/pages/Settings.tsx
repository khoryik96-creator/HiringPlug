import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { fieldSettingsKey, policiesKey } from "../api/keys";
import { SalaryCalculationSettings } from "../components/SalaryCalculationSettings";
import { ErrorMessage } from "../components/ErrorMessage";
import { chipStyle } from "../components/companyCategory";
import { fieldErrors } from "../components/forms";
import { NOTICE_UNIT, keysOf } from "../labels";

type List = Schemas["FieldList"];
type Row = { key?: string; label: string; hint: string; color: string; hidden: boolean; builtIn: boolean };

const rowOf = (o: Schemas["FieldOption"]): Row => ({ ...o, hint: o.hint ?? "", color: o.color ?? "#6b7280" });

/** General settings: the company's mandatory fields, salary in emails and dropdown options. */
export function Settings() {
  const settings = useQuery({ queryKey: fieldSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/settings/fields")) });
  return (
    <main>
      <h2>General</h2>
      <ErrorMessage error={settings.error} />
      {settings.data && (
        <>
          <AgencyName />
          <MandatoryFields fields={settings.data.mandatory} />
          <Policies />
          <SalaryCalculationSettings />
          <p><a href="#/industries">Manage industries &amp; subindustries</a></p>
          <h3>Dropdowns</h3>
          {settings.data.lists.filter((l) => !l.hints && !l.colors && l.name !== "industry").map((l) => <ListEditor key={l.name} list={l} />)}
          <h3 id="company-categories">Company categories</h3>
          <p className="muted">
            The categories a company can be in (signed client, prospect, VIP…), each with its colour: the colour of its card on the
            company&apos;s page and its chip in the list. Rename them, add your own, choose the colours.
          </p>
          {settings.data.lists.filter((l) => l.name === "company_category").map((l) => <ListEditor key={l.name} list={l} />)}
          <h3 id="contact-roles">Contact roles</h3>
          <p className="muted">
            What a contact does in hiring (hiring manager, HR…), each with its colour: the colour of their card and their chip in
            the contact list, where you can filter by it. Rename them, add your own, choose the colours.
          </p>
          {settings.data.lists.filter((l) => l.name === "contact_role").map((l) => <ListEditor key={l.name} list={l} />)}
          <h3 id="benefits-template">Company benefits template</h3>
          <p className="muted">
            What to record for each client on its page: its benefits, and the steps it takes before an offer. Rename, reorder,
            hide or add items; the hint under each says what to write.
          </p>
          {settings.data.lists.filter((l) => l.hints).map((l) => <ListEditor key={l.name} list={l} />)}
        </>
      )}
    </main>
  );
}

const SALARY_CHOICES: Record<Schemas["EmailSalaryVisibility"], string> = {
  salary_roles: "Only roles that may see salaries (recommended)",
  everyone: "Everyone who may read emails",
};

/** The agency's name: on formatted CVs and their file names ("Hyppies CV - Siti Aisyah.docx"). */
function AgencyName() {
  const queryClient = useQueryClient();
  const me = useQuery({ queryKey: ["me"], queryFn: async () => unwrap(await api.GET("/v1/me")) });
  const [name, setName] = useState<string | null>(null);
  const current = name ?? me.data?.tenant.name ?? "";
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/agency", { body: { name: current } })),
    onSuccess: async () => { setName(null); await queryClient.invalidateQueries({ queryKey: ["me"] }); },
  });
  const errors = fieldErrors(save.error);
  const example = current.trim() ? `${current.trim()} CV - Siti Aisyah.docx` : "CV - Siti Aisyah.docx";
  return (
    <section className="panel" aria-label="Agency">
      <h3>Agency</h3>
      <form className="row" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
        <label className="field">Agency name
          <input required value={current} maxLength={200} onChange={(e) => { setName(e.target.value); }} />
        </label>
        <button type="submit" disabled={save.isPending || name === null}>Save</button>
        {save.isSuccess && <span role="status" className="muted">Saved.</span>}
      </form>
      {errors.name && <small className="error">{errors.name}</small>}
      <p className="muted small">On formatted CVs and their file names, e.g. “{example}”, and on the careers page.</p>
      <ErrorMessage error={me.error ?? (errors.name ? null : save.error)} />
    </section>
  );
}

/** Who sees a sent email that shows salary. */
function Policies() {
  const queryClient = useQueryClient();
  const policies = useQuery({ queryKey: policiesKey, queryFn: async () => unwrap(await api.GET("/v1/settings/policies")) });
  const save = useMutation({
    mutationFn: async (v: Schemas["EmailSalaryVisibility"]) => unwrap(await api.PUT("/v1/settings/policies", { body: { emailSalaryVisibility: v } })),
    onSuccess: (p) => { queryClient.setQueryData(policiesKey, p); },
  });
  return (
    <section className="panel" aria-label="Salary in emails">
      <h3>Salary in emails</h3>
      <p className="muted">Who sees sent emails that show salary?</p>
      <ErrorMessage error={policies.error ?? save.error} />
      {policies.data && (
        <select aria-label="Who sees emails that show salary" value={policies.data.emailSalaryVisibility} disabled={save.isPending}
          onChange={(e) => { save.mutate(e.target.value as Schemas["EmailSalaryVisibility"]); }}>
          {keysOf(SALARY_CHOICES).map((k) => <option key={k} value={k}>{SALARY_CHOICES[k]}</option>)}
        </select>
      )}
      {save.isSuccess && <span role="status" className="muted"> Saved: emails sent from now on follow it.</span>}
    </section>
  );
}

function MandatoryFields({ fields }: { fields: Schemas["MandatoryField"][] }) {
  const queryClient = useQueryClient();
  const [chosen, setChosen] = useState(() => Object.fromEntries(fields.map((f) => [`${f.form}/${f.field}`, f.mandatory])));
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/settings/fields/mandatory", {
      body: { fields: fields.map((f) => ({ form: f.form, field: f.field, mandatory: chosen[`${f.form}/${f.field}`] ?? false })) },
    })),
    onSuccess: (data) => { queryClient.setQueryData(fieldSettingsKey, data); },
  });
  const group = (form: Schemas["FieldForm"], title: string) => (
    <fieldset>
      <legend>{title}</legend>
      {fields.filter((f) => f.form === form).map((f) => {
        const key = `${f.form}/${f.field}`;
        return (
          <label key={key} className="check">
            <input type="checkbox" checked={chosen[key] ?? false} disabled={f.alwaysRequired}
              onChange={(e) => { setChosen({ ...chosen, [key]: e.target.checked }); }} />
            {f.label}{f.alwaysRequired && <span className="muted"> (always)</span>}
          </label>
        );
      })}
    </fieldset>
  );
  return (
    <section className="panel" aria-label="Mandatory fields">
      <h3>Mandatory fields</h3>
      <div className="row">{group("candidate", "Candidates")}{group("job", "Jobs")}</div>
      <button type="button" disabled={save.isPending} onClick={() => { save.mutate(); }}>Save mandatory fields</button>
      {save.isSuccess && <span className="muted" role="status"> Saved.</span>}
      <ErrorMessage error={save.error} />
    </section>
  );
}

function ListEditor({ list }: { list: List }) {
  const queryClient = useQueryClient();
  const [rows, setRows] = useState<Row[]>(() => list.options.map(rowOf));
  const [adding, setAdding] = useState({ label: "", hint: "", color: "#2563eb", value: "1", unit: "month" });
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/settings/fields/lists/{fieldList}", {
      params: { path: { fieldList: list.name } },
      body: { options: rows.map((r) => ({ ...(r.key ? { key: r.key } : {}), label: r.label, hidden: r.hidden, ...(list.hints ? { hint: r.hint } : {}),
        ...(list.colors ? { color: r.color } : {}) })) },
    })),
    onSuccess: (saved) => {
      setRows(saved.options.map(rowOf));
      void queryClient.invalidateQueries({ queryKey: fieldSettingsKey });
    },
  });
  const errors = fieldErrors(save.error);
  const update = (i: number, change: Partial<Row>) => { setRows(rows.map((r, j) => (j === i ? { ...r, ...change } : r))); };
  const move = (i: number, by: number) => {
    const next = [...rows];
    const [r] = next.splice(i, 1);
    if (r) next.splice(i + by, 0, r);
    setRows(next);
  };
  const notice = list.name === "notice_period";
  const add = () => {
    if (notice) {
      const n = Number(adding.value);
      const unit = NOTICE_UNIT[adding.unit as Schemas["NoticePeriodUnit"]];
      setRows([...rows, { key: `${String(n)}_${adding.unit}`, label: n === 0 ? "Immediately" : `${String(n)} ${n === 1 ? unit.one : unit.many}`, hint: "", color: "", hidden: false, builtIn: false }]);
    } else if (adding.label.trim()) {
      setRows([...rows, { label: adding.label.trim(), hint: adding.hint.trim(), color: adding.color, hidden: false, builtIn: false }]);
    }
    setAdding({ ...adding, label: "", hint: "" });
  };
  return (
    <section className="panel" aria-label={list.label}>
      <h4>{list.label}</h4>
      <table>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.key ?? `new-${String(i)}`}>
              <td>
                {list.colors && (
                  <input type="color" className="color-input" aria-label={`Colour of ${r.label || `option ${String(i + 1)}`}`} value={r.color}
                    onChange={(e) => { update(i, { color: e.target.value }); }} />
                )}
                <input aria-label={`${list.label} option ${String(i + 1)}`} value={r.label} onChange={(e) => { update(i, { label: e.target.value }); }} />
                {list.colors && <span className="category-chip" style={chipStyle(r.color)}>{r.label || "…"}</span>}
                {list.hints && (
                  <input className="hint-input" aria-label={`Hint for ${r.label || `option ${String(i + 1)}`}`} placeholder="Hint: what to write (optional)"
                    maxLength={300} value={r.hint} onChange={(e) => { update(i, { hint: e.target.value }); }} />
                )}
                {errors[`options[${String(i)}].label`] && <small className="error">{errors[`options[${String(i)}].label`]}</small>}
                {errors[`options[${String(i)}].hint`] && <small className="error">{errors[`options[${String(i)}].hint`]}</small>}
                {errors[`options[${String(i)}].key`] && <small className="error">{errors[`options[${String(i)}].key`]}</small>}
                {errors[`options[${String(i)}].color`] && <small className="error">{errors[`options[${String(i)}].color`]}</small>}
              </td>
              <td>
                <label className="check"><input type="checkbox" checked={r.hidden} onChange={(e) => { update(i, { hidden: e.target.checked }); }} /> Hidden</label>
              </td>
              <td>
                <span className="row">
                <button type="button" className="link" aria-label={`Move ${r.label} up`} disabled={i === 0} onClick={() => { move(i, -1); }}>↑</button>
                <button type="button" className="link" aria-label={`Move ${r.label} down`} disabled={i === rows.length - 1} onClick={() => { move(i, 1); }}>↓</button>
                {list.kind === "open" && !r.builtIn && (
                  <button type="button" className="link" onClick={() => { setRows(rows.filter((_, j) => j !== i)); }}>Remove</button>
                )}
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {list.kind === "open" && (
        <div className="row">
          {notice ? (
            <>
              <input aria-label="Number" type="number" min={0} max={365} value={adding.value} onChange={(e) => { setAdding({ ...adding, value: e.target.value }); }} />
              <select aria-label="Unit" value={adding.unit} onChange={(e) => { setAdding({ ...adding, unit: e.target.value }); }}>
                {keysOf(NOTICE_UNIT).map((u) => <option key={u} value={u}>{NOTICE_UNIT[u].many}</option>)}
              </select>
            </>
          ) : (
            <>
              <input aria-label={`New ${list.label.toLowerCase()} option`} placeholder={list.hints ? "New item" : list.name === "contact_role" ? "New role" : list.colors ? "New category" : "New option"} value={adding.label}
                onChange={(e) => { setAdding({ ...adding, label: e.target.value }); }} />
              {list.colors && (
                <input type="color" className="color-input" aria-label="Colour of the new category" value={adding.color}
                  onChange={(e) => { setAdding({ ...adding, color: e.target.value }); }} />
              )}
              {list.hints && (
                <input aria-label={`Hint for the new ${list.label.toLowerCase()} item`} placeholder="Hint (optional)" maxLength={300} value={adding.hint}
                  onChange={(e) => { setAdding({ ...adding, hint: e.target.value }); }} />
              )}
            </>
          )}
          <button type="button" className="link" onClick={add}>Add</button>
        </div>
      )}
      {errors.options && <p className="error">{errors.options}</p>}
      <button type="button" disabled={save.isPending} onClick={() => { save.mutate(); }}>Save {list.label.toLowerCase()}</button>
      {save.isSuccess && <span className="muted" role="status"> Saved.</span>}
      <ErrorMessage error={save.error} />
    </section>
  );
}
