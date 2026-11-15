import type { Schemas } from "../api/client";
import { formatWhen, MAILBOX_CHECK } from "../labels";

/** The result of asking a contact's mail server whether their mailbox exists. */
export function MailboxBadge({ check, detailed = false }: {
  check: Schemas["ContactMailboxCheck"] | Schemas["MailboxCheckStatus"] | null;
  detailed?: boolean;
}) {
  if (!check) return null;
  const status = typeof check === "string" ? check : check.status;
  const why = typeof check !== "string" && check.reason && status !== "checking" ? check.reason : MAILBOX_CHECK[status].meaning;
  return (
    <span className="mailbox-check">
      <span className={`badge mailbox-${status}`} title={why}>{MAILBOX_CHECK[status].badge}</span>
      {detailed && typeof check !== "string" && (
        <span className="muted">
          {" "}{why}
          {check.provider && status !== "checking" && <> ({check.provider})</>}
          {check.checkedAt && status !== "checking" && <> · checked {formatWhen(check.checkedAt)}</>}
          {check.recheckAt && <> · checking again at {formatWhen(check.recheckAt)}</>}
        </span>
      )}
    </span>
  );
}
