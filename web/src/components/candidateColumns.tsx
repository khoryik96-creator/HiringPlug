// The candidate list's columns: what each shows, and the picker that lets
// each person choose theirs (saved for them, on any computer).
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { countryName, formatNotice } from "../labels";
import { AVAILABILITY, rangeText, salaryText, sinceText, statusLook, WA_KIND, waColumnLabel, waKind } from "./candidateLook";
import { Chips } from "./LabelInput";
import { useFieldSettings } from "./useFieldSettings";

export type Column = Schemas["CandidateColumn"];
type Summary = Schemas["CandidateSummary"];
type Look = { label: (list: "candidate_source", key: string) => string };

export const preferencesKey = ["me", "preferences"] as const;
const saveColumnsKey = ["save-candidate-columns"] as const;

/** What a list shows until its person chooses. */
// Work authorization is not one: it shows under each name, in colour.
export const DEFAULT_COLUMNS: Column[] = ["currentRole", "status", "noticePeriod", "currentSalary", "expectedSalary", "email"];

const restricted = <span className="muted">Restricted</span>;
const pair = (p: [string, string] | null) => p && <>{p[0]} <span className="muted small">{p[1]}</span></>;

export function workAuth(c: Summary): Schemas["WorkAuthorization"] | null {
  return c.workAuthorizationStatus ? { status: c.workAuthorizationStatus, basis: c.workAuthorizationBasis ?? null } : null;
}

export const COLUMNS: Record<Column, { header: string; cell: (c: Summary, look: Look) => ReactNode }> = {
  currentRole: { header: "Current role", cell: (c) => <>{c.currentTitle ?? c.currentCompany}{c.currentTitle && c.currentCompany && <span className="muted small">{c.currentCompany}</span>}</> },
  status: {
    header: "Status",
    cell: (c) => {
      const s = c.availability ? AVAILABILITY[c.availability] : null;
      const since = sinceText(c.availabilitySince, new Date(), true);
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
    mutationKey: saveColumnsKey,
    // Saves run one after another, so the last choice is the one kept.
    scope: { id: "candidate-columns" },
    mutationFn: async (candidateColumns: Column[] | null) => unwrap(await api.PUT("/v1/me/preferences", { body: { candidateColumns } })),
    // The list changes at once.
    onMutate: async (candidateColumns) => {
      await queryClient.cancelQueries({ queryKey: preferencesKey });
      queryClient.setQueryData(preferencesKey, (old: Schemas["Preferences"] | undefined) => ({ ...old, candidateColumns }));
    },
    // Once the last save is done (or one failed), show what is saved.
    onSettled: async () => {
      if (queryClient.isMutating({ mutationKey: saveColumnsKey }) <= 1) await queryClient.invalidateQueries({ queryKey: preferencesKey });
    },
  });
  const fs = useFieldSettings();
  // ready: the person's own columns are known (or could not be read), so a list can tell whether it shows salaries.
  return { columns: prefs.data?.candidateColumns ?? DEFAULT_COLUMNS, chosen: prefs.data?.candidateColumns != null, ready: !prefs.isPending, save, look: fs };
}

/** A name's initials: "Nurul Huda" -> "NH". */
export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  return ((words[0]?.[0] ?? "") + (words.length > 1 ? words[words.length - 1]?.[0] ?? "" : "")).toUpperCase() || "?";
}

/** A candidate in a list: their initials in a disc of their status colour, their name, and their work authorization (in its colour) under it. */
export function WaName({ c, href, children }: { c: Summary; href: string; children?: React.ReactNode }) {
  const wa = workAuth(c);
  const kind = WA_KIND[waKind(wa)];
  const label = wa ? waColumnLabel(wa) : kind.label;
  return (
    <span className="wa-who">
      <StatusDisc c={c} named />
      <span className="wa-name">
        <span><a href={href}>{c.fullName}</a>{children}</span>
        <small className="wa-label" style={{ color: kind.color }}>{label}</small>
      </span>
    </span>
  );
}

/** Their initials in a disc of their status's colours (Looking out, Placed...), as their page's banner; named: the status said to screen readers too. */
export function StatusDisc({ c, named = false, big = false }: { c: Summary; named?: boolean; big?: boolean }) {
  const look = statusLook(c.availability);
  const said = named ? { role: "img", "aria-label": `Status: ${look.label}`, title: `Status: ${look.label}` } : { "aria-hidden": true };
  return (
    <span className={big ? "wa-disc big" : "wa-disc"} style={{ background: look.bg, color: "#1F2937", boxShadow: `0 0 0 2px #fff, 0 0 0 4px ${look.edge}` }} {...said}>
      {initials(c.fullName)}
    </span>
  );
}

/** Choose and reset the columns (they are put in order by dragging their headings). */
export function ColumnPicker({ columns, chosen, onSave }: { columns: Column[]; chosen: boolean; onSave: (c: Column[] | null) => void }) {
  // A small icon at the end of the folder tabs (listTop.css).
  const [open, setOpen] = useState(false);
  const toggle = (col: Column, on: boolean) => { onSave(on ? [...columns, col] : columns.filter((c) => c !== col)); };
  return (
    <div className="column-picker">
      <button type="button" className="tool-btn" aria-label="Columns" title="Choose the columns" aria-expanded={open} onClick={() => { setOpen(!open); }}>
        <svg viewBox="0 0 16 16" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="1.6"><rect x="2" y="2.5" width="12" height="11" rx="1.5" /><path d="M6.2 2.5v11M9.8 2.5v11" /></svg>
      </button>
      {open && (
        <div className="panel column-menu" role="group" aria-label="Columns">
          <p className="small muted">Shown after the name. To change the order, drag a column&rsquo;s heading in the list.</p>
          {[...columns, ...ALL.filter((c) => !columns.includes(c))].map((col) => (
            <div key={col} className="column-choice">
              <label><input type="checkbox" checked={columns.includes(col)} onChange={(e) => { toggle(col, e.target.checked); }} /> {COLUMNS[col].header}</label>
            </div>
          ))}
          <div className="row">
            <button type="button" className="link" disabled={!chosen} onClick={() => { onSave(null); }}>Back to the default</button>
            <button type="button" className="link" onClick={() => { setOpen(false); }}>Done</button>
          </div>
        </div>
      )}
    </div>
  );
}
