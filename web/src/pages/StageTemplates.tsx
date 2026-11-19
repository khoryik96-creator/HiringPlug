import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { stageTemplatesKey } from "../api/keys";
import { ErrorMessage } from "../components/ErrorMessage";
import { fieldErrors } from "../components/forms";
import { STANDARD } from "../components/stages/columns";
import { type Draft, draftsOf, StepsEditor } from "../components/stages/StepsEditor";

type Template = Schemas["StageTemplate"];

/** Settings › Pipeline stages: the company's stage templates. */
export function StageTemplates() {
  const list = useQuery({ queryKey: stageTemplatesKey, queryFn: async () => unwrap(await api.GET("/v1/stage-templates")) });
  const [editing, setEditing] = useState<Template | "new" | null>(null);
  const items = list.data?.items ?? [];
  return (
    <main>
      <h2>Pipeline stages</h2>
      <p className="muted">
        Save the stages your jobs go through, e.g. Phone screen, Tech test and Panel. Each one counts as a standard stage, which
        reports, automations, offers and placements go by. The default template is used by new jobs and by every job that has not
        chosen its own; a job can choose another template, or change its stages, from its pipeline.
      </p>
      <ErrorMessage error={list.error} />
      {editing === "new" && <TemplateForm onClose={() => { setEditing(null); }} />}
      {editing === null && <button type="button" className="primary" onClick={() => { setEditing("new"); }}>New template</button>}
      <ul className="stage-templates">
        {items.map((t) => editing !== "new" && editing?.id === t.id
          ? <li key={t.id}><TemplateForm template={t} onClose={() => { setEditing(null); }} /></li>
          : <li key={t.id}><TemplateCard template={t} onEdit={editing === null ? () => { setEditing(t); } : undefined} /></li>)}
        {list.data && (
          <li className="panel stage-template standard">
            <div className="row spread"><h3>Standard stages</h3><span className="muted small">Built in</span></div>
            <StageChips steps={list.data.standard} />
            <p className="muted small">{items.some((t) => t.isDefault) ? "Jobs can still choose these." : "What jobs use while no template is the default."}</p>
          </li>
        )}
      </ul>
    </main>
  );
}

function StageChips({ steps }: { steps: Schemas["StageStep"][] }) {
  return (
    <ol className="stage-chips" aria-label="Stages">
      {steps.map((s) => <li key={s.key} className={`stage-chip stage-${s.stage}`}>{s.label}</li>)}
    </ol>
  );
}

function TemplateCard({ template: t, onEdit }: { template: Template; onEdit?: () => void }) {
  const queryClient = useQueryClient();
  const remove = useMutation({
    mutationFn: async () => { unwrap(await api.DELETE("/v1/stage-templates/{stageTemplateId}", { params: { path: { stageTemplateId: t.id } } })); },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: stageTemplatesKey });
      await queryClient.invalidateQueries({ queryKey: ["job-stages"] });
    },
  });
  const used = t.jobCount === 1 ? "1 job uses it" : `${String(t.jobCount)} jobs use it`;
  return (
    <section className="panel stage-template" aria-label={t.name}>
      <div className="row spread">
        <h3>{t.name} {t.isDefault && <span className="badge">Default</span>}</h3>
        <span className="row">
          {onEdit && <button type="button" onClick={onEdit}>Edit</button>}
          {onEdit && (
            <button type="button" className="danger secondary" disabled={remove.isPending} onClick={() => {
              const keep = t.jobCount > 0 ? ` ${t.jobCount === 1 ? "The job" : `The ${String(t.jobCount)} jobs`} using it keep their stages.` : "";
              if (window.confirm(`Delete the template "${t.name}"?${keep}`)) remove.mutate();
            }}>Delete</button>
          )}
        </span>
      </div>
      <StageChips steps={t.steps} />
      <p className="muted small">{t.isDefault ? "New jobs follow it." : ""}{t.jobCount > 0 ? `${t.isDefault ? " " : ""}${used}.` : ""}</p>
      <ErrorMessage error={remove.error} />
    </section>
  );
}

function TemplateForm({ template, onClose }: { template?: Template; onClose: () => void }) {
  const queryClient = useQueryClient();
  const [name, setName] = useState(template?.name ?? "");
  const [isDefault, setDefault] = useState(template?.isDefault ?? false);
  const [steps, setSteps] = useState<Draft[]>(draftsOf(template?.steps ?? STANDARD));
  const save = useMutation({
    mutationFn: async () => {
      const body = { name, isDefault, steps: steps.map((s) => ({ key: s.key, label: s.label, stage: s.stage })) };
      return template
        ? unwrap(await api.PUT("/v1/stage-templates/{stageTemplateId}", { params: { path: { stageTemplateId: template.id } }, body: { ...body, version: template.version } }))
        : unwrap(await api.POST("/v1/stage-templates", { body }));
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: stageTemplatesKey });
      await queryClient.invalidateQueries({ queryKey: ["job-stages"] });
      onClose();
    },
  });
  const errors = fieldErrors(save.error);
  return (
    <form className="panel stage-template editing" aria-label={template ? `Edit ${template.name}` : "New template"}
      onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h3>{template ? `Edit ${template.name}` : "New template"}</h3>
      <label className="field">Name
        <input maxLength={80} value={name} aria-invalid={errors.name ? true : undefined} onChange={(e) => { setName(e.target.value); }} />
        {errors.name && <small className="error">{errors.name}</small>}
      </label>
      <label className="row"><input type="checkbox" checked={isDefault} onChange={(e) => { setDefault(e.target.checked); }} /> Default for new jobs</label>
      <StepsEditor steps={steps} onChange={setSteps} errors={errors} />
      {template && template.jobCount > 0 && <p className="muted small">Jobs that chose it keep the stages they have.</p>}
      <ErrorMessage error={Object.keys(errors).length > 0 ? null : save.error} />
      <div className="row">
        <button type="submit" className="primary" disabled={save.isPending}>{save.isPending ? "Saving…" : "Save template"}</button>
        <button type="button" onClick={onClose} disabled={save.isPending}>Cancel</button>
      </div>
    </form>
  );
}
