// How invitations go, on the Team page: just sent (by the server, your
// mailbox, or not at all), and later how each person's invitation email went.
import type { Schemas } from "../api/client";
import { formatWhen } from "../labels";

/** How the latest invitation email went: waiting, sent, or failed with the mail server's answer. */
export function InvitationEmailState({ email }: { email: Schemas["InvitationEmail"] }) {
  switch (email.status) {
    case "queued": return <div className="team-invite-mail muted small">Invitation email waiting to be sent</div>;
    case "sent": return <div className="team-invite-mail muted small">Invitation email sent {formatWhen(email.at)}</div>;
    case "failed": return (
      <div className="team-invite-mail small error" role="note">
        <strong>Invitation email failed</strong>{email.error ? <>: {email.error}</> : null}. Send them the link from Send new invitation, or check the server&apos;s email settings.
      </div>
    );
  }
}

/** How an invitation went, in words, and what to do when it cannot arrive. */
export function InvitationSent({ email, link, delivery, onDone }: { email: string; link: string; delivery: Schemas["Invitation"]["delivery"]; onDone: () => void }) {
  const lead = {
    server: <>Invitation emailed to {email}. If it does not arrive, send them this link (it works for 7 days):</>,
    mailbox: <>Invitation emailed to {email} from your own mailbox: the server&apos;s email is not set up. If it does not arrive, send them this link (it works for 7 days):</>,
    test_inbox: <><strong>Not emailed:</strong> the server&apos;s email is only a test inbox, so nothing reaches {email}. Send them this link yourself (it works for 7 days):</>,
    none: <><strong>Not emailed:</strong> the server&apos;s email is not set up and you have no mailbox connected. Send them this link yourself (it works for 7 days):</>,
  }[delivery];
  const fix = delivery === "test_inbox" || delivery === "none";
  return (
    <div className={fix ? "warning" : "team-sent"} role="status">
      <p>{lead}</p>
      <code className="secret">{link}</code>{" "}
      <button type="button" className="link" onClick={() => {
        try { void navigator.clipboard.writeText(link); } catch { /* no clipboard here: the link is shown to copy by hand */ }
      }}>Copy link</button>{" "}
      <button type="button" className="link" onClick={onDone}>Done</button>
      {fix && (
        <p className="muted small">
          To have invitations emailed, connect your mailbox in <a href="#/email-connection">Settings › Email connection</a> and send a new
          invitation, or have whoever runs the server set up its email (HP_SMTP_* settings).
        </p>
      )}
    </div>
  );
}
