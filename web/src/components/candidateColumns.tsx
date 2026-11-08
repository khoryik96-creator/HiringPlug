// The candidate list's columns: what each shows, and the picker that lets
// each person choose theirs (saved for them, on any computer).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { countryName, formatNotice } from "../labels";
import { AVAILABILITY, rangeText, salaryText, sinceText, WA_KIND, waColumnLabel, waKind } from "./candidateLook";
import { Chips } from "./LabelInput";
import { useFieldSettings } from "./useFieldSettings";

export type Column = Schemas["CandidateColumn"];
type Summary = Schemas["CandidateSummary"];
type Look = { label: (list: "candidate_source", key: string) => string };

export const preferencesKey = ["me", "preferences"] as const;

/** What a list shows until its person chooses. */
export const DEFAULT_COLUMNS: Column[] = ["currentRole", "status", "noticePeriod", "currentSalary", "expectedSalary", "workAuthorization", "email"];

const restricted = <span className="muted">Restricted</span>;
const pair = (p: [string, string] | null) => p && <>{p[0]} <span className="muted small">{p[1]}</span></>;

export function workAuth(c: Summary): Schemas["WorkAuthorization"] | null {
  return c.workAuthorizationStatus ? { status: c.workAuthorizationStatus, basis: c.workAuthorizationBasis ?? null } : null;
}

export const COLUMNS: Record<Column, { header: string; cell: (c: Summary, look: Look) => ReactNode }> = {
  currentRole: { header: "Current role", cell: (c) => [c.currentTitle, c.currentCompany].filter(Boolean).join(", ") },
  status: {
    header: "Status",
    cell: (c) => {
      const s = c.availability ? AVAILABILITY[c.availability] : null;
      const since = sinceText(c.availabilitySince);
      return s && <><span className="status-dot" style={{ background: s.bg, borderColor: s.edge }} />{s.label}{since && <span className="muted small">{since}</span>}</>;
    },
  },
  noticePeriod: { header: "Notice period", cell: (c) => (c.noticePeriod ? formatNotice(c.noticePeriod) : "") },
  currentSalary: { header: "Current salary", cell: (c) => (c.salaryRestricted ? restricted : pair(c.currentSalary ? salaryText(c.currentSalary) : null)) },
  expectedSalary: { header: "Expected salary", cell: (c) => (c.salaryRestricted ? restricted : pair(c.expectedSalary ? rangeText(c.expectedSalary) : null)) },
  workAuthorization: { header: "Work authorization", cell: (c) => (c.workAuthorizationStatus ? waColumnLabel(workAuth(c)) : "") },
  nationality: { header: "Nationality", cell: (c) => (c.nationality ? countryName(c.nationality) : "") },
  residence: { header: "Residence", cell: (c) => (c.residenceCountry ? countryName(c.residenceCountry) : "") },
  location: { header: "Location", cell: (c) => c.location ?? "" },
  source: { header: "Source", cell: (c, look) => (c.source ? look.label("candidate_source", c.source) : "") },
  industry: { header: "Industry", cell: (c) => c.industry ?? "" },
  tags: { header: "Tags", cell: (c) => c.tags.length > 0 && <Chips labels={c.tags} /> },
  email: { header: "Email", cell: (c) => c.email ?? "" },
  phone: { header: "Phone", cell: (c) => c.phone ?? "" },
  updated: { header: "Updated", cell: (c) => new Date(c.updatedAt).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" }) },
};

const ALL = Object.keys(COLUMNS) as Column[];

/** The signed-in person's columns, and saving a new choice (null: back to the default). */
export function useCandidateColumns() {
  const queryClient = useQueryClient();
  const prefs = useQuery({ queryKey: preferencesKey, queryFn: async () => unwrap(await api.GET("/v1/me/preferences")), staleTime: 60_000 });
  const save = useMutation({
    mutationFn: async (candidateColumns: Column[] | null) => unwrap(await api.PUT("/v1/me/preferences", { body: { candidateColumns } })),
    // The list changes at once; a failed save puts the earlier choice back.
    onMutate: async (candidateColumns) => {
      await queryClient.cancelQueries({ queryKey: preferencesKey });
      const before = queryClient.getQueryData<Schemas["Preferences"]>(preferencesKey);
      queryClient.setQueryData(preferencesKey, { candidateColumns });
      return { before };
    },
    onError: (_e, _v, context) => { queryClient.setQueryData(preferencesKey, context?.before); },
    onSuccess: (p) => { queryClient.setQueryData(preferencesKey, p); },
  });
  const fs = useFieldSettings();
  return { columns: prefs.data?.candidateColumns ?? DEFAULT_COLUMNS, chosen: prefs.data?.candidateColumns != null, save, look: fs };
}

/** The coloured bar beside a name: local, PR, RP-T / spouse, needs a pass, or not known. */
export function WaBar({ c }: { c: Summary }) {
  const wa = workAuth(c);
  const kind = WA_KIND[waKind(wa)];
  const label = wa ? waColumnLabel(wa) : kind.label;
  return <span className="wa-bar" style={{ background: kind.color }} title={`Work authorization: ${label}`} aria-label={`Work authorization: ${label}`} role="img" />;
}

export function WaLegend() {
  return (
    <p className="wa-legend small muted" aria-label="Colours beside the names">
      {(Object.keys(WA_KIND) as (keyof typeof WA_KIND)[]).map((k) => (
        <span key={k}><span className="wa-bar" style={{ background: WA_KIND[k].color }} />{WA_KIND[k].label}</span>
      ))}
    </p>
  );
}

/** Choose, order and reset the columns. */
export function ColumnPicker({ columns, chosen, onSave }: { columns: Column[]; chosen: boolean; onSave: (c: Column[] | null) => void }) {
  const [open, setOpen] = useState(false);
  const move = (col: Column, by: number) => {
    const next = columns.filter((c) => c !== col);
    const at = columns.indexOf(col) + by;
    if (at < 0 || at > next.length) return;
    next.splice(at, 0, col);
    onSave(next);
  };
  const toggle = (col: Column, on: boolean) => { onSave(on ? [...columns, col] : columns.filter((c) => c !== col)); };
  return (
    <div className="column-picker">
      <button type="button" aria-expanded={open} onClick={() => { setOpen(!open); }}>Columns</button>
      {open && (
        <div className="panel column-menu" role="group" aria-label="Columns">
          <p className="small muted">Shown after the name, in this order</p>
          {[...columns, ...ALL.filter((c) => !columns.includes(c))].map((col) => {
            const on = columns.includes(col);
            return (
              <div key={col} className="column-choice">
                <label><input type="checkbox" checked={on} onChange={(e) => { toggle(col, e.target.checked); }} /> {COLUMNS[col].header}</label>
                {on && (
                  <>
                    <button type="button" className="tool" aria-label={`Move ${COLUMNS[col].header} up`} disabled={columns.indexOf(col) === 0} onClick={() => { move(col, -1); }}>↑</button>
                    <button type="button" className="tool" aria-label={`Move ${COLUMNS[col].header} down`} disabled={columns.indexOf(col) === columns.length - 1} onClick={() => { move(col, 1); }}>↓</button>
                  </>
                )}
              </div>
            );
          })}
          <div className="row">
            <button type="button" className="link" disabled={!chosen} onClick={() => { onSave(null); }}>Back to the default</button>
            <button type="button" className="link" onClick={() => { setOpen(false); }}>Done</button>
          </div>
        </div>
      )}
    </div>
  );
}
