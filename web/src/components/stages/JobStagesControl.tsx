import "./stages.css";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../../api/client";
import { jobStagesKey, stageTemplatesKey } from "../../api/keys";
import { ErrorMessage } from "../ErrorMessage";
import { fieldErrors } from "../forms";
import { STANDARD } from "./columns";
import { type Draft, draftsOf, StepsEditor } from "./StepsEditor";

type JobStages = Schemas["JobStages"];

/** What a job's stages are, in a few words. */
function sourceText(s: JobStages): string {
  switch (s.source) {
    case "standard": return "Standard stages";
    case "default": return `${s.templateName ?? "Default"} (default)`;
    case "template": return s.templateName ?? "Template";
    case "custom": return s.templateName ? `${s.templateName}, changed for this job` : "This job's own stages";
  }
}

/**
 * A job's stages, above its pipeline: what they are, and (with
 * pipeline:update) choosing a template, following the default again, or
 * changing the stages for this job only.
 */
export function JobStagesControl({ jobId, stages, canChange }: { jobId: string; stages: JobStages; canChange: boolean }) {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<Draft[] | null>(null);
  const templates = useQuery({
    queryKey: stageTemplatesKey,
    queryFn: async () => unwrap(await api.GET("/v1/stage-templates")),
    enabled: open,
  });
  const save = useMutation({
    mutationFn: async (body: Omit<Schemas["JobStagesInput"], "version">) =>
      unwrap(await api.PUT("/v1/jobs/{jobId}/stages", { params: { path: { jobId } }, body: { ...body, version: stages.version } })),
    onSuccess: (saved) => {
      queryClient.setQueryData(jobStagesKey(jobId), saved);
      void queryClient.invalidateQueries({ queryKey: stageTemplatesKey });
      setEditing(null);
    },
  });
  const errors = fieldErrors(save.error);
  const items = templates.data?.items ?? [];
  const byDefault = items.find((t) => t.isDefault);
  const chosen = stages.source === "default" ? "default" : stages.source === "template" ? `t:${stages.templateId ?? ""}` : stages.source === "standard" ? "default" : "custom";
  const choose = (value: string) => {
    if (value === "default") save.mutate({});
    else if (value === "standard") save.mutate({ steps: STANDARD.map(({ key, label, stage }) => ({ key, label, stage })) });
    else if (value.startsWith("t:")) save.mutate({ templateId: value.slice(2) });
  };
  return (
    <div className="job-stages">
      <button type="button" className="pill-btn" aria-expanded={open} onClick={() => { setOpen(!open); setEditing(null); }}>
        Stages: {sourceText(stages)}
      </button>
      {open && (
        <section className="panel job-stages-panel" aria-label="Stages of this job">
          {canChange ? (
            <label className="field">Use
              <select value={chosen} disabled={save.isPending || !templates.data} onChange={(e) => { choose(e.target.value); }}>
                <option value="default">{byDefault ? `The default: ${byDefault.name}` : "The standard stages (the default)"}</option>
                {byDefault && <option value="standard">The standard stages</option>}
                {items.filter((t) => !t.isDefault).map((t) => <option key={t.id} value={`t:${t.id}`}>{t.name}</option>)}
                {byDefault && <option value={`t:${byDefault.id}`}>A copy of {byDefault.name} (kept if it changes)</option>}
                {chosen === "custom" && <option value="custom">This job&apos;s own stages</option>}
              </select>
            </label>
          ) : <p className="muted small">{sourceText(stages)}</p>}
          {editing ? (
            <form aria-label="This job's stages" onSubmit={(e) => {
              e.preventDefault();
              save.mutate({ templateId: stages.source === "template" || stages.source === "custom" ? stages.templateId : null, steps: editing.map(({ key, label, stage }) => ({ key, label, stage })) });
            }}>
              <StepsEditor steps={editing} onChange={setEditing} errors={errors} />
              <p className="muted small">Candidates in a stage you remove stay in the first stage of the same kind. Only this job changes.</p>
              <div className="row">
                <button type="submit" className="primary" disabled={save.isPending}>{save.isPending ? "Saving…" : "Save for this job"}</button>
                <button type="button" onClick={() => { setEditing(null); save.reset(); }} disabled={save.isPending}>Cancel</button>
              </div>
            </form>
          ) : (
            <>
              <ol className="stage-chips" aria-label="Stages">
                {stages.steps.map((s) => <li key={s.key} className={`stage-chip stage-${s.stage}`}>{s.label}</li>)}
              </ol>
              {canChange && <button type="button" className="link" onClick={() => { setEditing(draftsOf(stages.steps)); }}>Change the stages for this job</button>}
            </>
          )}
          <ErrorMessage error={Object.keys(errors).length > 0 ? null : save.error} />
          <ErrorMessage error={templates.error} />
        </section>
      )}
    </div>
  );
}
