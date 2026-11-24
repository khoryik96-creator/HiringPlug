// This month at a glance: how much is used against the limit, in money or
// tokens, a meter with the warning mark, and when the limit starts again.
import { resetDay } from "../../components/AiLimitNotice";
import { aiMoney, allTokens, limitState, monthName, shortTokens, type Usage } from "./format";

function Stat({ value, title, label }: { value: string; title?: string; label: string }) {
  return (
    <div className="ai-usage-stat">
      <div className="ai-usage-big" title={title}>{value}</div>
      <div className="muted small">{label}</div>
    </div>
  );
}

export function ThisMonth({ usage }: { usage: Usage }) {
  const { limit } = usage;
  const currency = limit.currency;
  const tokens = usage.inputTokens + usage.outputTokens;
  const state = limitState(limit);
  const uses = `${allTokens(usage.uses)} ${usage.uses === 1 ? "use" : "uses"}`;
  const filled = Math.min(Math.max(limit.usedPercent, 0), 100);
  return (
    <section className="panel ai-usage-month" aria-label="This month">
      <div className="row spread">
        <h3>{monthName(usage.month)}</h3>
        <span className={`badge ai-usage-pill ${state.tone}`}>{state.label}</span>
      </div>
      <div className="ai-usage-stats">
        {limit.kind === "money" && limit.money !== null ? (
          <>
            <Stat value={aiMoney(usage.cost ?? "0.0000", currency)} label={`of ${aiMoney(limit.money, currency)} this month`} />
            <Stat value={shortTokens(tokens)} title={`${allTokens(tokens)} tokens`} label={`tokens · ${uses}`} />
          </>
        ) : (
          <>
            <Stat value={shortTokens(tokens)} title={`${allTokens(tokens)} tokens`}
              label={`of ${limit.tokens === null ? "—" : shortTokens(limit.tokens)} tokens this month`} />
            <Stat value={allTokens(usage.uses)} label={usage.uses === 1 ? "use" : "uses"} />
            {usage.cost !== null && <Stat value={aiMoney(usage.cost, currency)} label="estimated cost" />}
          </>
        )}
      </div>
      <div className={`ai-usage-meter ${state.tone}`} role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={filled}
        aria-label={`${String(limit.usedPercent)}% of the limit used`}>
        <span style={{ width: `${String(filled)}%` }} />
        <i className="ai-usage-tick" style={{ left: `${String(limit.warnPercent)}%` }} title={`Warning at ${String(limit.warnPercent)}%`} />
      </div>
      <p className="muted small">
        {limit.usedPercent}% used · resets {resetDay(limit.resetsAt)} · warning at {limit.warnPercent}%
      </p>
      {limit.reached && <p className="small ai-usage-stop">AI features are paused until {resetDay(limit.resetsAt)}. Raise the limit to start them again now.</p>}
    </section>
  );
}
