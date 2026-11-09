// The company's dropdown options and mandatory fields (Settings), for forms
// and pages. Until they load, the built-in options are used.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { fieldSettingsKey } from "../api/keys";
import { CANDIDATE_SOURCE, formatNotice, NOTICE_UNIT, sourceLabel, WORK_ARRANGEMENT, WORK_TYPE } from "../labels";

type ListName = Schemas["FieldListName"];
type Settings = Schemas["FieldSettings"];
export type Option = { key: string; label: string };

/** The app's own options, used until the company's load (the API's defaults, in settings/domain.go). */
const BUILT_IN: Record<ListName, Record<string, string>> = {
  candidate_source: CANDIDATE_SOURCE,
  notice_period: { "0_day": "Immediately", "1_week": "1 week", "2_week": "2 weeks", "1_month": "1 month", "2_month": "2 months", "3_month": "3 months" },
  industry: Object.fromEntries([
    "Digital & E-Commerce", "Financial Services", "FMCG", "Industrial/Manufacturing", "Information Technology & Services",
    "Life Science/Medical", "Property & Construction", "Professional Services", "Education", "Government Sector",
  ].map((i) => [i, i])),
  work_type: WORK_TYPE,
  work_arrangement: WORK_ARRANGEMENT,
  screening_call: { rfl: "RFL", lf: "LF" },
  reference_check: {
    referee: "Referee (name, title, company, contact)", relationship: "How they worked together, and for how long",
    role: "Role and responsibilities", strengths: "Strengths", improve: "Areas to improve", reason_left: "Reason for leaving",
    rehire: "Would you rehire them?",
  },
  company_benefits: {
    job_grade: "Job grade/level", annual_leave: "Annual leave", other_leave: "Miscellaneous leave", medical: "Medical and insurance",
    work_mode: "Work mode", working_hours: "Working hours and flexibility", dental_optical: "Dental & optical", fixed_bonus: "Fixed bonus",
    performance_bonus: "Performance bonus", other_bonus: "Miscellaneous bonus", commission: "Commission structure", special: "Special benefits",
  },
  pre_offer_process: {
    reference_check: "Reference check", background_check: "Background check", health_screening: "Health screening",
    payslips: "Payslips + bonus slips", identity: "NRIC/passport (expat)", education: "Highest education certificate", additional: "Additional process",
  },
  company_category: { signed_client: "Signed client", prospective_client: "Prospective client", unknown: "Unknown", vip_client: "VIP client" },
};

/** The built-in categories' colours, used until the company's load. */
const BUILT_IN_COLOR: Record<string, string> = {
  signed_client: "#16a34a", prospective_client: "#2563eb", unknown: "#6b7280", vip_client: "#b8860b",
};

/** A notice period key (2_week) in words, for one no longer in the list. */
function noticeWords(key: string): string {
  const [value, unit] = key.split("_");
  return unit && unit in NOTICE_UNIT ? formatNotice({ value: Number(value), unit: unit as Schemas["NoticePeriodUnit"] }) : key;
}

/** The settings, and helpers that read them. */
export function useFieldSettings() {
  const data = useQuery({
    queryKey: fieldSettingsKey,
    queryFn: async () => unwrap(await api.GET("/v1/settings/fields")),
    staleTime: 60_000,
  }).data;
  return fieldHelpers(data);
}

export function fieldHelpers(data: Settings | undefined) {
  const list = (name: ListName) => data?.lists.find((l) => l.name === name);
  /** A stored value in words. */
  const label = (name: ListName, key: string): string =>
    list(name)?.options.find((o) => o.key === key)?.label ?? BUILT_IN[name][key]
      ?? (name === "notice_period" ? noticeWords(key) : name === "candidate_source" ? sourceLabel(key) : key);
  /** Options to choose from: the visible ones, plus current if it is hidden or no longer listed. */
  const options = (name: ListName, current?: string | null): Option[] => {
    const l = list(name);
    const all: Option[] = l
      ? l.options.filter((o) => !o.hidden || o.key === current)
      : Object.entries(BUILT_IN[name]).map(([key, text]) => ({ key, label: text }));
    if (current && !all.some((o) => o.key === current)) all.push({ key: current, label: label(name, current) });
    return all;
  };
  /** Whether a field must be filled in on a form (the app's own required ones included). */
  const mandatory = (form: Schemas["FieldForm"], field: string): boolean =>
    data?.mandatory.some((m) => m.form === form && m.field === field && m.mandatory) ?? false;
  /** An option's colour (lists with colours), grey when it has none. */
  const color = (name: ListName, key: string): string =>
    list(name)?.options.find((o) => o.key === key)?.color ?? BUILT_IN_COLOR[key] ?? "#6b7280";
  return { options, label, mandatory, color };
}

/** A label with " *" when the field is mandatory. */
export function labelFor(label: string, mandatory: boolean): string {
  return mandatory ? `${label} *` : label;
}

/** A notice period's key (as in the settings list): 2_week. */
export function noticeKey(value: string | number, unit: string): string {
  return `${String(value)}_${unit}`;
}
