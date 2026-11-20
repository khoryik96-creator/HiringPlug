import type React from "react";
import { useRef } from "react";

import type { ContactSummary } from "../api/client";
import { formatWhen, linkedinDates, timeAgo } from "../labels";
import { chipStyle } from "./companyCategory";
import { roleColor, roleLabel } from "./contactRole";
import { EmailCheckBadge } from "./EmailCheckBadge";
import { InFolders } from "./folders/FolderView";
import { MailboxBadge } from "./MailboxBadge";
import { useRowPicking } from "./useRowPicking";
import { PinnedScroll } from "./StickyScrollbar";
import { useColumnMove, useStoredOrder } from "./useColumnMove";
import { useColumnWidths } from "./useColumnWidths";
import { useFieldSettings } from "./useFieldSettings";

/** What LinkedIn showed of a contact's time at the company, under their status. */
function LinkedinNote({ e }: { e: NonNullable<ContactSummary["linkedinEmployment"]> }) {
  const text = (e.current === null ? "LinkedIn: unclear" : e.current ? `LinkedIn: since ${linkedinDates(e.joined)}` : `LinkedIn: left ${linkedinDates(e.left)}`)
    + (e.matchedByName && e.current !== null ? " (by the company's name, not acted on)" : "");
  return <span className="muted small linkedin-note" title={`Checked ${formatWhen(e.checkedAt)}`}>{text} · {timeAgo(e.checkedAt)}</span>;
}

type FieldSettings = ReturnType<typeof useFieldSettings>;

/** The columns after the name, in the order they come: their heading and cell. */
const CELLS = {
  role: { label: "Role", cell: (c: ContactSummary, fs: FieldSettings) => (
    <span className="category-chip" style={chipStyle(roleColor(fs, c.role))}>{roleLabel(fs, c.role)}</span>
  ) },
  status: { label: "Status", cell: (c: ContactSummary) => (
    <>
      {c.status === "left" ? <span className="status-chip left">Has left</span> : <span className="status-chip">Works there</span>}
      {c.linkedinEmployment && <LinkedinNote e={c.linkedinEmployment} />}
    </>
  ) },
  title: { label: "Job title", cell: (c: ContactSummary) => c.title },
  company: { label: "Company", cell: (c: ContactSummary) => <a href={`#/companies/${c.companyId}`}>{c.companyName}</a> },
  email: { label: "Email", cell: (c: ContactSummary) => (
    <>
      {c.email}{" "}
      {/* A mailbox answer says more than the quick check of the address. */}
      {c.mailboxStatus ? <MailboxBadge check={c.mailboxStatus} /> : <EmailCheckBadge check={c.emailCheckStatus} />}
      {c.mailboxCheckedAt && c.mailboxStatus !== "checking" && (
        <span className="muted small freshness" title={`Mail server asked ${formatWhen(c.mailboxCheckedAt)}`}> {timeAgo(c.mailboxCheckedAt)}</span>
      )}
    </>
  ) },
  phone: { label: "Phone", cell: (c: ContactSummary) => c.mobile ?? c.phone },
};
type Cell = keyof typeof CELLS;

/** Contacts in a table, linking to each contact (and their company, if shown). */
export function ContactTable({ contacts, showCompany = false, empty, picked, onPick, onClearPicked, dragName, inFolders }: {
  contacts: ContactSummary[];
  showCompany?: boolean;
  empty: string;
  /** Chosen, to email each separately (with onPick, clicking a row chooses it). */
  picked?: Set<string>;
  onPick?: (contacts: ContactSummary[], on: boolean) => void;
  /** Unchooses every contact chosen (Esc), also those not shown now. */
  onClearPicked?: () => void;
  /** Lets a contact's name be dragged onto a folder. */
  dragName?: (id: string) => React.HTMLAttributes<HTMLElement> | undefined;
  /** With it, an "In folders" column. */
  inFolders?: (id: string) => string[];
}) {
  const picking = useRowPicking({
    ids: contacts.map((c) => c.id), chosen: picked ?? new Set<string>(), enabled: !!onPick,
    set: (ids, on) => { onPick?.(contacts.filter((c) => ids.includes(c.id)), on); },
    clear: () => { if (onClearPicked) onClearPicked(); else onPick?.(contacts.filter((c) => picked?.has(c.id)), false); },
  });
  const fs = useFieldSettings();
  const table = showCompany ? "contacts" : "company-contacts";
  // After the name, the columns are in the order the person dragged them into.
  const columns = useStoredOrder<Cell>(table, (Object.keys(CELLS) as Cell[]).filter((k) => showCompany || k !== "company"));
  const tableRef = useRef<HTMLTableElement>(null);
  const moveColumn = useColumnMove(tableRef, columns.order, columns.save);
  const widths = useColumnWidths(table, ["name", ...columns.order, ...(inFolders ? ["folders"] : [])], tableRef);
  return (
    <>
    {(widths.set || columns.changed) && (
      <div className="widths-reset">
        {columns.changed && <button type="button" className="link small" onClick={columns.reset}>Reset column order</button>}
        {widths.set && <button type="button" className="link small" onClick={widths.reset}>Reset column widths</button>}
      </div>
    )}
    <PinnedScroll>
    <table ref={tableRef} {...picking.tableProps} className={`contacts-table${onPick ? " pick-table" : ""}${widths.set ? " sized" : ""}`} style={widths.tableStyle}>
      <thead>
        <tr>
          <th {...widths.head("name", "Name")}>Name{widths.handle("name", "Name")}</th>
          {columns.order.map((k) => <th key={k} {...widths.head(k, CELLS[k].label)} {...moveColumn.props(k)}>{CELLS[k].label}{widths.handle(k, CELLS[k].label)}</th>)}
          {inFolders && <th {...widths.head("folders", "In folders")}>In folders{widths.handle("folders", "In folders")}</th>}
        </tr>
      </thead>
      <tbody>
        {contacts.map((c) => {
          const pickProps = onPick ? picking.rowProps(c.id) : undefined;
          const bounces = c.mailboxStatus === "invalid";
          return (
          <tr key={c.id} {...pickProps} className={[pickProps?.className, bounces ? "bounces" : ""].filter(Boolean).join(" ") || undefined}>
            <td {...dragName?.(c.id)}>
              <a href={`#/contacts/${c.id}`}>{c.fullName}</a>
              {onPick && picked?.has(c.id) && <span className="visually-hidden"> (chosen)</span>}
            </td>
            {columns.order.map((k) => <td key={k}>{CELLS[k].cell(c, fs)}</td>)}
            {inFolders && <td className="col-folders"><InFolders names={inFolders(c.id)} /></td>}
          </tr>
          );
        })}
        {contacts.length === 0 && (
          <tr><td colSpan={columns.order.length + (inFolders ? 2 : 1)} className="muted">{empty}</td></tr>
        )}
      </tbody>
    </table>
    </PinnedScroll>
    </>
  );
}
