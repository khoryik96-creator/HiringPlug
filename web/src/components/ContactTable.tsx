import type { ContactSummary } from "../api/client";
import { EmailCheckBadge } from "./EmailCheckBadge";

/** Contacts in a table, linking to each contact (and their company, if shown). */
export function ContactTable({ contacts, showCompany = false, empty, picked, onPick }: {
  contacts: ContactSummary[];
  showCompany?: boolean;
  empty: string;
  /** Ticked, to email each separately (with onPick: the ticks show). */
  picked?: Set<string>;
  onPick?: (contacts: ContactSummary[], on: boolean) => void;
}) {
  const all = contacts.length > 0 && contacts.every((c) => picked?.has(c.id));
  return (
    <table>
      <thead>
        <tr>
          {onPick && (
            <th className="pick-col">
              <input type="checkbox" aria-label="Choose all shown" checked={all} onChange={(e) => { onPick(contacts, e.target.checked); }} />
            </th>
          )}
          <th>Name</th>
          <th>Job title</th>
          {showCompany && <th>Company</th>}
          <th>Email</th>
          <th>Phone</th>
        </tr>
      </thead>
      <tbody>
        {contacts.map((c) => (
          <tr key={c.id}>
            {onPick && (
              <td className="pick-col">
                <input type="checkbox" aria-label={`Choose ${c.fullName}`} checked={picked?.has(c.id) ?? false} onChange={(e) => { onPick([c], e.target.checked); }} />
              </td>
            )}
            <td>
              <a href={`#/contacts/${c.id}`}>{c.fullName}</a>
              {c.status === "left" && <span className="muted"> (has left)</span>}
            </td>
            <td>{c.title}</td>
            {showCompany && <td><a href={`#/companies/${c.companyId}`}>{c.companyName}</a></td>}
            <td>{c.email} <EmailCheckBadge check={c.emailCheckStatus} /></td>
            <td>{c.mobile ?? c.phone}</td>
          </tr>
        ))}
        {contacts.length === 0 && (
          <tr><td colSpan={(showCompany ? 5 : 4) + (onPick ? 1 : 0)} className="muted">{empty}</td></tr>
        )}
      </tbody>
    </table>
  );
}
