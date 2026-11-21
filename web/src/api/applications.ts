import { api, type Schemas, unwrap } from "./client";

/** Keep existing board and candidate selectors complete while reading bounded API pages. */
export async function loadApplications(kind: "job" | "candidate", id: string, signal: AbortSignal): Promise<Schemas["ApplicationList"]> {
  const items: Schemas["Application"][] = [];
  let cursor: string | undefined;
  do {
    const query = cursor ? { cursor } : undefined;
    const page: Schemas["ApplicationList"] = kind === "job"
      ? unwrap(await api.GET("/v1/jobs/{jobId}/applications", { params: { path: { jobId: id }, ...(query && { query }) }, signal }))
      : unwrap(await api.GET("/v1/candidates/{candidateId}/applications", { params: { path: { candidateId: id }, ...(query && { query }) }, signal }));
    items.push(...page.items);
    cursor = page.nextCursor;
  } while (cursor);
  return { items };
}
