import { useState } from "react";
import type { Candidate } from "../api/client";
import { WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";
import type React from "react";

import { WA_KIND, waColumnLabel, waFor, waKind, type WaKind } from "./candidateLook";
import { ErrorMessage } from "./ErrorMessage";
import { useCandidateUpdate } from "./useCandidateUpdate";

export function WorkAuthorizationPicker({ candidate, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const [choosingPass, setChoosingPass] = useState(false);
  const save = useCandidateUpdate(candidate);
  const kind = waKind(candidate.workAuthorization);
  // Read from their CV while none is set: shown striped until someone confirms it.
  const suggestion = !candidate.workAuthorization ? candidate.workAuthorizationSuggestion ?? null : null;
  const suggested = suggestion ? waKind({ status: suggestion.status, basis: suggestion.basis, note: null }) : null;
  const suggestedPass = suggestion?.basis === "rp_t" || suggestion?.basis === "spouse_visa" ? suggestion.basis : undefined;
  const pick = (choice: WaKind, pass?: "rp_t" | "spouse_visa") => {
    if (choice === "expat" && !pass) { setChoosingPass(true); return; }
    save.mutate({ workAuthorization: waFor(choice, candidate.workAuthorization, pass) }, { onSuccess: () => { setChoosingPass(false); } });
  };
  const hint = suggestion && suggested && (
    <span className="wa-suggested-chip" style={{ "--wa": WA_KIND[suggested].color } as React.CSSProperties} title={suggestion.reason}>
      {waColumnLabel({ status: suggestion.status, basis: suggestion.basis, note: null })} · suggested from the CV
    </span>
  );
  if (!canEdit) {
    const value = candidate.workAuthorization;
    return <>{value ? `${WORK_AUTH_STATUS[value.status]}${value.status === "expat_eligible" && value.basis ? `: ${WORK_AUTH_BASIS[value.basis]}` : ""}` : hint || "—"}</>;
  }
  return <div className="work-authorization-picker">
    <div className="work-authorization-options" role="group" aria-label="Work authorization">
      {(Object.keys(WA_KIND) as WaKind[]).map((choice) => {
        const isSuggested = suggested === choice && choice !== "unknown";
        return <button key={choice} type="button" aria-pressed={kind === choice && !isSuggested}
          className={isSuggested ? "wa-suggested" : undefined} style={isSuggested ? { "--wa": WA_KIND[choice].color } as React.CSSProperties : undefined}
          title={isSuggested && suggestion ? `Suggested from the CV (${suggestion.reason}): click to confirm` : undefined}
          disabled={save.isPending} onClick={() => { pick(choice, isSuggested ? suggestedPass : undefined); }}>{WA_KIND[choice].label}</button>;
      })}
    </div>
    {suggestion && suggested !== "unknown" && <p className="wa-suggestion-note muted small">Striped: suggested from the CV ({suggestion.reason}). Click it to confirm.</p>}
    {choosingPass && <div className="row pass-choices" role="group" aria-label="Pass held">
      <span>Pass held</span>
      <button type="button" disabled={save.isPending} onClick={() => { pick("expat", "rp_t"); }}>RP-T</button>
      <button type="button" disabled={save.isPending} onClick={() => { pick("expat", "spouse_visa"); }}>Spouse visa</button>
      <button type="button" className="link" onClick={() => { setChoosingPass(false); }}>Cancel</button>
    </div>}
    <ErrorMessage error={save.error}/>
  </div>;
}
