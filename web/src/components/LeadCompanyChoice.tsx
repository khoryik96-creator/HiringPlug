// Leads › Find HR, step 1: which company the lead is. The companies already
// in HiringPlug with the same or a similar name, to link the lead to, or the
// lead's company added as a new one (with its LinkedIn page) and linked.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, ApiError, type Company, type Permission, type Schemas, unwrap } from "../api/client";
import { companyKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";
import { companyPage } from "./linkedinCompany";
import { LinkedinPageFinder } from "./LinkedinPageFinder";

type Lead = Schemas["JobLead"];

export function LeadCompanyChoice({ lead, permissions, onLinked }: {
  lead: Lead; permissions: Permission[]; onLinked: (companyId: string) => void;
}) {
  const queryClient = useQueryClient();
  const canLink = permissions.includes("leads:manage");
  const canAdd = canLink && permissions.includes("companies:create");
  const matches = useQuery({
    queryKey: ["companies", "named-like", lead.companyName],
    queryFn: async () => unwrap(await api.GET("/v1/companies/named-like", { params: { query: { name: lead.companyName } } })).items,
  });
  const [name, setName] = useState(lead.companyName);
  const [linkedin, setLinkedin] = useState("");
  const [addAnyway, setAddAnyway] = useState(false);
  // Added already, then the link failed: trying again only links it (its name and page are kept).
  const [created, setCreated] = useState<Company | null>(null);
  // A company in HiringPlug has the LinkedIn page chosen, under another name.
  const [pageOwner, setPageOwner] = useState<{ page: string; id: string; name: string } | null>(null);

  const linkTo = async (companyId: string) => {
    try {
      const saved = unwrap(await api.PATCH("/v1/job-leads/{jobLeadId}", { params: { path: { jobLeadId: lead.id } }, body: { version: lead.version, companyId } }));
      onLinked(saved.companyId ?? companyId);
    } finally {
      // Linked, or changed since it was read (its status, say): the list read again, so trying again can work.
      void queryClient.invalidateQueries({ queryKey: ["job-leads"] });
    }
  };
  const link = useMutation({ mutationFn: linkTo });
  const add = useMutation({
    mutationFn: async () => {
      let target = created;
      if (!target) {
        const page = companyPage(linkedin);
        const slug = /\/company\/([A-Za-z0-9_-]+)\//.exec(page)?.[1];
        const owner = slug ? unwrap(await api.GET("/v1/companies", { params: { query: { linkedinCompany: slug, limit: 1 } } })).items[0] : undefined;
        if (owner) {
          setPageOwner({ page, id: owner.id, name: owner.name });
          return;
        }
        target = unwrap(await api.POST("/v1/companies", { body: { name: name.trim(), ...(page ? { linkedinUrl: page } : {}) } }));
        setCreated(target);
        queryClient.setQueryData(companyKey(target.id), target);
        void queryClient.invalidateQueries({ queryKey: ["companies"] });
      }
      await linkTo(target.id);
    },
  });
  const busy = link.isPending || add.isPending;
  const items = matches.data ?? [];
  // Once added, the form stays (with why the link failed) even though the company now has the same name.
  const showAdd = created !== null || addAnyway || (matches.isSuccess && !items.some((m) => m.sameName));
  const existing = add.error instanceof ApiError && add.error.code === "COMPANY_DUPLICATE_NAME" ? add.error.details.existingCompanyId : undefined;
  const pageWrong = linkedin.trim() !== "" && companyPage(linkedin) === "";
  const samePage = pageOwner && pageOwner.page === companyPage(linkedin) ? pageOwner : null;

  return (
    <div className="lead-hr-step" role="group" aria-label="Which company is it?">
      <h4>1 · Which company is it?</h4>
      {!canLink && <p className="muted small">Linking a lead to a company needs the permission to manage leads.</p>}
      <ErrorMessage error={matches.error ?? link.error} />
      {matches.isPending && <p className="muted small">Looking for it in HiringPlug…</p>}
      {matches.isSuccess && items.length === 0 && <p className="muted small">No company in HiringPlug has this name or one like it.</p>}
      {items.length > 0 && (
        <ul className="lead-hr-matches">
          {items.map((m) => (
            <li key={m.id}>
              <div>
                <strong>{m.name}</strong>
                <div className="muted small">in HiringPlug · {m.sameName ? "same name" : "similar name"}{m.city ? ` · ${m.city}` : ""}</div>
              </div>
              {canLink && <button type="button" className="doc-btn primary" disabled={busy} aria-label={`This one: ${m.name}`}
                onClick={() => { link.mutate(m.id); }}>This one</button>}
            </li>
          ))}
        </ul>
      )}
      {canLink && !showAdd && matches.isSuccess && (
        <button type="button" className="link small" onClick={() => { setAddAnyway(true); }}>None of these: add it as a new company</button>
      )}
      {canLink && showAdd && (canAdd ? (
        <form className="lead-hr-new" aria-label="New company" onSubmit={(e) => { e.preventDefault(); add.mutate(); }}>
          <h4>{created ? `${created.name} is added to HiringPlug` : `Add ${name.trim() || lead.companyName} as a new company`}</h4>
          {created && <p className="muted small">The lead is not linked to it yet.</p>}
          <label className="field">Name
            <input required maxLength={200} value={created ? created.name : name} disabled={busy || created !== null} onChange={(e) => { setName(e.target.value); }} />
          </label>
          <LinkedinPageFinder name={name} value={created ? (created.linkedinUrl ?? "") : linkedin} onChange={setLinkedin} disabled={busy || created !== null} />
          {samePage && (
            <div className="warning small" role="status">
              <p>{samePage.name} in HiringPlug has this LinkedIn page.</p>
              <button type="button" className="doc-btn" disabled={busy} onClick={() => { link.mutate(samePage.id); }}>Link the lead to {samePage.name}</button>
            </div>
          )}
          <ErrorMessage error={add.error}>
            {typeof existing === "string" && <button type="button" className="doc-btn" disabled={busy} onClick={() => { add.reset(); link.mutate(existing); }}>Link the lead to that one</button>}
          </ErrorMessage>
          <div className="row">
            <button type="submit" className="primary" disabled={busy || !name.trim() || pageWrong || samePage !== null}>
              {add.isPending ? (created ? "Linking…" : "Adding…") : created ? `Link to ${created.name}` : "Add and link"}
            </button>
          </div>
        </form>
      ) : <p className="muted small">Adding a company needs the permission to create companies.</p>)}
    </div>
  );
}
