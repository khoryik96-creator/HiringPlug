// What used the AI: per feature (this month, or a month picked on the chart)
// and per person this month, where what the system did on its own is
// "Automatic". Each table scrolls inside its box on a phone.
import { useState } from "react";

import { SectionTabs } from "../../components/SectionTabs";
import { allTokens, costText, FEATURE, type FeatureUse, type Usage } from "./format";

export function FeatureTable({ rows, currency, label }: { rows: FeatureUse[]; currency: string; label: string }) {
  if (rows.length === 0) return <p className="muted">No AI use.</p>;
  return (
    <div className="table-scroll">
      <table className="ai-usage-table" aria-label={label}>
        <thead>
          <tr><th>Feature</th><th className="num">Uses</th><th className="num">Tokens in</th><th className="num">Tokens out</th><th className="num">Cost</th></tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.feature}>
              <td>{FEATURE[r.feature]}</td>
              <td className="num">{allTokens(r.uses)}</td>
              <td className="num">{allTokens(r.inputTokens)}</td>
              <td className="num">{allTokens(r.outputTokens)}</td>
              <td className="num">{costText(r.cost, currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function PersonTable({ usage }: { usage: Usage }) {
  const currency = usage.limit.currency;
  if (usage.byPerson.length === 0) return <p className="muted">No one has used the AI this month.</p>;
  return (
    <div className="table-scroll">
      <table className="ai-usage-table" aria-label="This month by person">
        <thead>
          <tr><th>Who</th><th className="num">Uses</th><th className="num">Tokens</th><th className="num">Cost</th></tr>
        </thead>
        <tbody>
          {usage.byPerson.map((p) => (
            <tr key={p.userId ?? "automatic"}>
              <td>{p.userId === null ? <span title="What HyPlug did on its own, e.g. reading CVs as they arrive">Automatic</span> : p.name}</td>
              <td className="num">{allTokens(p.uses)}</td>
              <td className="num">{allTokens(p.tokens)}</td>
              <td className="num">{costText(p.cost, currency)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

type Tab = "feature" | "person";

export function UsageTables({ usage }: { usage: Usage }) {
  const [tab, setTab] = useState<Tab>("feature");
  return (
    <section className="panel ai-usage-tables" aria-label="What used the AI this month">
      <SectionTabs<Tab> name="ai-usage" label="This month's use" active={tab} onChange={setTab} sections={[
        { id: "feature", label: "By feature", content: <FeatureTable rows={usage.byFeature} currency={usage.limit.currency} label="This month by feature" /> },
        { id: "person", label: "By person", content: <PersonTable usage={usage} /> },
      ]} />
      {!usage.pricesSet && <p className="muted small">Set the prices per million tokens to see what each use costs.</p>}
    </section>
  );
}
