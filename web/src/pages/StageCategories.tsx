import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { stageCategoriesKey, stageTemplatesKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { fieldErrors } from "../components/forms";
import { byKind, type Category, useStageCategories } from "../components/stages/categories";
import { useCan } from "../components/useCan";
import { OPEN_STAGES, STAGE } from "../labels";

type Kind = Schemas["OpenStage"];
/** A category as edited (a new one has no key yet); id keeps React's rows apart. */
interface Row { id: string; key?: string; label: string; kind: Kind }

let next = 0;
const rowsOf = (cats: Category[]): Row[] => cats.map((c) => ({ id: c.key, key: c.key, label: c.label, kind: c.kind }));

/**
 * Settings › Pipeline stages › Categories: what stages fall under (1st, 2nd
 * and Final Interview...), grouped by the kind each counts as. Everyone sees
 * them; owners, and those given pipeline.categories:manage, change them.
 */
export function StageCategories() {
  const cats = useStageCategories();
  const canManage = useCan("pipeline.categories:manage");
  const [rows, setRows] = useState<Row[] | null>(null);
  const queryClient = useQueryClient();
  const save = useMutation({
    mutationFn: async (items: Row[]) => unwrap(await api.PUT("/v1/stage-categories", {
      body: { version: cats.data?.version ?? "0", items: items.map(({ key, label, kind }) => ({ key, label, kind })) },
    })),
    onSuccess: async (saved) => {
      queryClient.setQueryData(stageCategoriesKey, saved);
      await queryClient.invalidateQueries({ queryKey: stageTemplatesKey });
      await queryClient.invalidateQueries({ queryKey: ["job-stages"] });
      setRows(null);
    },
  });
  const errors = fieldErrors(save.error);
  const errorAt = (r: Row) => {
    const i = rows?.indexOf(r) ?? -1;
    return errors[`items.${String(i)}.label`] ?? errors[`items.${String(i)}.kind`];
  };
  const set = (id: string, label: string) => { setRows((rs) => rs?.map((r) => (r.id === id ? { ...r, label } : r)) ?? null); };
  const shift = (kind: Kind, i: number, by: number) => {
    setRows((rs) => {
      if (!rs) return rs;
      const group = rs.filter((r) => r.kind === kind);
      const [moved] = group.splice(i, 1);
      if (!moved) return rs;
      group.splice(i + by, 0, moved);
      return OPEN_STAGES.flatMap((k) => (k === kind ? group : rs.filter((r) => r.kind === k)));
    });
  };
  return (
    <section className="panel stage-categories" aria-label="Categories">
      <div className="row spread">
        <h3>Categories</h3>
        {canManage && !rows && cats.data && <button type="button" onClick={() => { setRows(rowsOf(cats.data.items)); }}>Edit categories</button>}
      </div>
      <p className="muted small">What each stage falls under. Reports count candidates by category, and a candidate&apos;s stage shows it.</p>
      <ErrorMessage error={cats.error} />
      {cats.data && !rows && (
        <dl className="category-groups">
          {byKind(cats.data.items).map((g) => (
            <div key={g.kind} className={`category-group stage-${g.kind}`}>
              <dt className="category-kind">Counts as {STAGE[g.kind].toLowerCase()}</dt>
              <dd><ol className="stage-chips plain">{g.items.map((c) => <li key={c.key} className={`stage-chip stage-${c.kind}`}>{c.label}</li>)}</ol></dd>
            </div>
          ))}
        </dl>
      )}
      {rows && (
        <form aria-label="Edit categories" onSubmit={(e) => { e.preventDefault(); save.mutate(rows); }}>
          <div className="category-groups">
            {OPEN_STAGES.map((kind) => {
              const group = rows.filter((r) => r.kind === kind);
              return (
                <div key={kind} role="group" aria-label={`Counts as ${STAGE[kind].toLowerCase()}`} className={`category-group editing stage-${kind}`}>
                  <span className="category-kind">Counts as {STAGE[kind].toLowerCase()}</span>
                  <div>
                  <ol className="steps-rows">
                    {group.map((r, i) => {
                      const error = errorAt(r);
                      return (
                        <li key={r.id} className={`steps-row stage-${kind}`}>
                          <span className="steps-dot" aria-hidden="true" />
                          <input aria-label={`${STAGE[kind]} category ${String(i + 1)} name`} maxLength={40} value={r.label}
                            aria-invalid={error ? true : undefined} onChange={(e) => { set(r.id, e.target.value); }} />
                          <span className="steps-actions">
                            <button type="button" className="icon-btn" aria-label={`Move ${r.label || "this category"} up`} disabled={i === 0} onClick={() => { shift(kind, i, -1); }}>↑</button>
                            <button type="button" className="icon-btn" aria-label={`Move ${r.label || "this category"} down`} disabled={i === group.length - 1} onClick={() => { shift(kind, i, 1); }}>↓</button>
                            <button type="button" className="icon-btn danger secondary" aria-label={`Remove ${r.label || "this category"}`} disabled={group.length === 1}
                              onClick={() => { setRows(rows.filter((x) => x.id !== r.id)); }}>✕</button>
                          </span>
                          {error && <small className="error steps-error">{error}</small>}
                        </li>
                      );
                    })}
                  </ol>
                  <button type="button" className="link" disabled={rows.length >= 40} onClick={() => {
                    const last = rows.map((r) => r.kind).lastIndexOf(kind);
                    next += 1;
                    setRows([...rows.slice(0, last + 1), { id: `new${String(next)}`, label: "", kind }, ...rows.slice(last + 1)]);
                  }}>+ Add {/^[aeiou]/.test(kind) ? "an" : "a"} {STAGE[kind].toLowerCase()} category</button>
                  </div>
                </div>
              );
            })}
          </div>
          {errors.items && <small className="error">{errors.items}</small>}
          <ErrorMessage error={Object.keys(errors).length > 0 ? null : save.error} />
          <p className="muted small">A category in use (by a stage or a candidate) cannot be removed: move them first. Renaming keeps it.</p>
          <div className="row">
            <button type="submit" className="primary" disabled={save.isPending}>{save.isPending ? "Saving…" : "Save categories"}</button>
            <button type="button" onClick={() => { setRows(null); save.reset(); }} disabled={save.isPending}>Cancel</button>
          </div>
        </form>
      )}
    </section>
  );
}
