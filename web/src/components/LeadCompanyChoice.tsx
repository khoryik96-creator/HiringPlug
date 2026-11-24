// Leads › Find HR, step 1: which company the lead is. The companies already
// in HiringPlug with the same or a similar name, to link the lead to, or the
// lead's company added as a new one (with its LinkedIn page) and linked.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";

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
  // Added already, then the link failed: trying again only links it.
  const created = useRef<Company | null>(null);

  const linkTo = async (companyId: string) => {
    const saved = unwrap(await api.PATCH("/v1/job-leads/{jobLeadId}", { params: { path: { jobLeadId: lead.id } }, body: { version: lead.version, companyId } }));
    void queryClient.invalidateQueries({ queryKey: ["job-leads"] });
    onLinked(saved.companyId ?? companyId);
  };
  const link = useMutation({ mutationFn: linkTo });
  const add = useMutation({
    mutationFn: async () => {
      if (!created.current) {
        const page = companyPage(linkedin);
        const c = unwrap(await api.POST("/v1/companies", { body: { name: name.trim(), ...(page ? { linkedinUrl: page } : {}) } }));
        created.current = c;
        queryClient.setQueryData(companyKey(c.id), c);
        void queryClient.invalidateQueries({ queryKey: ["companies"] });
      }
      await linkTo(created.current.id);
    },
  });
  const busy = link.isPending || add.isPending;
  const items = matches.data ?? [];
  const showAdd = addAnyway || (matches.isSuccess && !items.some((m) => m.sameName));
  const existing = add.error instanceof ApiError && add.error.code === "COMPANY_DUPLICATE_NAME" ? add.error.details.existingCompanyId : undefined;
  const pageWrong = linkedin.trim() !== "" && companyPage(linkedin) === "";

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
          <h4>Add {name.trim() || lead.companyName} as a new company</h4>
          <label className="field">Name
            <input required maxLength={200} value={name} disabled={busy} onChange={(e) => { setName(e.target.value); }} />
          </label>
          <LinkedinPageFinder name={name} value={linkedin} onChange={setLinkedin} disabled={busy} />
          <ErrorMessage error={add.error}>
            {typeof existing === "string" && <button type="button" className="doc-btn" disabled={busy} onClick={() => { add.reset(); link.mutate(existing); }}>Link the lead to that one</button>}
          </ErrorMessage>
          <div className="row">
            <button type="submit" className="primary" disabled={busy || !name.trim() || pageWrong}>{add.isPending ? "Adding…" : "Add and link"}</button>
          </div>
        </form>
      ) : <p className="muted small">Adding a company needs the permission to create companies.</p>)}
    </div>
  );
}
