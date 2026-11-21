import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Application, type Schemas, unwrap } from "../api/client";
import { jobApplicationsKey } from "../api/keys";

type Stage = Schemas["Stage"];
const all = "all";
type Scope = Stage | typeof all;

async function page(jobId: string, scope: Scope, signal: AbortSignal, cursor?: string, limit = 100) {
  return unwrap(await api.GET("/v1/jobs/{jobId}/applications", {
    signal, params: { path: { jobId }, query: {
      limit, ...(cursor ? { cursor } : {}), ...(scope !== all ? { stage: scope } : {}),
      ...(scope === all && !cursor ? { includeCounts: true } : {}),
    } },
  }));
}

const fullDepth = Number.MAX_SAFE_INTEGER;
const pageKey = (jobId: string, scope: Scope, depth: number) => [...jobApplicationsKey(jobId), "pipeline", scope, depth];

export function completePipelineStage(jobId: string, scope: Scope) {
  return { queryKey: pageKey(jobId, scope, fullDepth),
    queryFn: ({ signal }: { signal: AbortSignal }) => pages(jobId, scope, signal, fullDepth, 1000) };
}

async function pages(jobId: string, scope: Scope, signal: AbortSignal, depth: number, limit: number) {
  const items: Application[] = [];
  let cursor: string | undefined;
  let counts: Schemas["PipelineColumnCount"][] | undefined;
  for (let i = 0; i < depth; i++) {
    const result = await page(jobId, scope, signal, cursor, limit);
    counts ??= result.counts;
    items.push(...result.items);
    cursor = result.nextCursor;
    if (!cursor) break;
  }
  return { items, nextCursor: cursor, counts };
}

export function usePipelinePages(jobId: string) {
  const queryClient = useQueryClient();
  const [depths, setDepths] = useState<Partial<Record<Scope, number>>>({ all: 1 });
  const scopes = Object.keys(depths) as Scope[];
  const queries = useQueries({ queries: scopes.map((scope) => {
    const depth = depths[scope] ?? 1;
    return {
      queryKey: pageKey(jobId, scope, depth),
      queryFn: ({ signal }: { signal: AbortSignal }) => pages(jobId, scope, signal, depth, depth === fullDepth ? 1000 : 100),
      placeholderData: () => queryClient.getQueryData<Awaited<ReturnType<typeof pages>>>(pageKey(jobId, scope, depth - 1)),
    };
  }) });
  const items = new Map<string, Application>();
  for (const query of queries) for (const item of query.data?.items ?? []) items.set(item.id, item);
  function requested(scope: Scope) { return scopes.includes(scope); }
  function resultFor(scope: Scope) { return queries[scopes.indexOf(scope)]; }
  return {
    items: [...items.values()],
    counts: queries[0]?.data?.counts,
    loaded: Boolean(queries[0]?.data),
    error: queries.find((q) => q.error)?.error,
    requested,
    pending: (scope: Scope) => resultFor(scope)?.isFetching ?? false,
    more: (scope: Scope) => Boolean(resultFor(scope)?.data?.nextCursor),
    load: (scope: Scope) => { setDepths((d) => ({ ...d, [scope]: (d[scope] ?? 0) + 1 })); },
    reveal: (scope: Scope) => { setDepths((d) => ({ ...d, [scope]: fullDepth })); },
  };
}
