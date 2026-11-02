import { useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { usedValuesKey } from "../api/keys";

/** Tags, skills, industries and specializations already used, for suggestions. */
export function useUsedValues() {
  return useQuery({
    queryKey: usedValuesKey,
    queryFn: async () => unwrap(await api.GET("/v1/candidates/used-values")),
    staleTime: 60_000,
  }).data ?? { tags: [], skills: [], industries: [], specializations: [] };
}
