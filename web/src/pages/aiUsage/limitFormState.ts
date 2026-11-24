// The limit-and-prices form's state: what the inputs hold, made from the
// saved settings, and the body to save. Amounts stay strings throughout.
import type { Schemas } from "../../api/client";
import { FormInvalid } from "../../components/forms";

export type Settings = Schemas["AiUsageSettings"];
export type SettingsUpdate = Schemas["AiUsageSettingsUpdate"];

export type Draft = {
  kind: Schemas["AiUsageLimitKind"];
  /** The token limit is the server's default (HP_AI_MONTHLY_TOKENS). */
  useDefault: boolean;
  tokens: string;
  money: string;
  currency: string;
  priceIn: string;
  priceOut: string;
  warn: string;
};

export function draftOf(s: Settings): Draft {
  return {
    kind: s.limitKind,
    useDefault: s.limitTokens === null,
    tokens: String(s.limitTokens ?? s.serverDefaultTokens),
    money: s.limitMoney ?? "",
    currency: s.currency,
    priceIn: s.priceInputPerMillion ?? "",
    priceOut: s.priceOutputPerMillion ?? "",
    warn: String(s.warnPercent),
  };
}

/** "1,000.50 " -> "1000.50"; "" -> null. */
function amount(value: string): string | null {
  const v = value.replace(/[\s,]/g, "");
  return v === "" ? null : v;
}

/** The body to save; FormInvalid for whole numbers that are not (the API checks the rest). */
export function bodyOf(d: Draft, version: number): SettingsUpdate {
  const problems: Record<string, string> = {};
  const tokens = d.tokens.replace(/[\s,]/g, "");
  if (!d.useDefault && !/^\d+$/.test(tokens)) problems.limitTokens = "Enter a whole number of tokens, or use the server default.";
  if (!/^\d+$/.test(d.warn.trim())) problems.warnPercent = "Enter a whole percentage between 1 and 99.";
  if (Object.keys(problems).length > 0) throw new FormInvalid(problems);
  return {
    limitKind: d.kind,
    limitTokens: d.useDefault ? null : Number(tokens),
    limitMoney: amount(d.money),
    currency: d.currency.trim().toUpperCase(),
    priceInputPerMillion: amount(d.priceIn),
    priceOutputPerMillion: amount(d.priceOut),
    warnPercent: Number(d.warn.trim()),
    version,
  };
}

/** The settings as the server applies them: back to its default token limit, prices and currency kept. */
export function serverDefault(d: Draft, serverDefaultTokens: number): Draft {
  return { ...d, kind: "tokens", useDefault: true, tokens: String(serverDefaultTokens) };
}
