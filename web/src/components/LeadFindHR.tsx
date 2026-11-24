// Leads › Find HR (option A): a side panel beside the list for one lead.
// Step 1 settles which company it is (LeadCompanyChoice); step 2 is the
// company's Find HR scan, laid out for the panel. A lead linked already
// starts at step 2, asking first for the company's LinkedIn page if it has none.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";

import { api, type Company, type Permission, type Schemas, unwrap } from "../api/client";
import { companyKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { FindHR } from "./FindHR";
import { LeadCompanyChoice } from "./LeadCompanyChoice";
import { companyPage } from "./linkedinCompany";
import { LinkedinPageFinder } from "./LinkedinPageFinder";

/** Render it with key={lead.id}: another lead starts again at its own step. */
export function LeadFindHR({ lead, permissions, onClose }: { lead: Schemas["JobLead"]; permissions: Permission[]; onClose: () => void }) {
  // Linked here: step 2 at once, before the list is read again.
  const [linkedId, setLinkedId] = useState<string | null>(null);
  const companyId = linkedId ?? lead.companyId;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || e.defaultPrevented) return;
      // Escape in a dialog opened from the panel (Apollo) closes that dialog only.
      if (document.querySelector("dialog[open]") || (e.target instanceof Element && e.target.closest("dialog"))) return;
      onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => { document.removeEventListener("keydown", onKey); };
  }, [onClose]);

  return (
    <aside className="panel lead-hr" aria-label={`Find HR for ${lead.companyName}`}>
      <div className="lead-hr-head">
        <h3>Find HR · {lead.companyName}</h3>
        <button type="button" className="icon-action" aria-label="Close" onClick={onClose}>✕</button>
      </div>
      {companyId ? <LinkedCompany id={companyId} permissions={permissions} />
        : <LeadCompanyChoice lead={lead} permissions={permissions} onLinked={setLinkedId} />}
    </aside>
  );
}

function LinkedCompany({ id, permissions }: { id: string; permissions: Permission[] }) {
  const company = useQuery({
    queryKey: companyKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: id } } })),
  });
  const c = company.data;
  if (!c) return company.error ? <ErrorMessage error={company.error} /> : <p className="muted small">Loading…</p>;
  const hasPage = companyPage(c.linkedinUrl ?? "") !== "";
  return (
    <>
      <p className="lead-hr-company small">
        <span className="muted">Company: </span>
        <a href={`#/companies/${c.id}`} target="_blank" rel="noreferrer">{c.name} ↗</a>
        {c.city && <span className="muted"> · {c.city}</span>}
      </p>
      {!hasPage && <CompanyLinkedin company={c} canUpdate={permissions.includes("companies:update")} />}
      <div className="lead-hr-step" role="group" aria-label="Its HR people on LinkedIn">
        <h4>2 · Its HR people on LinkedIn</h4>
        {hasPage
          ? <FindHR company={c} canCreate={permissions.includes("contacts:create")} canUseApollo={permissions.includes("contacts.apollo:use")} compact />
          : <p className="muted small">Add its LinkedIn page first.</p>}
      </div>
    </>
  );
}

/** The company has no LinkedIn page in HiringPlug yet: pasted or found, then saved. */
function CompanyLinkedin({ company, canUpdate }: { company: Company; canUpdate: boolean }) {
  const queryClient = useQueryClient();
  const [url, setUrl] = useState("");
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PATCH("/v1/companies/{companyId}", {
      params: { path: { companyId: company.id } }, body: { version: company.version, linkedinUrl: companyPage(url) },
    })),
    onSuccess: (saved) => {
      queryClient.setQueryData(companyKey(company.id), saved);
      void queryClient.invalidateQueries({ queryKey: ["companies"] });
    },
  });
  if (!canUpdate) return <p className="muted small">{company.name} has no LinkedIn page in HiringPlug yet, and adding it needs the permission to edit companies.</p>;
  return (
    <form className="lead-hr-step" aria-label="Its LinkedIn page" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h4>Its LinkedIn page</h4>
      <LinkedinPageFinder name={company.name} value={url} onChange={setUrl} disabled={save.isPending} />
      <ErrorMessage error={save.error} />
      <div className="row">
        <button type="submit" className="primary" disabled={save.isPending || !companyPage(url)}>{save.isPending ? "Saving…" : "Save page"}</button>
      </div>
    </form>
  );
}
