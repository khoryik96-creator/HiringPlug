import type { Schemas } from "../api/client";
import { EMAIL_CHECK } from "../labels";

/** The result of a contact's email check, with the reasons as a tooltip and text. */
export function EmailCheckBadge({ check, detailed = false }: {
  check: Schemas["EmailCheck"] | Schemas["EmailCheckStatus"] | null;
  detailed?: boolean;
}) {
  if (!check) return null;
  const status = typeof check === "string" ? check : check.status;
  const reasons = typeof check === "string" ? [] : check.reasons;
  const why = reasons.length > 0 ? reasons.join("; ") : EMAIL_CHECK[status].meaning;
  return (
    <span>
      <span className={`badge ${status}`} title={why}>{EMAIL_CHECK[status].badge}</span>
      {detailed && <span className="muted"> {why}</span>}
    </span>
  );
}
