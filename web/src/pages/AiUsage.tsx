// Settings › AI usage: this month's AI use against the company's limit, the
// limit and the prices, what each feature and each person used, and the last
// 12 months. Costs are estimates from the token counts at the company's
// prices; the AI provider's bill is final.
import { useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { LimitForm } from "./aiUsage/LimitForm";
import { MonthsChart } from "./aiUsage/MonthsChart";
import { ThisMonth } from "./aiUsage/ThisMonth";
import { UsageTables } from "./aiUsage/UsageTables";
import "./aiUsage/aiUsage.css";

const MONTHS = 12;
export const aiUsageKey = ["ai-usage", "report", MONTHS] as const;

export function AiUsage({ canManage }: { canManage: boolean }) {
  const usage = useQuery({
    queryKey: aiUsageKey,
    queryFn: async () => unwrap(await api.GET("/v1/ai-usage", { params: { query: { months: MONTHS } } })),
  });
  const u = usage.data;
  return (
    <main className="ai-usage">
      <h2>AI usage</h2>
      <p className="muted">
        What the AI features used this month, what it costs, and the monthly limit. At the limit, AI features stop until the next month
        starts; everyone is warned first.
      </p>
      <ErrorMessage error={usage.error} />
      {usage.isPending && <p className="muted" role="status">Loading…</p>}
      <div className="ai-usage-top">
        {u && <ThisMonth usage={u} />}
        <LimitForm canManage={canManage} />
      </div>
      {u && (
        <>
          <UsageTables usage={u} />
          <MonthsChart usage={u} />
        </>
      )}
      <p className="ai-usage-note">Costs are worked out from the token counts and your prices; your provider&apos;s bill is the final word.</p>
    </main>
  );
}
