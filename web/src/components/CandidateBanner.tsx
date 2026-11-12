// Availability colours the card; work authorization is edited in Details.
import { useState } from "react";
import type { Candidate } from "../api/client";
import { useCandidateUpdate } from "./useCandidateUpdate";
import { formatNotice, regionName } from "../labels";
import {
  AVAILABILITY, type Availability, jobLine, NO_STATUS_BG, raiseText, rangeText, salaryText, sinceText,
  WA_KIND, waColumnLabel, waKind,
} from "./candidateLook";
import { ErrorMessage } from "./ErrorMessage";
import { useFieldSettings } from "./useFieldSettings";

const SHOWN_SKILLS = 6;

export function CandidateBanner({ candidate: c, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const fs = useFieldSettings();
  const [expanded, setExpanded] = useState(false);
  const save = useCandidateUpdate(c);
  const status = c.availability ? AVAILABILITY[c.availability] : null;
  const kind = waKind(c.workAuthorization);
  const lives = livesIn(c.location, c.residenceCountry ? regionName(c.residenceCountry) ?? c.residenceCountry : null);
  const comp = c.compensation;
  const restricted = c.restrictedFields.includes("compensation");
  const current = comp?.currentSalary ? salaryText(comp.currentSalary) : null;
  const expected = comp?.expectedSalary ? rangeText(comp.expectedSalary) : null;

  return (
    <section className="banner candidate-banner" aria-label="Candidate" style={{ background: status?.bg ?? NO_STATUS_BG }}>
      <div className="banner-body">
        <div className="banner-top">
          <div className="banner-who">
            <div className="row">
              <h2 className="banner-name">{c.fullName}</h2>
              {canEdit ? <select className="banner-status" aria-label="Availability" value={c.availability ?? ""} disabled={save.isPending}
                onChange={e => { save.mutate({ availability: (e.target.value || null) as Availability | null }); }}>
                <option value="">Not set</option>
                {(Object.keys(AVAILABILITY) as Availability[]).map(a => <option key={a} value={a}>{AVAILABILITY[a].label}</option>)}
              </select> : status && <span className="banner-status">{status.label}</span>}
              {status && sinceText(c.availabilitySince) && <span className="banner-since">{sinceText(c.availabilitySince)}</span>}
            </div>
            {jobLine(c) && <p className="banner-job">{jobLine(c)}</p>}
          </div>
          <dl className="banner-facts">
            <div><dt>Nationality</dt><dd>{c.nationality ? regionName(c.nationality) ?? c.nationality : "—"}</dd></div>
            <div><dt>Lives in</dt><dd>{lives || "NA"}</dd></div>
            <div><dt>Source</dt><dd>{c.source ? fs.label("candidate_source", c.source) : "—"}</dd></div>
          </dl>
        </div>

        {c.skills.length > 0 && (
          <div className="banner-skills">
            <span>{(expanded ? c.skills : c.skills.slice(0, SHOWN_SKILLS)).join(" · ")}</span>
            {c.skills.length > SHOWN_SKILLS && <button type="button" className="link" aria-expanded={expanded}
              onClick={() => { setExpanded(!expanded); }}>{expanded ? "Fewer skills" : `+${c.skills.length - SHOWN_SKILLS} skills`}</button>}
          </div>
        )}

        <div className="banner-strip">
          <div><span className="label">Notice period</span><span className="figure">{formatNotice(c.noticePeriod)}</span></div>
          <div>
            <span className="label">Current salary</span>
            {restricted ? <span className="figure small">Restricted</span>
              : current ? <span className="figure">{current[0]} <small>{current[1]}</small></span> : <span className="figure">—</span>}
          </div>
          <div>
            <span className="label">Expected salary</span>
            {restricted ? <span className="figure small">Restricted</span>
              : expected ? <span className="figure">{expected[0]} <small>{expected[1]}</small></span> : <span className="figure">—</span>}
            {!restricted && raiseText(comp) && <span className="note">{raiseText(comp)}</span>}
          </div>
        </div>

        <ErrorMessage error={save.error} />
      </div>
      <div className="banner-column" style={{ background: WA_KIND[kind].color }}>
        <span><span className="visually-hidden">Work authorization: </span>{waColumnLabel(c.workAuthorization)}</span>
      </div>
    </section>
  );
}

/** "Petaling Jaya, Malaysia"; the country once when the location already names it. */
export function livesIn(location: string | null, country: string | null): string {
  if (location && country && location.toLowerCase().includes(country.toLowerCase())) return location;
  return [location, country].filter(Boolean).join(", ");
}
