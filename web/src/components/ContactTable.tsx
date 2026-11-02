import type { ContactSummary } from "../api/client";
import { EmailCheckBadge } from "./EmailCheckBadge";

/** Contacts in a table, linking to each contact (and their company, if shown). */
export function ContactTable({ contacts, showCompany = false, empty }: {
  contacts: ContactSummary[];
  showCompany?: boolean;
  empty: string;
}) {
  return (
    <table>
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
          <tr key={c.id}>
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
          <tr><td colSpan={showCompany ? 5 : 4} className="muted">{empty}</td></tr>
        )}
      </tbody>
    </table>
  );
}
