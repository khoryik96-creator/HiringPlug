import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { companyKey } from "../api/keys";
import { CompanyAutofill } from "../components/CompanyAutofill";
import { CompanyContacts } from "../components/CompanyContacts";
import { CompanyForm } from "../components/CompanyForm";
import { CompanyJobs } from "../components/CompanyJobs";
import { ErrorMessage } from "../components/ErrorMessage";
import { Timeline } from "../components/Timeline";
import { COMPANY_SIZE, COMPANY_TYPE, countryName } from "../labels";

/** One company. Render it with key={id} so edit state never carries over. */
export function CompanyDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const company = useQuery({
    queryKey: companyKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: id } } })),
  });
  const c = company.data;
  if (!c) return company.error ? <ErrorMessage error={company.error} /> : <p className="muted">Loading…</p>;

  const place = [c.city, c.country ? countryName(c.country) : null].filter(Boolean).join(", ");
  return (
    <main>
      <a href="#/companies">← Companies</a>
      {company.error && (
        <ErrorMessage error={company.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      <div className="row spread">
        <h2>{c.name}</h2>
        {permissions.includes("companies:update") && !editing && (
          <button type="button" onClick={() => { setEditing(true); }}>Edit company</button>
        )}
      </div>
      <p className="muted">{[COMPANY_TYPE[c.type], c.industry, place].filter(Boolean).join(" · ")}</p>
      {editing ? (
        <CompanyForm company={c} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel" aria-label="Company details">
          <dl>
            <dt>Website</dt>
            <dd>{c.website ? <a href={c.website} target="_blank" rel="noreferrer">{c.website}</a> : "—"}</dd>
            <dt>Size</dt>
            <dd>{c.size ? COMPANY_SIZE[c.size] : "—"}</dd>
            <dt>Phone</dt>
            <dd>{c.phone ?? "—"}</dd>
            <dt>Address</dt>
            <dd className="pre">{c.address ?? "—"}</dd>
            <dt>Registration no.</dt>
            <dd>{c.registrationNumber ?? "—"}</dd>
            <dt>About</dt>
            <dd className="pre">{c.about ?? "—"}</dd>
          </dl>
        </section>
      )}
      {permissions.includes("companies:update") && !editing && <CompanyAutofill companyId={c.id} website={c.website} />}
      <CompanyJobs company={c} canCreate={permissions.includes("jobs:create")} />
      <CompanyContacts company={c} canCreate={permissions.includes("contacts:create")} />
      <Timeline subjectType="company" subjectId={c.id} permissions={permissions} />
    </main>
  );
}
