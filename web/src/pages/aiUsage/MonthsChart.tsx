// The last months as bars (cost for a money limit, else tokens), oldest on
// the left, with this month's limit as a line. Picking a month lists what
// each feature used in it.
import { type RefObject, useLayoutEffect, useRef, useState } from "react";

import {
  aiMoney, allTokens, costText, fromTenThousandths, monthName, monthShort, shortTokens, tenThousandths, type Usage,
} from "./format";
import { FeatureTable } from "./UsageTables";

// Drawn at the box's own width (one unit a pixel), so its words stay readable on a phone.
const H = 220;
const LEFT = 44;
const RIGHT = 8;
const TOP = 16;
const BOTTOM = 28;
const PLOT = H - TOP - BOTTOM;

/** The width of an element, followed as it changes (640 until measured, e.g. in tests). */
function useWidth<T extends HTMLElement>(): [RefObject<T | null>, number] {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(640);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (el.clientWidth > 0) setWidth(el.clientWidth);
    if (typeof ResizeObserver === "undefined") return;
    const watch = new ResizeObserver(([entry]) => { if (entry && entry.contentRect.width > 0) setWidth(Math.round(entry.contentRect.width)); });
    watch.observe(el);
    return () => { watch.disconnect(); };
  }, []);
  return [ref, width];
}

/** The smallest of 1, 2, 2.5, 5 × 10^k at or above n (n > 0). */
export function niceCeil(n: number): number {
  if (n <= 0) return 1;
  const p = 10 ** Math.floor(Math.log10(n));
  for (const step of [1, 2, 2.5, 5, 10]) if (step * p >= n) return step * p;
  return 10 * p;
}

type Month = Usage["months"][number];

export function MonthsChart({ usage }: { usage: Usage }) {
  const [picked, setPicked] = useState<string | null>(null);
  const [box, W] = useWidth<HTMLDivElement>();
  const { limit } = usage;
  const currency = limit.currency;
  const money = limit.kind === "money" && limit.money !== null;
  // Money as whole ten-thousandths (exact); tokens as they are.
  const valueOf = (m: Month) => (money ? tenThousandths(m.cost ?? "0") : m.inputTokens + m.outputTokens);
  const limitValue = money ? tenThousandths(limit.money ?? "0") : limit.tokens ?? 0;
  const months = [...usage.months].reverse();
  const top = niceCeil(Math.max(limitValue, ...months.map(valueOf)) * 1.1); // room above the line for its label
  const y = (v: number) => TOP + PLOT - (v / top) * PLOT;
  const tick = (v: number) => (money ? fromTenThousandths(Math.round(v)) : shortTokens(v));
  const slot = (W - LEFT - RIGHT) / Math.max(months.length, 1);
  const bar = Math.min(slot * 0.62, 56);
  const narrow = slot < 34; // a phone: every other month's name
  const said = (m: Month) => money
    ? `${monthName(m.month)}: ${costText(m.cost, currency)}, ${allTokens(m.inputTokens + m.outputTokens)} tokens`
    : `${monthName(m.month)}: ${allTokens(m.inputTokens + m.outputTokens)} tokens${m.cost === null ? "" : `, ${aiMoney(m.cost, currency)}`}`;
  const chosen = months.find((m) => m.month === picked);
  const pick = (month: string) => { setPicked(picked === month ? null : month); };
  return (
    <section className="panel ai-usage-chart" aria-label={`Last ${String(months.length)} months`}>
      <div className="row spread">
        <h3>Last {months.length} months</h3>
        <span className="muted small">{money ? `cost per month, ${currency}` : "tokens per month"}</span>
      </div>
      <div ref={box} className="ai-usage-plot">
      <svg width={W} height={H} viewBox={`0 0 ${String(W)} ${String(H)}`} role="group" aria-label={money ? "Cost per month" : "Tokens per month"}>
        {[0, top / 2, top].map((v) => (
          <g key={v}>
            <line x1={LEFT} x2={W - RIGHT} y1={y(v)} y2={y(v)} className={v === 0 ? "axis" : "grid"} />
            <text x={LEFT - 6} y={y(v) + 4} textAnchor="end">{tick(v)}</text>
          </g>
        ))}
        {months.map((m, i) => {
          const x = LEFT + i * slot;
          const h = (valueOf(m) / top) * PLOT;
          const current = m.month === usage.month;
          return (
            <g key={m.month} role="button" tabIndex={0} aria-label={said(m)} aria-pressed={picked === m.month}
              className={`ai-usage-bar${current ? " current" : ""}${picked === m.month ? " picked" : ""}`}
              onClick={() => { pick(m.month); }}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(m.month); } }}>
              <title>{said(m)}</title>
              <rect x={x} y={TOP} width={slot} height={PLOT + BOTTOM} className="hit" />
              {h > 0 && <rect x={x + (slot - bar) / 2} y={TOP + PLOT - Math.max(h, 2)} width={bar} height={Math.max(h, 2)} rx={3} className="fill" />}
              {(!narrow || (months.length - 1 - i) % 2 === 0) && <text x={x + slot / 2} y={H - 10} textAnchor="middle">{monthShort(m.month)}</text>}
            </g>
          );
        })}
        {limitValue > 0 && (
          <g className="limit">
            <line x1={LEFT} x2={W - RIGHT} y1={y(limitValue)} y2={y(limitValue)} />
            <text x={W - RIGHT} y={y(limitValue) - 5} textAnchor="end">
              Limit {money ? aiMoney(limit.money ?? "0", currency) : shortTokens(limitValue)}
            </text>
          </g>
        )}
      </svg>
      </div>
      <p className="muted small">The line is this month&apos;s limit. Click a month to see its features.</p>
      {chosen && (
        <div className="ai-usage-picked">
          <div className="row spread">
            <h4>{monthName(chosen.month)}</h4>
            <button type="button" className="link" onClick={() => { setPicked(null); }}>Close</button>
          </div>
          <p className="muted small">
            {allTokens(chosen.uses)} {chosen.uses === 1 ? "use" : "uses"} · {allTokens(chosen.inputTokens + chosen.outputTokens)} tokens
            {chosen.cost !== null && <> · {aiMoney(chosen.cost, currency)}</>}
          </p>
          <FeatureTable rows={chosen.byFeature} currency={currency} label={`${monthName(chosen.month)} by feature`} />
        </div>
      )}
    </section>
  );
}
