import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";
import { ContactForm } from "../components/ContactForm";
import { ContactTable } from "../components/ContactTable";
import { ErrorMessage } from "../components/ErrorMessage";

/** Search and add contacts across all companies. */
export function ContactList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState("");
  const [adding, setAdding] = useState(false);
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
          <input aria-label="Search contacts" placeholder="Search name, email or title" value={q} onChange={(e) => { setQ(e.target.value); }} />
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
      {list.data && <ContactTable contacts={list.data.items} showCompany empty={q ? "No contacts match." : "No contacts yet."} />}
    </main>
  );
}
