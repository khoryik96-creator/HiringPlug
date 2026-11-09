// A client's benefits as rows to show and fill: the agency's template (the
// company_benefits and pre_offer_process lists in Settings) in its order,
// with what was saved for each; items saved before but since taken out of
// the template stay (marked) while they say something.
import type { Schemas } from "../api/client";

export type Benefit = Schemas["CompanyBenefit"];
export type Step = Schemas["CompanyPreOfferStep"];
type Item = { key: string; label: string };
export type Row<T> = { item: T; hint: string; retired: boolean };

const said = (x: Benefit | Step) => ("value" in x ? x.value != null : x.applies != null) || x.remarks != null;

export function rowsFor<T extends Item>(template: Schemas["FieldList"] | undefined, saved: T[], blank: (i: Item) => T): Row<T>[] {
  const byKey = new Map(saved.map((s) => [s.key, s]));
  const rows: Row<T>[] = [];
  for (const o of template?.options ?? []) {
    const s = byKey.get(o.key);
    if (o.hidden && !(s && said(s as unknown as Benefit))) continue;
    rows.push({ item: s ? { ...s, label: o.label } : blank(o), hint: o.hint ?? "", retired: o.hidden });
    byKey.delete(o.key);
  }
  for (const s of byKey.values()) {
    if (said(s as unknown as Benefit)) rows.push({ item: s, hint: "", retired: true });
  }
  return rows;
}

export const blankBenefit = (i: Item): Benefit => ({ key: i.key, label: i.label, value: null, remarks: null });
export const blankStep = (i: Item): Step => ({ key: i.key, label: i.label, applies: null, remarks: null });
export const filled = (b: Benefit) => b.value != null || b.remarks != null;
