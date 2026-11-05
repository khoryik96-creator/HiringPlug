// The company's dropdown options and mandatory fields (Settings), for forms
// and pages. Until they load, the built-in options are used.
import { useQuery } from "@tanstack/react-query";

import { api, type Schemas, unwrap } from "../api/client";
import { fieldSettingsKey } from "../api/keys";
import { CANDIDATE_SOURCE, WORK_ARRANGEMENT, WORK_TYPE } from "../labels";

type ListName = Schemas["FieldListName"];
type Settings = Schemas["FieldSettings"];
export type Option = { key: string; label: string };

const BUILT_IN: Partial<Record<ListName, Record<string, string>>> = {
  candidate_source: CANDIDATE_SOURCE, work_type: WORK_TYPE, work_arrangement: WORK_ARRANGEMENT,
};

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
    list(name)?.options.find((o) => o.key === key)?.label ?? BUILT_IN[name]?.[key] ?? key;
  /** Options to choose from: the visible ones, plus current if it is hidden or no longer listed. */
  const options = (name: ListName, current?: string | null): Option[] => {
    const l = list(name);
    const all: Option[] = l
      ? l.options.filter((o) => !o.hidden || o.key === current)
      : Object.entries(BUILT_IN[name] ?? {}).map(([key, text]) => ({ key, label: text }));
    if (current && !all.some((o) => o.key === current)) all.push({ key: current, label: label(name, current) });
    return all;
  };
  /** Whether a field must be filled in on a form (the app's own required ones included). */
  const mandatory = (form: Schemas["FieldForm"], field: string): boolean =>
    data?.mandatory.some((m) => m.form === form && m.field === field && m.mandatory) ?? false;
  return { options, label, mandatory };
}

/** A label with " *" when the field is mandatory. */
export function labelFor(label: string, mandatory: boolean): string {
  return mandatory ? `${label} *` : label;
}

/** A notice period's key (as in the settings list): 2_week. */
export function noticeKey(value: string | number, unit: string): string {
  return `${String(value)}_${unit}`;
}
