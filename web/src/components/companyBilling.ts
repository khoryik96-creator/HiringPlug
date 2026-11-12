// What a client was billed: the fees of placements on its jobs that stood
// (not fallen through or cancelled), this year and in all, per currency.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { placementsKey } from "../api/keys";

/** "RM 20,000", or "RM 20,000 + SGD 3,500" in more than one currency; null when nothing. */
export function billingText(totals: Schemas["FeeTotal"][] | undefined): string | null {
  const parts = (totals ?? []).filter((t) => Number(t.total) > 0).map((t) => {
    const n = Number(t.total).toLocaleString("en-MY", { maximumFractionDigits: 0 });
    return `${t.currency === "MYR" ? "RM" : t.currency} ${n}`;
  });
  return parts.length ? parts.join(" + ") : null;
}

/** This year's billing (the year in the agency's time zone) and all of it, for those who may see fees. */
export function useCompanyBilling(companyId: string, enabled: boolean) {
  return useQuery({
    queryKey: [...placementsKey, "billing", companyId],
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/billing", { params: { path: { companyId } } })),
    enabled,
  });
}
