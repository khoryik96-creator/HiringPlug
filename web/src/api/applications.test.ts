import { afterEach, expect, it, vi } from "vitest";

import { api } from "./client";
import { loadApplications } from "./applications";
import { ok } from "../test-utils";

afterEach(() => { vi.restoreAllMocks(); });

it("continues loading applications after the first page", async () => {
  const get = vi.spyOn(api, "GET")
    .mockResolvedValueOnce(ok({ items: [{ id: "appl_first" }], nextCursor: "appl_first" }))
    .mockResolvedValueOnce(ok({ items: [{ id: "appl_last" }] }));
  const signal = new AbortController().signal;
  const all = await loadApplications("job", "job_fixture", signal);
  expect(all.items.map((app) => app.id)).toEqual(["appl_first", "appl_last"]);
  expect(get).toHaveBeenLastCalledWith("/v1/jobs/{jobId}/applications", {
    params: { path: { jobId: "job_fixture" }, query: { cursor: "appl_first" } }, signal,
  });
});
