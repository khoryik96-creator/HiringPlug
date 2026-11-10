import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Company, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";
import { ContactForm } from "./ContactForm";
import { ContactTable } from "./ContactTable";
import { ErrorMessage } from "./ErrorMessage";
import { PickBar, SeparateEmail } from "./SeparateEmail";
import { contactPerson, usePicked } from "./usePicked";

/** A company's contacts, with "Add contact" already set to this company. */
export function CompanyContacts({ company, canCreate, canEmail = false }: { company: Company; canCreate: boolean; canEmail?: boolean }) {
  const [adding, setAdding] = useState(false);
  const pick = usePicked();
  const [emailing, setEmailing] = useState(false);
  const list = useQuery({
    queryKey: [...contactsKey, "company", company.id],
    queryFn: async () =>
      unwrap(await api.GET("/v1/contacts", { params: { query: { companyId: company.id, limit: 100 } } })).items,
  });
  return (
    <section className="panel" aria-label="Contacts">
      <div className="row spread">
        <h3>Contacts</h3>
        {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add contact</button>}
      </div>
      {adding && (
        <ContactForm contact={null} company={{ id: company.id, name: company.name }} onDone={() => { setAdding(false); }} />
      )}
      <ErrorMessage error={list.error} />
      {emailing && <SeparateEmail people={pick.people} onClose={() => { setEmailing(false); pick.clear(); }} />}
      {canEmail && !emailing && <PickBar count={pick.picked.size} onEmail={() => { setEmailing(true); }} onClear={pick.clear} />}
      {list.data && (
        <ContactTable contacts={list.data} empty="No contacts at this company yet." picked={pick.ids}
          onPick={canEmail ? (cs, on) => { pick.pick(cs.map(contactPerson), on); } : undefined} />
      )}
    </section>
  );
}
