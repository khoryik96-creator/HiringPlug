import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";
import { IconButton, PlusIcon } from "../components/ActionIcons";
import { ContactForm } from "../components/ContactForm";
import { ContactTable } from "../components/ContactTable";
import { useDebounced } from "../components/useDebounced";
import { BooleanSearchBox, ListSearchError, looksBoolean } from "../components/search/BooleanSearchBox";
import { useFolderView, useInFolders, WithFolders } from "../components/folders/FolderView";
import { PickBar, SeparateEmail } from "../components/SeparateEmail";
import { contactPerson, usePicked } from "../components/usePicked";
import { useFieldSettings } from "../components/useFieldSettings";
import { hashParam } from "../hashQuery";

/** "" any role; NOT_SET those without one; otherwise a role's key. */
const NOT_SET = "-";

/** Search and add contacts across all companies; filter by role and whether they still work there. */
export function ContactList({ canCreate, canEmail = false, canFolders = false, meId = "" }: { canCreate: boolean; canEmail?: boolean; canFolders?: boolean; meId?: string }) {
  const fs = useFieldSettings();
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [role, setRole] = useState("");
  const [status, setStatus] = useState<Schemas["ContactStatus"] | "">("");
  const [adding, setAdding] = useState(false);
  const pick = usePicked();
  const folders = useFolderView("contact", canFolders, meId, pick.clear);
  const folder = folders.filter;
  const canPick = canEmail || canFolders; // rows are chosen to email or to put in a folder
  const [emailing, setEmailing] = useState(false);
  const typed = useDebounced(q, 350); // one search once typing pauses
  const list = useInfiniteQuery({
    queryKey: [...contactsKey, typed, role, status, folder],
    enabled: folders.ready,
    queryFn: async ({ pageParam }) => unwrap(await api.GET("/v1/contacts", { params: { query: {
      limit: 100,
      ...(typed.trim() ? { [looksBoolean(typed) ? "boolean" : "q"]: typed.trim() } : {}), // AND, OR, NOT…: a boolean search
      ...(role === NOT_SET ? { roleNotSet: true } : role ? { role } : {}),
      ...(status ? { status } : {}),
      ...(folder ? { folder } : {}),
      ...(pageParam ? { cursor: pageParam } : {}),
    } } })),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => last.nextCursor,
  });
  const contacts = list.data?.pages.flatMap((page) => page.items);
  const filtered = !!(q || role || status);
  const inFolders = useInFolders(folders, contacts?.map((c) => c.id) ?? []);

  return (
    <main className="list-page">
      <h2 className="visually-hidden">Contacts</h2>
      <div className="list-top">
        <BooleanSearchBox value={q} onChange={setQ} label="Search contacts" placeholder="Search by name, email, title or phone, or AND, OR, NOT…">
          <select aria-label="Role" className={`search-pill${role ? " on" : ""}`} value={role} onChange={(e) => { setRole(e.target.value); }}>
            <option value="">All roles</option>
            {fs.options("contact_role").map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
            <option value={NOT_SET}>Role not set</option>
          </select>
          <select aria-label="Status" className={`search-pill${status ? " on" : ""}`} value={status} onChange={(e) => { setStatus(e.target.value as Schemas["ContactStatus"] | ""); }}>
            <option value="">Everyone</option>
            <option value="active">Still there</option>
            <option value="left">Has left</option>
          </select>
        </BooleanSearchBox>
        {canCreate && !adding && <IconButton label="Add contact" icon={<PlusIcon />} className="icon-primary" onClick={() => { setAdding(true); }} />}
      </div>
      {adding && (
        <ContactForm
          contact={null}
          onDone={() => { setAdding(false); }}
          onSaved={(c) => { window.location.hash = `#/contacts/${c.id}`; }}
        />
      )}
      <ListSearchError error={list.error} />
      {emailing && <SeparateEmail people={pick.people} onClose={() => { setEmailing(false); pick.clear(); }} />}
      <WithFolders view={folders}>
      {contacts && (
        <>
          <div className="list-meta">
            <span role="status">
              {contacts.length}{list.hasNextPage ? "+" : ""} contact{contacts.length === 1 ? "" : "s"}{list.hasNextPage ? ", more below" : ""}
            </span>
            {canPick && !emailing && (
              <PickBar count={pick.picked.size} onEmail={canEmail ? () => { setEmailing(true); } : undefined} onClear={pick.clear}
                shown={{ count: contacts.length, more: list.hasNextPage, all: contacts.length > 0 && contacts.every((c) => pick.ids.has(c.id)),
                  onAll: () => { pick.pick(contacts.map(contactPerson), true); } }}>
                {folders.actions([...pick.ids])}
              </PickBar>
            )}
          </div>
          <ContactTable contacts={contacts} showCompany
            empty={filtered ? "No contacts match." : folder ? "Nothing in this folder yet: choose contacts under All contacts, then Add to folder (or drag them onto it)." : "No contacts yet."}
            picked={pick.ids} onPick={canPick ? (cs, on) => { pick.pick(cs.map(contactPerson), on); } : undefined} onClearPicked={pick.clear}
            dragName={(id) => folders.drag(id, pick.ids)} inFolders={folders.hasFolders ? inFolders : undefined} />
          {list.hasNextPage && (
            <button type="button" className="doc-btn" disabled={list.isFetchingNextPage} onClick={() => { void list.fetchNextPage(); }}>Show more</button>
          )}
        </>
      )}
      </WithFolders>
    </main>
  );
}
