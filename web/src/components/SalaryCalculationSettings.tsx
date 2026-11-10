import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";

export const salaryRulesKey = ["salary-calculation-rules"] as const;
export const useSalaryRules = () => useQuery({ queryKey: salaryRulesKey, queryFn: async () => unwrap(await api.GET("/v1/settings/salary-calculation")) });

export function SalaryCalculationSettings() {
  const query = useSalaryRules();
  return <section className="panel" aria-label="Salary calculation"><h3>Salary calculation</h3><ErrorMessage error={query.error} />
    {query.data && <RulesForm key={JSON.stringify(query.data)} initial={query.data} />}
  </section>;
}

function RulesForm({ initial }: { initial: Schemas["SalaryCalculationRules"] }) {
  const client = useQueryClient();
  const [rules, set] = useState(initial);
  const save = useMutation({ mutationFn: async () => unwrap(await api.PUT("/v1/settings/salary-calculation", { body: rules })),
    onSuccess: (data) => { client.setQueryData(salaryRulesKey, data); } });
  return <form className="form-grid" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
    <label className="row full"><input type="checkbox" checked={rules.allowances} onChange={(e) => { set({ ...rules, allowances: e.target.checked }); }} /> Include fixed allowances</label>
    <label className="row full"><input type="checkbox" checked={rules.bonus} onChange={(e) => { set({ ...rules, bonus: e.target.checked }); }} /> Include guaranteed bonus</label>
    <label className="field">Bonus basis<select value={rules.bonusOnAllowances ? "with_allowances" : "basic"} onChange={(e) => { set({ ...rules, bonusOnAllowances: e.target.value === "with_allowances" }); }}>
      <option value="basic">Basic salary</option><option value="with_allowances">Basic salary + fixed allowances</option></select></label>
    <label className="field">Rounding<select value={rules.cents ? "cents" : "whole"} onChange={(e) => { set({ ...rules, cents: e.target.value === "cents" }); }}>
      <option value="whole">Nearest whole unit</option><option value="cents">Two decimal places</option></select></label>
    <p className="salary-formula full">Basic monthly salary{rules.allowances ? " + fixed allowances" : ""}{rules.bonus ? ` + (${rules.bonusOnAllowances ? "basic + allowances" : "basic"}) × extra guaranteed months ÷ 12` : ""}</p>
    <div className="row full"><button type="submit" disabled={save.isPending}>Save calculation rules</button>{save.isSuccess && <span role="status">Saved</span>}</div>
    <ErrorMessage error={save.error} />
  </form>;
}
