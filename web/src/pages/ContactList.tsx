import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";
import { ContactForm } from "../components/ContactForm";
import { ContactTable } from "../components/ContactTable";
import { ErrorMessage } from "../components/ErrorMessage";
import { PickBar, SeparateEmail } from "../components/SeparateEmail";
import { contactPerson, usePicked } from "../components/usePicked";
import { hashParam } from "../hashQuery";

/** Search and add contacts across all companies. */
export function ContactList({ canCreate, canEmail = false }: { canCreate: boolean; canEmail?: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [adding, setAdding] = useState(false);
  const pick = usePicked();
  const [emailing, setEmailing] = useState(false);
  const list = useQuery({
    queryKey: [...contactsKey, q],
    queryFn: async () =>
      unwrap(await api.GET("/v1/contacts", { params: { query: q.trim() ? { q: q.trim() } : {} } })),
  });

  return (
    <main>
      <div className="row spread">
        <h2>Contacts</h2>
        <div className="row">
          <input aria-label="Search contacts" placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); }} />
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
      {list.data && (
        <ContactTable contacts={list.data.items} showCompany empty={q ? "No contacts match." : "No contacts yet."}
          picked={pick.ids} onPick={canEmail ? (cs, on) => { pick.pick(cs.map(contactPerson), on); } : undefined} />
      )}
    </main>
  );
}
