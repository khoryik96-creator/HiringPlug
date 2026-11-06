// The candidate page's header: a banner coloured by the candidate's status,
// with a column on its right coloured by their work authorization. Both are
// picked here and saved straight away (with the version shown, so someone
// else's newer change is never overwritten).
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { ApiError, api, type Candidate, type Schemas, unwrap } from "../api/client";
import { candidateKey, candidatesKey } from "../api/keys";
import { formatNotice, regionName } from "../labels";
import {
  AVAILABILITY, type Availability, jobLine, NO_STATUS_BG, raiseText, rangeText, salaryText,
  WA_KIND, waColumnLabel, waFor, waKind, type WaKind,
} from "./candidateLook";
import { ErrorMessage } from "./ErrorMessage";
import { useFieldSettings } from "./useFieldSettings";

const SHOWN_SKILLS = 8;

export function CandidateBanner({ candidate: c, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const [choosingPass, setChoosingPass] = useState(false);
  const save = useMutation({
    mutationFn: async (change: Pick<Schemas["CandidateUpdate"], "availability" | "workAuthorization">) =>
      unwrap(await api.PATCH("/v1/candidates/{candidateId}", { params: { path: { candidateId: c.id } }, body: { version: c.version, ...change } })),
    onSuccess: (updated) => {
      setChoosingPass(false);
      queryClient.setQueryData(candidateKey(c.id), updated);
      void queryClient.invalidateQueries({ queryKey: candidatesKey });
    },
    onError: (e) => {
      // Someone saved first: show their version, so the next click starts from it.
      if (e instanceof ApiError && e.code === "CONFLICT_STALE_VERSION") void queryClient.invalidateQueries({ queryKey: candidateKey(c.id) });
    },
  });
  const status = c.availability ? AVAILABILITY[c.availability] : null;
  const kind = waKind(c.workAuthorization);
  const pickWa = (k: WaKind) => {
    if (k === "expat") { setChoosingPass(true); return; }
    save.mutate({ workAuthorization: waFor(k, c.workAuthorization) });
  };
  const lives = livesIn(c.location, c.residenceCountry ? regionName(c.residenceCountry) ?? c.residenceCountry : null);
  const comp = c.compensation;
  const restricted = c.restrictedFields.includes("compensation");
  const current = comp?.currentSalary ? salaryText(comp.currentSalary) : null;
  const expected = comp?.expectedSalary ? rangeText(comp.expectedSalary) : null;

  return (
    <section className="banner" aria-label="Candidate" style={{ background: status?.bg ?? NO_STATUS_BG }}>
      <div className="banner-body">
        <div className="banner-top">
          <div className="banner-who">
            <div className="row">
              <h2 className="banner-name">{c.fullName}</h2>
              {status && <span className="banner-status">{status.label}</span>}
            </div>
            {jobLine(c) && <p className="banner-job">{jobLine(c)}</p>}
          </div>
          <dl className="banner-facts">
            <div><dt>Nationality</dt><dd>{c.nationality ? regionName(c.nationality) ?? c.nationality : "—"}</dd></div>
            <div><dt>Lives in</dt><dd>{lives || "NA"}</dd></div>
            <div><dt>Source</dt><dd>{c.source ? fs.label("candidate_source", c.source) : "—"}</dd></div>
          </dl>
        </div>

        {canEdit && (
          <div className="row" role="group" aria-label="Status">
            {(Object.keys(AVAILABILITY) as Availability[]).map((a) => (
              <button key={a} type="button" className="banner-pill" aria-pressed={c.availability === a} disabled={save.isPending}
                onClick={() => { save.mutate({ availability: c.availability === a ? null : a }); }}>
                <span className="dot" style={{ background: AVAILABILITY[a].bg, borderColor: AVAILABILITY[a].edge }} />
                {AVAILABILITY[a].label}
              </button>
            ))}
          </div>
        )}

        {c.skills.length > 0 && (
          <div className="row banner-skills">
            {c.skills.slice(0, SHOWN_SKILLS).map((s) => <span key={s} className="banner-chip">{s}</span>)}
            {c.skills.length > SHOWN_SKILLS && <span className="banner-chip">+{c.skills.length - SHOWN_SKILLS}</span>}
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

        {canEdit && (
          <div className="row" role="group" aria-label="Work authorization">
            {(Object.keys(WA_KIND) as WaKind[]).map((k) => (
              <button key={k} type="button" className="banner-pill" aria-pressed={kind === k} disabled={save.isPending}
                onClick={() => { pickWa(k); }}>
                <span className="dot square" style={{ background: WA_KIND[k].color, borderColor: WA_KIND[k].color }} />
                {WA_KIND[k].label}
              </button>
            ))}
            {choosingPass && (
              <span className="row" role="group" aria-label="Pass held">
                <span>Which pass?</span>
                <button type="button" onClick={() => { save.mutate({ workAuthorization: waFor("expat", c.workAuthorization, "rp_t") }); }}>RP-T</button>
                <button type="button" onClick={() => { save.mutate({ workAuthorization: waFor("expat", c.workAuthorization, "spouse_visa") }); }}>Spouse visa</button>
                <button type="button" className="link" onClick={() => { setChoosingPass(false); }}>Cancel</button>
              </span>
            )}
          </div>
        )}
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
