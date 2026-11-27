// A job's own stages as the board's columns. Each stage is of one standard
// kind (applied ... placed): reports, automations, offers and placements go
// by the kind. A candidate is in the stage they were moved to, else the
// first stage of their kind; with none of their kind left (stages changed
// since), a column of that kind is shown so nobody is hidden.
import { useQuery } from "@tanstack/react-query";

import { api, type Application, type Schemas, unwrap } from "../../api/client";
import { jobStagesKey } from "../../api/keys";
import { OPEN_STAGES, STAGE } from "../../labels";

export type OpenStage = Schemas["OpenStage"];
export type Step = Schemas["StageStep"];

export interface Column {
  key: string;
  label: string;
  stage: OpenStage;
  /** The job's stage to move to (undefined: a column of a kind the job no longer has). */
  step?: string;
}

/** The standard stages, one of each kind (until the company's, one per category, are loaded). */
export const STANDARD: Step[] = OPEN_STAGES.map((st) => ({ key: st, label: STAGE[st], stage: st, category: st }));

export function useJobStages(jobId: string) {
  return useQuery({
    queryKey: jobStagesKey(jobId),
    queryFn: async () => unwrap(await api.GET("/v1/jobs/{jobId}/stages", { params: { path: { jobId } } })),
  });
}

const kindIndex = (st: OpenStage) => OPEN_STAGES.indexOf(st);

/**
 * The board's columns: the job's stages, and one for each kind someone is in
 * that the job no longer has. With the standard stages (own: false) a move
 * names only the stage, as it always did.
 */
export function columnsFor(steps: Step[], apps: Pick<Application, "stage">[], own = true): Column[] {
  const out: Column[] = steps.map((s) => ({ key: s.key, label: s.label, stage: s.stage, step: own ? s.key : undefined }));
  for (const st of OPEN_STAGES) {
    if (out.some((c) => c.stage === st) || !apps.some((a) => a.stage === st)) continue;
    const at = out.findIndex((c) => kindIndex(c.stage) > kindIndex(st));
    out.splice(at < 0 ? out.length : at, 0, { key: `kind:${st}`, label: STAGE[st], stage: st });
  }
  return out;
}

/** The column a candidate is in (undefined when rejected or withdrawn). */
export function columnOf(a: Pick<Application, "stage" | "step">, columns: Column[]): Column | undefined {
  if (a.stage === "rejected" || a.stage === "withdrawn") return undefined;
  return columns.find((c) => c.step !== undefined && c.step === a.step && c.stage === a.stage) ?? columns.find((c) => c.stage === a.stage);
}

/** Whether a job has stages of its own (not the standard ones). */
export const ownStages = (source: Schemas["JobStages"]["source"] | undefined) => source !== undefined && source !== "standard";

/** The column after the candidate's (their usual next step), if any. */
export function nextColumn(a: Application, columns: Column[]): Column | undefined {
  const here = columnOf(a, columns);
  if (!here) return undefined;
  return columns[columns.indexOf(here) + 1];
}
