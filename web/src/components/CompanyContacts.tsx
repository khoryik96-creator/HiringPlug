import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Company, unwrap } from "../api/client";
import { contactsKey } from "../api/keys";
import { ContactForm } from "./ContactForm";
import { ContactTable } from "./ContactTable";
import { ErrorMessage } from "./ErrorMessage";
import { useLinkedinChecks } from "./LinkedinChecks";
import { PickBar, SeparateEmail } from "./SeparateEmail";
import { contactPerson, usePicked } from "./usePicked";

/** A company's contacts, with "Add contact" already set to this company. */
export function CompanyContacts({ company, canCreate, canEmail = false, canCheck = false }: {
  company: Company; canCreate: boolean; canEmail?: boolean;
  /** May verify the contacts' emails (contacts:update). */
  canCheck?: boolean;
}) {
  const queryClient = useQueryClient();
  const [adding, setAdding] = useState(false);
  const pick = usePicked();
  const [emailing, setEmailing] = useState(false);
  const list = useQuery({
    queryKey: [...contactsKey, "company", company.id],
    queryFn: async () =>
      unwrap(await api.GET("/v1/contacts", { params: { query: { companyId: company.id, limit: 100 } } })).items,
    // Follow emails being verified.
    refetchInterval: (q) => (q.state.data?.some((c) => c.mailboxStatus === "checking") ? 2000 : false),
  });
  const verify = useMutation({
    mutationFn: async () => unwrap(await api.POST("/v1/contacts/mailbox-checks", { body: { companyId: company.id } })),
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: contactsKey }); },
  });
  const checking = list.data?.filter((c) => c.mailboxStatus === "checking").length ?? 0;
  const bounces = list.data?.filter((c) => c.mailboxStatus === "invalid").length ?? 0;
  const withEmail = list.data?.filter((c) => c.email).length ?? 0;
  const linkedin = useLinkedinChecks({ companyId: company.id });
  const canLinkedin = canCheck && !!company.linkedinUrl && (list.data?.length ?? 0) > 0;
  return (
    <section className="panel" aria-label="Contacts">
      <div className="row spread">
        <h3>Contacts</h3>
        <span className="row">
          {canLinkedin && linkedin.button}
          {canCheck && withEmail > 0 && (
            <button type="button" disabled={verify.isPending || checking > 0}
              title="Ask each contact's mail server whether their mailbox exists (no email is sent)"
              onClick={() => { verify.mutate(); }}>
              {checking > 0 ? `Verifying ${String(checking)}…` : "Verify emails"}
            </button>
          )}
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add contact</button>}
        </span>
      </div>
      {adding && (
        <ContactForm contact={null} company={{ id: company.id, name: company.name }} onDone={() => { setAdding(false); }} />
      )}
      <ErrorMessage error={list.error} />
      <ErrorMessage error={verify.error} />
      {canLinkedin && linkedin.panel}
      {canCheck && !company.linkedinUrl && (list.data?.length ?? 0) > 0 && (
        <p className="muted small">Add the company&apos;s LinkedIn page to check on LinkedIn whether contacts still work here.</p>
      )}
      {bounces > 0 && checking === 0 && (
        <p className="check-summary warning" role="status">
          {bounces === 1 ? "1 contact's email bounces" : `${String(bounces)} contacts' emails bounce`}: their mailbox no longer exists (often because they left). Marked in red below.
        </p>
      )}
      {emailing && <SeparateEmail people={pick.people} onClose={() => { setEmailing(false); pick.clear(); }} />}
      {canEmail && !emailing && <PickBar count={pick.picked.size} onEmail={() => { setEmailing(true); }} onClear={pick.clear} />}
      {list.data && (
        <ContactTable contacts={list.data} empty="No contacts at this company yet." picked={pick.ids}
          onPick={canEmail ? (cs, on) => { pick.pick(cs.map(contactPerson), on); } : undefined} />
      )}
    </section>
  );
}
