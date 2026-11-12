import { useState } from "react";
import type { Candidate } from "../api/client";
import { WORK_AUTH_BASIS, WORK_AUTH_STATUS } from "../labels";
import { WA_KIND, waFor, waKind, type WaKind } from "./candidateLook";
import { ErrorMessage } from "./ErrorMessage";
import { useCandidateUpdate } from "./useCandidateUpdate";

export function WorkAuthorizationPicker({ candidate, canEdit }: { candidate: Candidate; canEdit: boolean }) {
  const [choosingPass, setChoosingPass] = useState(false);
  const save = useCandidateUpdate(candidate);
  const kind = waKind(candidate.workAuthorization);
  const pick = (choice: WaKind, pass?: "rp_t" | "spouse_visa") => {
    if (choice === "expat" && !pass) { setChoosingPass(true); return; }
    save.mutate({ workAuthorization: waFor(choice, candidate.workAuthorization, pass) }, { onSuccess: () => { setChoosingPass(false); } });
  };
  if (!canEdit) {
    const value = candidate.workAuthorization;
    return <>{value ? `${WORK_AUTH_STATUS[value.status]}${value.status === "expat_eligible" && value.basis ? `: ${WORK_AUTH_BASIS[value.basis]}` : ""}` : "—"}</>;
  }
  return <div className="work-authorization-picker">
    <div className="work-authorization-options" role="group" aria-label="Work authorization">
      {(Object.keys(WA_KIND) as WaKind[]).map((choice) => <button key={choice} type="button" aria-pressed={kind === choice}
        disabled={save.isPending} onClick={() => { pick(choice); }}>{WA_KIND[choice].label}</button>)}
    </div>
    {choosingPass && <div className="row pass-choices" role="group" aria-label="Pass held">
      <span>Pass held</span>
      <button type="button" disabled={save.isPending} onClick={() => { pick("expat", "rp_t"); }}>RP-T</button>
      <button type="button" disabled={save.isPending} onClick={() => { pick("expat", "spouse_visa"); }}>Spouse visa</button>
      <button type="button" className="link" onClick={() => { setChoosingPass(false); }}>Cancel</button>
    </div>}
    <ErrorMessage error={save.error}/>
  </div>;
}
