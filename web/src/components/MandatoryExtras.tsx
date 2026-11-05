// Inputs for the fields a company made mandatory, on quick "add" forms that
// otherwise ask only for the basics.
import type { Schemas } from "../api/client";
import { keysOf, WORK_AUTH_STATUS } from "../labels";
import { useFieldSettings } from "./useFieldSettings";

type Body = Partial<Schemas["CandidateCreate"]>;

const TEXT: Record<string, string> = {
  nationality: "Nationality (e.g. MY)", location: "Location", currentTitle: "Current title", currentCompany: "Current company",
};
const SHOWN = new Set(["firstName", "lastName", "email", "phone"]);

/** The extra fields to ask for, and their values as part of the request body. */
export function CandidateExtras({ values, onChange }: { values: Record<string, string>; onChange: (v: Record<string, string>) => void }) {
  const fs = useFieldSettings();
  const needed = ["nationality", "location", "currentTitle", "currentCompany", "industry", "source", "noticePeriod", "workAuthorization"]
    .filter((f) => !SHOWN.has(f) && fs.mandatory("candidate", f));
  const set = (f: string) => (e: { target: { value: string } }) => { onChange({ ...values, [f]: e.target.value }); };
  const select = (f: string, label: string, options: { key: string; label: string }[]) => (
    <select key={f} aria-label={label} value={values[f] ?? ""} onChange={set(f)}>
      <option value="">{label}…</option>
      {options.map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
    </select>
  );
  return (
    <>
      {needed.map((f) => {
        switch (f) {
          case "industry": return select(f, "Industry", fs.options("industry"));
          case "source": return select(f, "Source", fs.options("candidate_source"));
          case "noticePeriod": return select(f, "Notice period", fs.options("notice_period"));
          case "workAuthorization": return select(f, "Work authorization", keysOf(WORK_AUTH_STATUS).map((k) => ({ key: k, label: WORK_AUTH_STATUS[k] })));
          default: return <input key={f} aria-label={TEXT[f]} placeholder={TEXT[f]} value={values[f] ?? ""} onChange={set(f)} />;
        }
      })}
    </>
  );
}

/** The extras as request fields (empty ones left out, so the API names what is missing). */
export function extrasBody(values: Record<string, string>): Body {
  const out: Body = {};
  for (const [f, raw] of Object.entries(values)) {
    const v = raw.trim();
    if (!v) continue;
    if (f === "noticePeriod") {
      const [value = "0", unit = "month"] = v.split("_");
      out.noticePeriod = { value: Number(value), unit: unit as Schemas["NoticePeriodUnit"] };
    } else if (f === "workAuthorization") {
      out.workAuthorization = { status: v as Schemas["WorkAuthorizationStatus"], basis: null, note: null };
    } else if (f === "nationality") {
      out.nationality = v.toUpperCase();
    } else {
      (out as Record<string, string>)[f] = v;
    }
  }
  return out;
}
