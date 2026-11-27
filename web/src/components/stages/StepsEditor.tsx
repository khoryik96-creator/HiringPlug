import "./stages.css";
import { OPEN_STAGES, STAGE } from "../../labels";
import { byKind, useStageCategories } from "./categories";
import type { OpenStage, Step } from "./columns";

/** A stage as edited: its key stays the same when it is renamed or moved. */
export interface Draft { key: string; label: string; stage: OpenStage; category?: string }

/** What is saved for drafts. */
export const stepsBody = (steps: Draft[]) => steps.map(({ key, label, stage, category }) => ({ key, label, stage, category }));

/** A key for a new stage (the API keeps it: letters and digits). */
export const newKey = () => `s${Math.random().toString(36).slice(2, 12).replace(/[^a-z0-9]/g, "")}`;

export const draftsOf = (steps: Step[]): Draft[] => steps.map((s) => ({ key: s.key, label: s.label, stage: s.stage, category: s.category }));

/**
 * The stages of a template or a job, as rows: a name, the category it falls
 * under (1st Interview...; its kind is what automations, offers and
 * placements go by), moved up or down, removed, or one added. Field errors
 * come from the API as steps.N.label / steps.N.stage / steps.N.category.
 */
export function StepsEditor({ steps, onChange, errors = {} }: {
  steps: Draft[]; onChange: (steps: Draft[]) => void; errors?: Record<string, string>;
}) {
  const cats = useStageCategories().data?.items;
  const set = (i: number, patch: Partial<Draft>) => { onChange(steps.map((s, j) => (j === i ? { ...s, ...patch } : s))); };
  const swap = (i: number, j: number) => {
    const next = [...steps];
    [next[i], next[j]] = [next[j] as Draft, next[i] as Draft];
    onChange(next);
  };
  return (
    <div className="steps-editor">
      <ol className="steps-rows">
        {steps.map((s, i) => {
          const label = errors[`steps.${String(i)}.label`];
          const kind = errors[`steps.${String(i)}.stage`] ?? errors[`steps.${String(i)}.category`];
          const under = cats?.find((c) => c.key === s.category && c.kind === s.stage) ?? cats?.find((c) => c.kind === s.stage);
          return (
            <li key={s.key} className={`steps-row stage-${s.stage}`}>
              <span className="steps-dot" aria-hidden="true" />
              <input aria-label={`Stage ${String(i + 1)} name`} maxLength={40} value={s.label} aria-invalid={label ? true : undefined}
                onChange={(e) => { set(i, { label: e.target.value }); }} />
              {cats ? (
                <select aria-label={`Stage ${String(i + 1)} falls under`} value={under?.key ?? ""} aria-invalid={kind ? true : undefined}
                  onChange={(e) => {
                    const c = cats.find((x) => x.key === e.target.value);
                    if (c) set(i, { stage: c.kind, category: c.key });
                  }}>
                  {byKind(cats).map((g) => (
                    <optgroup key={g.kind} label={`Counts as ${STAGE[g.kind].toLowerCase()}`}>
                      {g.items.map((c) => <option key={c.key} value={c.key}>{c.label}</option>)}
                    </optgroup>
                  ))}
                </select>
              ) : (
                <select aria-label={`Stage ${String(i + 1)} counts as`} value={s.stage} aria-invalid={kind ? true : undefined}
                  onChange={(e) => { set(i, { stage: e.target.value as OpenStage, category: undefined }); }}>
                  {OPEN_STAGES.map((st) => <option key={st} value={st}>Counts as {STAGE[st].toLowerCase()}</option>)}
                </select>
              )}
              <span className="steps-actions">
                <button type="button" className="icon-btn" aria-label={`Move ${s.label || "this stage"} up`} disabled={i === 0} onClick={() => { swap(i, i - 1); }}>↑</button>
                <button type="button" className="icon-btn" aria-label={`Move ${s.label || "this stage"} down`} disabled={i === steps.length - 1} onClick={() => { swap(i, i + 1); }}>↓</button>
                <button type="button" className="icon-btn danger secondary" aria-label={`Remove ${s.label || "this stage"}`} disabled={steps.length === 1}
                  onClick={() => { onChange(steps.filter((_, j) => j !== i)); }}>✕</button>
              </span>
              {(label ?? kind) && <small className="error steps-error">{label ?? kind}</small>}
            </li>
          );
        })}
      </ol>
      {errors.steps && <small className="error">{errors.steps}</small>}
      <button type="button" className="link" disabled={steps.length >= 20} onClick={() => {
        const last = steps[steps.length - 1];
        // A new stage before Placed (and of the kind before it), or at the end.
        const at = last?.stage === "placed" ? steps.length - 1 : steps.length;
        const kind: OpenStage = steps[at - 1]?.stage ?? "applied";
        onChange([...steps.slice(0, at), { key: newKey(), label: "", stage: kind, category: steps[at - 1]?.category }, ...steps.slice(at)]);
      }}>+ Add a stage</button>
    </div>
  );
}
