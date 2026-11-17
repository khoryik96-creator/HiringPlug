// On a new contact's form: their LinkedIn profile fills in their name and job
// title, and offers the companies in HiringPlug that may be where they work.
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { companionAtLeast, useCompanion } from "../companion";
import { linkedinDates } from "../labels";
import { type CompanyMatch, companiesFor, nameParts, profilePage, type ProfileRead, readProfile, roleAt } from "./contactLinkedin";
import { companyPage } from "./linkedinCompany";
import type { PickedCompany } from "./CompanyPicker";

const recent = (v: string | null) => companionAtLeast(v, "1.3.0");

export function ContactFromLinkedin({ url, company, onRead, onCompany }: {
  url: string;
  company: PickedCompany | null;
  /** What to fill in: only what is empty is filled by the form. */
  onRead: (fill: { firstName: string; lastName: string; title: string; linkedinUrl: string }) => void;
  onCompany: (c: PickedCompany) => void;
}) {
  const companion = useCompanion();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [read, setRead] = useState<ProfileRead | null>(null);
  const [matches, setMatches] = useState<CompanyMatch[] | null>(null);
  const [at, setAt] = useState<string | null>(null); // the chosen company's LinkedIn page
  const page = profilePage(url);

  const fill = async () => {
    setBusy(true); setError(""); setMatches(null);
    try {
      const r = await readProfile(page);
      // Opened at a company: their role there (they may hold several).
      const linkedin = company
        ? unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: company.id } } })).linkedinUrl
        : null;
      setRead(r); setAt(linkedin ?? null);
      const role = roleAt(r.roles, linkedin);
      onRead({ ...nameParts(r.name), title: role?.title ?? "", linkedinUrl: page });
      if (role) setMatches(await companiesFor(role));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const role = read ? roleAt(read.roles, at) : undefined;
  const elsewhere = !!company && !!at && !!role && companyPage(at) !== role.company_url;
  return (
    <div className="full from-linkedin">
      <button type="button" disabled={!page || busy || companion.ready !== true || !recent(companion.version)} onClick={() => { void fill(); }}
        title={companion.ready ? "Your browser reads their LinkedIn profile" : "Install the LinkedIn companion first"}>
        {busy ? "Reading LinkedIn…" : "Fill in from LinkedIn"}
      </button>
      {companion.ready === true && !recent(companion.version) && <span className="muted small"> Update the LinkedIn companion to 1.3 to fill in from LinkedIn.</span>}
      {error && <p className="warning small" role="alert">{error}</p>}
      {read && (
        <div className="linkedin-read" role="status">
          {role
            ? <span>LinkedIn: <strong>{role.title || "a role"}</strong> at <strong>{role.company_name}</strong> since {linkedinDates(role.since)}</span>
            : <span className="muted">LinkedIn shows no current role.</span>}
          {elsewhere && <span className="warning small">Not at {company.name} on LinkedIn now: check before adding them here.</span>}
          {matches && !company && (matches.length > 0
            ? <span className="row">Their company: {matches.slice(0, 4).map((m) => (
              <button key={m.id} type="button" className="doc-btn" onClick={() => { onCompany({ id: m.id, name: m.name }); }}>
                {m.name}{m.samePage ? " (same LinkedIn page)" : ""}
              </button>
            ))}</span>
            : <span className="muted small">No company in HiringPlug matches {role?.company_name}: add it under Companies first.</span>)}
        </div>
      )}
    </div>
  );
}
