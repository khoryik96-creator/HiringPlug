import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { fieldSettingsKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { fieldErrors } from "../components/forms";
import { NOTICE_UNIT, keysOf } from "../labels";

type List = Schemas["FieldList"];
type Row = { key?: string; label: string; hidden: boolean; builtIn: boolean };

/** Main settings: the company's dropdown options and mandatory fields. */
export function Settings() {
  const settings = useQuery({ queryKey: fieldSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/settings/fields")) });
  return (
    <main>
      <h2>Settings</h2>
      <ErrorMessage error={settings.error} />
      {settings.data && (
        <>
          <MandatoryFields fields={settings.data.mandatory} />
          <h3>Dropdowns</h3>
          <p className="muted">
            Hidden options are no longer offered, but records that have them keep them. Options that come with the app can be
            hidden but not removed. Work type and arrangement can only be renamed, reordered or hidden: job boards need their values.
          </p>
          {settings.data.lists.map((l) => <ListEditor key={l.name} list={l} />)}
        </>
      )}
    </main>
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
      <p className="muted">People must fill these in when they add a record (CSV imports too), and cannot empty them later. Older records stay editable.</p>
      <div className="row">{group("candidate", "Candidates")}{group("job", "Jobs")}</div>
      <button type="button" disabled={save.isPending} onClick={() => { save.mutate(); }}>Save mandatory fields</button>
      {save.isSuccess && <span className="muted" role="status"> Saved.</span>}
      <ErrorMessage error={save.error} />
    </section>
  );
}

function ListEditor({ list }: { list: List }) {
  const queryClient = useQueryClient();
  const [rows, setRows] = useState<Row[]>(() => list.options.map((o) => ({ ...o })));
  const [adding, setAdding] = useState({ label: "", value: "1", unit: "month" });
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/settings/fields/lists/{fieldList}", {
      params: { path: { fieldList: list.name } },
      body: { options: rows.map((r) => ({ ...(r.key ? { key: r.key } : {}), label: r.label, hidden: r.hidden })) },
    })),
    onSuccess: (saved) => {
      setRows(saved.options.map((o) => ({ ...o })));
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
      setRows([...rows, { key: `${String(n)}_${adding.unit}`, label: n === 0 ? "Immediately" : `${String(n)} ${n === 1 ? unit.one : unit.many}`, hidden: false, builtIn: false }]);
    } else if (adding.label.trim()) {
      setRows([...rows, { label: adding.label.trim(), hidden: false, builtIn: false }]);
    }
    setAdding({ ...adding, label: "" });
  };
  return (
    <section className="panel" aria-label={list.label}>
      <h4>{list.label}</h4>
      <table>
        <tbody>
          {rows.map((r, i) => (
            <tr key={r.key ?? `new-${String(i)}`}>
              <td>
                <input aria-label={`${list.label} option ${String(i + 1)}`} value={r.label} onChange={(e) => { update(i, { label: e.target.value }); }} />
                {errors[`options[${String(i)}].label`] && <small className="error">{errors[`options[${String(i)}].label`]}</small>}
                {errors[`options[${String(i)}].key`] && <small className="error">{errors[`options[${String(i)}].key`]}</small>}
              </td>
              <td>
                <label className="check"><input type="checkbox" checked={r.hidden} onChange={(e) => { update(i, { hidden: e.target.checked }); }} /> Hidden</label>
              </td>
              <td className="row">
                <button type="button" className="link" aria-label={`Move ${r.label} up`} disabled={i === 0} onClick={() => { move(i, -1); }}>↑</button>
                <button type="button" className="link" aria-label={`Move ${r.label} down`} disabled={i === rows.length - 1} onClick={() => { move(i, 1); }}>↓</button>
                {list.kind === "open" && !r.builtIn && (
                  <button type="button" className="link" onClick={() => { setRows(rows.filter((_, j) => j !== i)); }}>Remove</button>
                )}
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
            <input aria-label={`New ${list.label.toLowerCase()} option`} placeholder="New option" value={adding.label}
              onChange={(e) => { setAdding({ ...adding, label: e.target.value }); }} />
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
