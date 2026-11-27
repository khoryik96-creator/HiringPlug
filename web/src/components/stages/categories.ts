// The company's stage categories: what each stage falls under (1st, 2nd and
// Final Interview...), which reports count by and a candidate's stage tag
// shows. Each is one of the six kinds.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../../api/client";
import { stageCategoriesKey } from "../../api/keys";
import { OPEN_STAGES, STAGE } from "../../labels";

export type Category = Schemas["StageCategory"];
type Stage = Schemas["Stage"];

export function useStageCategories() {
  return useQuery({ queryKey: stageCategoriesKey, queryFn: async () => unwrap(await api.GET("/v1/stage-categories")), staleTime: 60_000 });
}

/** The categories grouped by kind, in order (kinds with none left out). */
export function byKind(cats: Category[]): { kind: Schemas["OpenStage"]; items: Category[] }[] {
  return OPEN_STAGES.map((kind) => ({ kind, items: cats.filter((c) => c.kind === kind) })).filter((g) => g.items.length > 0);
}

/**
 * What a candidate's stage is called: its category's name (the kind's first
 * when it has none recorded), else the stage's own (closed, or loading).
 */
export function stageName(stage: Stage, category: string | null | undefined, cats: Category[] | undefined): string {
  if (stage === "rejected" || stage === "withdrawn" || !cats) return STAGE[stage];
  const own = cats.find((c) => c.key === category && c.kind === stage) ?? cats.find((c) => c.kind === stage);
  return own?.label ?? STAGE[stage];
}

/**
 * The categories a person can have a target for: every one but those of the
 * submitted and placed kinds, which CVs sent and Placements already count.
 */
export function targetCategories(cats: Category[]): Category[] {
  return cats.filter((c) => c.kind !== "submitted" && c.kind !== "placed");
}
