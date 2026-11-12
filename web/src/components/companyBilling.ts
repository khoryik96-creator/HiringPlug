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

/** This year's billing and all of it, for those who may see fees. */
export function useCompanyBilling(companyId: string, enabled: boolean, today = new Date()) {
  const year = today.getFullYear();
  const totals = (query: Record<string, string>) => ({
    queryKey: [...placementsKey, "billing", companyId, query],
    queryFn: async () => unwrap(await api.GET("/v1/placements", { params: { query: { companyId, ...query } } })).feeTotals,
    enabled,
  });
  const thisYear = useQuery(totals({ startFrom: `${String(year)}-01-01`, startTo: `${String(year)}-12-31` }));
  const allTime = useQuery(totals({}));
  return { year, thisYear: thisYear.data, allTime: allTime.data, loading: thisYear.isPending || allTime.isPending };
}
