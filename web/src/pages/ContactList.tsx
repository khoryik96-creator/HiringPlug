import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";
import { ContactForm } from "../components/ContactForm";
import { ContactTable } from "../components/ContactTable";
import { ErrorMessage } from "../components/ErrorMessage";
import { PickBar, SeparateEmail } from "../components/SeparateEmail";
import { contactPerson, usePicked } from "../components/usePicked";
import { useFieldSettings } from "../components/useFieldSettings";
import { hashParam } from "../hashQuery";

/** "" any role; NOT_SET those without one; otherwise a role's key. */
const NOT_SET = "-";

/** Search and add contacts across all companies; filter by role and whether they still work there. */
export function ContactList({ canCreate, canEmail = false }: { canCreate: boolean; canEmail?: boolean }) {
  const fs = useFieldSettings();
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [role, setRole] = useState("");
  const [status, setStatus] = useState<Schemas["ContactStatus"] | "">("");
  const [adding, setAdding] = useState(false);
  const pick = usePicked();
  const [emailing, setEmailing] = useState(false);
  const list = useInfiniteQuery({
    queryKey: [...contactsKey, q, role, status],
    queryFn: async ({ pageParam }) => unwrap(await api.GET("/v1/contacts", { params: { query: {
      limit: 100,
      ...(q.trim() ? { q: q.trim() } : {}),
      ...(role === NOT_SET ? { roleNotSet: true } : role ? { role } : {}),
      ...(status ? { status } : {}),
      ...(pageParam ? { cursor: pageParam } : {}),
    } } })),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
  });
  const contacts = list.data?.pages.flatMap((page) => page.items);
  const filtered = !!(q || role || status);

  return (
    <main>
      <div className="row spread">
        <h2>Contacts</h2>
        <div className="row">
          <input aria-label="Search contacts" placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <select aria-label="Role" value={role} onChange={(e) => { setRole(e.target.value); }}>
            <option value="">All roles</option>
            {fs.options("contact_role").map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
            <option value={NOT_SET}>Role not set</option>
          </select>
          <select aria-label="Status" value={status} onChange={(e) => { setStatus(e.target.value as Schemas["ContactStatus"] | ""); }}>
            <option value="">Everyone</option>
            <option value="active">Still there</option>
            <option value="left">Has left</option>
          </select>
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add contact</button>}
        </div>
      </div>
      {adding && (
        <ContactForm
          contact={null}
          onDone={() => { setAdding(false); }}
          onSaved={(c) => { window.location.hash = `#/contacts/${c.id}`; }}
        />
      )}
      <ErrorMessage error={list.error} />
      {emailing && <SeparateEmail people={pick.people} onClose={() => { setEmailing(false); pick.clear(); }} />}
      {canEmail && !emailing && <PickBar count={pick.picked.size} onEmail={() => { setEmailing(true); }} onClear={pick.clear} />}
      {contacts && (
        <>
          <p className="muted small" role="status">
            {contacts.length}{list.hasNextPage ? "+" : ""} contact{contacts.length === 1 ? "" : "s"}{list.hasNextPage ? ", more below" : ""}
          </p>
          <ContactTable contacts={contacts} showCompany empty={filtered ? "No contacts match." : "No contacts yet."}
            picked={pick.ids} onPick={canEmail ? (cs, on) => { pick.pick(cs.map(contactPerson), on); } : undefined} />
          {list.hasNextPage && (
            <button type="button" className="doc-btn" disabled={list.isFetchingNextPage} onClick={() => { void list.fetchNextPage(); }}>Show more</button>
          )}
        </>
      )}
    </main>
  );
}
