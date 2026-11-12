import type { ContactSummary } from "../api/client";
import { EmailCheckBadge } from "./EmailCheckBadge";
import { pickRowProps } from "./pickRow";
import { PinnedScroll } from "./StickyScrollbar";

/** Contacts in a table, linking to each contact (and their company, if shown). */
export function ContactTable({ contacts, showCompany = false, empty, picked, onPick }: {
  contacts: ContactSummary[];
  showCompany?: boolean;
  empty: string;
  /** Chosen, to email each separately (with onPick, clicking a row chooses it). */
  picked?: Set<string>;
  onPick?: (contacts: ContactSummary[], on: boolean) => void;
}) {
  const all = contacts.length > 0 && contacts.every((c) => picked?.has(c.id));
  return (
    <>
    {onPick && contacts.length > 0 && (
      <button type="button" className="link small" onClick={() => { onPick(contacts, !all); }}>{all ? "Unselect all shown" : "Select all shown"}</button>
    )}
    <PinnedScroll>
    <table className={onPick ? "pick-table" : undefined}>
      <thead>
        <tr>
          <th>Name</th>
          <th>Job title</th>
          {showCompany && <th>Company</th>}
          <th>Email</th>
          <th>Phone</th>
        </tr>
      </thead>
      <tbody>
        {contacts.map((c) => (
          <tr key={c.id} {...(onPick ? pickRowProps(picked?.has(c.id) ?? false, () => { onPick([c], !(picked?.has(c.id) ?? false)); }) : {})}>
            <td>
              <a href={`#/contacts/${c.id}`}>{c.fullName}</a>
              {onPick && picked?.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
              {c.status === "left" && <span className="muted"> (has left)</span>}
            </td>
            <td>{c.title}</td>
            {showCompany && <td><a href={`#/companies/${c.companyId}`}>{c.companyName}</a></td>}
            <td>{c.email} <EmailCheckBadge check={c.emailCheckStatus} /></td>
            <td>{c.mobile ?? c.phone}</td>
          </tr>
        ))}
        {contacts.length === 0 && (
          <tr><td colSpan={showCompany ? 5 : 4} className="muted">{empty}</td></tr>
        )}
      </tbody>
    </table>
    </PinnedScroll>
    </>
  );
}
