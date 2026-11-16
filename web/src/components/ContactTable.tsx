import type React from "react";
import { useRef } from "react";

import type { ContactSummary } from "../api/client";
import { formatWhen, linkedinDates, timeAgo } from "../labels";
import { chipStyle } from "./companyCategory";
import { roleColor, roleLabel } from "./contactRole";
import { EmailCheckBadge } from "./EmailCheckBadge";
import { InFolders } from "./folders/FolderView";
import { MailboxBadge } from "./MailboxBadge";
import { pickRowProps } from "./pickRow";
import { PinnedScroll } from "./StickyScrollbar";
import { useColumnWidths } from "./useColumnWidths";
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
  const heads: [string, string][] = [
    ["name", "Name"], ["role", "Role"], ["status", "Status"], ["title", "Job title"],
    ...(showCompany ? [["company", "Company"] as [string, string]] : []), ["email", "Email"], ["phone", "Phone"],
    ...(inFolders ? [["folders", "In folders"] as [string, string]] : []),
  ];
  const tableRef = useRef<HTMLTableElement>(null);
  const widths = useColumnWidths(showCompany ? "contacts" : "company-contacts", heads.map(([k]) => k), tableRef);
  return (
    <>
    {widths.set && <div className="widths-reset"><button type="button" className="link small" onClick={widths.reset}>Reset column widths</button></div>}
    <PinnedScroll>
    <table ref={tableRef} className={`contacts-table${onPick ? " pick-table" : ""}${widths.set ? " sized" : ""}`} style={widths.tableStyle}>
      <thead>
        <tr>
          {heads.map(([k, label]) => <th key={k} data-column={k} style={widths.style(k)}>{label}{widths.handle(k, label)}</th>)}
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
