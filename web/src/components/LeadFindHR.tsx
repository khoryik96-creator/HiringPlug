// Leads › Find HR (option A): a side panel beside the list for one lead.
// Step 1 settles which company it is (LeadCompanyChoice); step 2 is the
// company's Find HR scan, laid out for the panel. A lead linked already
// starts at step 2, asking first for the company's LinkedIn page if it has none.
import { useQuery } from "@tanstack/react-query";
import { type CSSProperties, type KeyboardEvent, useEffect, useRef, useState } from "react";

import { api, type Permission, type Schemas, unwrap } from "../api/client";
import { companyKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { FindHR } from "./FindHR";
import { activeRuns, stopRun } from "./findHrRun";
import { nameTab, openTab } from "./openRecords";
import { LeadCompanyChoice } from "./LeadCompanyChoice";
import { CompanyLinkedin } from "./LeadCompanyLinkedin";
import { companyPage } from "./linkedinCompany";

type Lead = Schemas["JobLead"];

/** Render it with key={lead.id}: another lead starts again at its own step. `onSaved` gets the lead as saved or read again here. */
export function LeadFindHR({ lead, permissions, onClose, onSaved }: { lead: Lead; permissions: Permission[]; onClose: () => void; onSaved?: (lead: Lead) => void }) {
  // Linked here: step 2 at once, before the list is read again.
  const [linkedId, setLinkedId] = useState<string | null>(null);
  const companyId = linkedId ?? lead.companyId;
  const top = useUnderBars();
  const head = useRef<HTMLHeadingElement>(null);
  // Into view with the focus on its title (on a narrow screen the panel is above the list).
  useEffect(() => { head.current?.focus(); }, []);
  // Escape in the panel closes it, but not while typing in it or in a dialog opened from it (Apollo).
  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key !== "Escape" || e.defaultPrevented) return;
    const t = e.target instanceof Element ? e.target : null;
    if (document.querySelector("dialog[open]") || t?.closest("dialog, input, textarea, select, [contenteditable='true']")) return;
    onClose();
  };

  return (
    <aside className="panel lead-hr" aria-label={`Find HR for ${lead.companyName}`} onKeyDown={onKeyDown}
      style={top === null ? undefined : { "--lead-hr-top": `${String(top + 12)}px` } as CSSProperties}>
      <div className="lead-hr-head">
        <h3 ref={head} tabIndex={-1}>Find HR · {lead.companyName}</h3>
        <button type="button" className="icon-action" aria-label="Close" onClick={onClose}>✕</button>
      </div>
      {companyId ? <LinkedCompany id={companyId} permissions={permissions} />
        : <LeadCompanyChoice lead={lead} permissions={permissions} onLinked={setLinkedId} onSaved={onSaved} />}
    </aside>
  );
}

/**
 * The height of the bars kept at the top as the page scrolls (the top bar, the open records), so the panel
 * sticks under them. The open records bar comes and goes (the first record opened, the last closed): watched for.
 */
function useUnderBars(): number | null {
  const [height, setHeight] = useState<number | null>(null);
  useEffect(() => {
    let bars: HTMLElement[] = [];
    const sizes = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => { measure(); });
    function measure() { setHeight(bars.reduce((sum, b) => sum + b.offsetHeight, 0)); }
    const find = () => {
      const now = [".topbar", ".open-tabs"].flatMap((s) => document.querySelector<HTMLElement>(s) ?? []);
      if (now.length === bars.length && now.every((b, i) => b === bars[i])) return;
      bars = now;
      sizes?.disconnect();
      bars.forEach((b) => { sizes?.observe(b); });
      measure();
    };
    find();
    measure();
    const added = typeof MutationObserver === "undefined" ? null : new MutationObserver(find);
    added?.observe(document.body, { childList: true, subtree: true });
    return () => { added?.disconnect(); sizes?.disconnect(); };
  }, []);
  return height;
}

function LinkedCompany({ id, permissions }: { id: string; permissions: Permission[] }) {
  const company = useQuery({
    queryKey: companyKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: id } } })),
  });
  // A scan started here has no Stop once the panel is gone (closed, or another lead's opened): it stops with
  // it. One already running when the panel opened (from the company's own page) goes on.
  useEffect(() => {
    const before = activeRuns.get(id);
    return () => {
      const run = activeRuns.get(id);
      if (run && run !== before) stopRun(run);
    };
  }, [id]);
  const c = company.data;
  if (!c) return company.error ? <ErrorMessage error={company.error} /> : <p className="muted small">Loading…</p>;
  const hasPage = companyPage(c.linkedinUrl ?? "") !== "";
  return (
    <>
      <p className="lead-hr-company small">
        <span className="muted">Company: </span>
        <a href={`#/companies/${c.id}`} title="Opens as a tab here; the panel stays"
          onClick={(e) => { e.preventDefault(); openTab(`#/companies/${c.id}`, "company"); nameTab(`#/companies/${c.id}`, c.name); }}>{c.name}</a>
        {c.city && <span className="muted"> · {c.city}</span>}
      </p>
      {!hasPage && <CompanyLinkedin company={c} canUpdate={permissions.includes("companies:update")} />}
      <div className="lead-hr-step" role="group" aria-label="Its HR people on LinkedIn">
        <h4>2 · Its HR people on LinkedIn</h4>
        {hasPage
          ? <FindHR company={c} canCreate={permissions.includes("contacts:create")} canUseApollo={permissions.includes("contacts.apollo:use")} compact />
          : <p className="muted small">Add its LinkedIn page first.</p>}
      </div>
    </>
  );
}
