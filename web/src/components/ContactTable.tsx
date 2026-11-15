import type React from "react";

import type { ContactSummary } from "../api/client";
import { formatWhen, linkedinDates, timeAgo } from "../labels";
import { chipStyle } from "./companyCategory";
import { roleColor, roleLabel } from "./contactRole";
import { EmailCheckBadge } from "./EmailCheckBadge";
import { InFolders } from "./folders/FolderView";
import { MailboxBadge } from "./MailboxBadge";
import { pickRowProps } from "./pickRow";
import { PinnedScroll } from "./StickyScrollbar";
import { useFieldSettings } from "./useFieldSettings";

/** What LinkedIn showed of a contact's time at the company, under their status. */
function LinkedinNote({ e }: { e: NonNullable<ContactSummary["linkedinEmployment"]> }) {
  const text = e.current === null ? "LinkedIn: unclear" : e.current ? `LinkedIn: since ${linkedinDates(e.joined)}` : `LinkedIn: left ${linkedinDates(e.left)}`;
  return <span className="muted small linkedin-note" title={`Checked ${formatWhen(e.checkedAt)}`}>{text} · {timeAgo(e.checkedAt)}</span>;
}

/** Contacts in a table, linking to each contact (and their company, if shown). */
export function ContactTable({ contacts, showCompany = false, empty, picked, onPick, dragName, inFolders }: {
  contacts: ContactSummary[];
  showCompany?: boolean;
  empty: string;
  /** Chosen, to email each separately (with onPick, clicking a row chooses it). */
  picked?: Set<string>;
  onPick?: (contacts: ContactSummary[], on: boolean) => void;
  /** Lets a contact's name be dragged onto a folder. */
  dragName?: (id: string) => React.HTMLAttributes<HTMLElement> | undefined;
  /** With it, an "In folders" column. */
  inFolders?: (id: string) => string[];
}) {
  const fs = useFieldSettings();
  return (
    <>
    <PinnedScroll>
    <table className={onPick ? "contacts-table pick-table" : "contacts-table"}>
      <thead>
        <tr>
          <th>Name</th>
          <th>Role</th>
          <th>Status</th>
          <th>Job title</th>
          {showCompany && <th>Company</th>}
          <th>Email</th>
          <th>Phone</th>
          {inFolders && <th>In folders</th>}
        </tr>
      </thead>
      <tbody>
        {contacts.map((c) => {
          const pickProps = onPick ? pickRowProps(picked?.has(c.id) ?? false, () => { onPick([c], !(picked?.has(c.id) ?? false)); }) : undefined;
          const bounces = c.mailboxStatus === "invalid";
          return (
          <tr key={c.id} {...pickProps} className={[pickProps?.className, bounces ? "bounces" : ""].filter(Boolean).join(" ") || undefined}>
            <td {...dragName?.(c.id)}>
              <a href={`#/contacts/${c.id}`}>{c.fullName}</a>
              {onPick && picked?.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
            </td>
            <td>
              <span className="category-chip" style={chipStyle(roleColor(fs, c.role))}>{roleLabel(fs, c.role)}</span>
            </td>
            <td>
              {c.status === "left" ? <span className="status-chip left">Has left</span> : <span className="status-chip">Works there</span>}
              {c.linkedinEmployment && <LinkedinNote e={c.linkedinEmployment} />}
            </td>
            <td>{c.title}</td>
            {showCompany && <td><a href={`#/companies/${c.companyId}`}>{c.companyName}</a></td>}
            <td>
              {c.email}{" "}
              {/* A mailbox answer says more than the quick check of the address. */}
              {c.mailboxStatus ? <MailboxBadge check={c.mailboxStatus} /> : <EmailCheckBadge check={c.emailCheckStatus} />}
              {c.mailboxCheckedAt && c.mailboxStatus !== "checking" && (
                <span className="muted small freshness" title={`Mail server asked ${formatWhen(c.mailboxCheckedAt)}`}> {timeAgo(c.mailboxCheckedAt)}</span>
              )}
            </td>
            <td>{c.mobile ?? c.phone}</td>
            {inFolders && <td className="col-folders"><InFolders names={inFolders(c.id)} /></td>}
          </tr>
          );
        })}
        {contacts.length === 0 && (
          <tr><td colSpan={(showCompany ? 7 : 6) + (inFolders ? 1 : 0)} className="muted">{empty}</td></tr>
        )}
      </tbody>
    </table>
    </PinnedScroll>
    </>
  );
}
