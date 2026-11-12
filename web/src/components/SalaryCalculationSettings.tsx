// Settings > Salary calculation: how the screening calculator turns a salary
// breakdown into a fixed monthly figure, and how salary reads in emails. The
// server applies these saved rules; the browser never sends its own.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { ErrorMessage } from "./ErrorMessage";

type Rules = Schemas["SalaryCalculationRules"];

export const salaryRulesKey = ["salary-calculation-rules"] as const;

export const useSalaryRules = () =>
  useQuery({
    queryKey: salaryRulesKey,
    queryFn: async () => unwrap(await api.GET("/v1/settings/salary-calculation")),
  });

export function SalaryCalculationSettings() {
  const query = useSalaryRules();
  const [saved, setSaved] = useState(false);
  return (
    <section className="panel" aria-label="Salary calculation">
      <h3>Salary calculation</h3>
      <ErrorMessage error={query.error} />
      {query.data && (
        <RulesForm
          key={JSON.stringify(query.data)}
          initial={query.data}
          onSaved={() => { setSaved(true); }}
          onEdited={() => { setSaved(false); }}
        />
      )}
      {saved && <span role="status">Saved</span>}
    </section>
  );
}

function RulesForm({ initial, onSaved, onEdited }: { initial: Rules; onSaved: () => void; onEdited: () => void }) {
  const client = useQueryClient();
  const [rules, set] = useState(initial);
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/settings/salary-calculation", { body: rules })),
    onSuccess: (data) => {
      onSaved();
      client.setQueryData(salaryRulesKey, data);
      void client.invalidateQueries({ queryKey: ["email-context"] }); // emails show salary the new way
    },
  });
  const change = (next: Rules) => {
    onEdited();
    set(next);
  };
  const busy = save.isPending;

  return (
    <form className="form-grid" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <label className="field full">
        Salary in emails
        <select
          disabled={busy}
          value={rules.emailSalaryDisplay ?? "monthly"}
          onChange={(e) => { change({ ...rules, emailSalaryDisplay: e.target.value as "monthly" | "breakdown" }); }}
        >
          <option value="monthly">Monthly salary</option>
          <option value="breakdown">Monthly salary breakdown</option>
        </select>
      </label>
      <label className="row full">
        <input
          type="checkbox"
          disabled={busy}
          checked={rules.allowances}
          onChange={(e) => { change({ ...rules, allowances: e.target.checked }); }}
        />
        {" "}Include fixed allowances
      </label>
      <label className="row full">
        <input
          type="checkbox"
          disabled={busy}
          checked={rules.bonus}
          onChange={(e) => { change({ ...rules, bonus: e.target.checked }); }}
        />
        {" "}Include guaranteed bonus
      </label>
      <label className="field">
        Bonus basis
        <select
          disabled={busy}
          value={rules.bonusOnAllowances ? "with_allowances" : "basic"}
          onChange={(e) => { change({ ...rules, bonusOnAllowances: e.target.value === "with_allowances" }); }}
        >
          <option value="basic">Basic salary</option>
          <option value="with_allowances">Basic salary + included fixed allowances</option>
        </select>
      </label>
      <label className="field">
        Rounding
        <select
          disabled={busy}
          value={rules.cents ? "cents" : "whole"}
          onChange={(e) => { change({ ...rules, cents: e.target.value === "cents" }); }}
        >
          <option value="whole">Nearest whole unit</option>
          <option value="cents">Two decimal places</option>
        </select>
      </label>
      <p className="salary-formula full">{formula(rules)}</p>
      <div className="row full">
        <button type="submit" disabled={busy}>Save calculation rules</button>
      </div>
      <ErrorMessage error={save.error} />
    </form>
  );
}

/** "Basic monthly salary + fixed allowances + (basic) × extra guaranteed months ÷ 12" */
function formula(rules: Rules): string {
  let text = "Basic monthly salary";
  if (rules.allowances) text += " + fixed allowances";
  if (rules.bonus) {
    const basis = rules.bonusOnAllowances && rules.allowances ? "basic + allowances" : "basic";
    text += ` + (${basis}) × extra guaranteed months ÷ 12`;
  }
  return text;
}
