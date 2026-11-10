import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { companyKey } from "../api/keys";
import { CompanyAutofill } from "../components/CompanyAutofill";
import { CompanyBanner } from "../components/CompanyBanner";
import { CompanyBenefits } from "../components/CompanyBenefits";
import { CompanyTerms } from "../components/CompanyTerms";
import { CompanyContacts } from "../components/CompanyContacts";
import { CompanyForm } from "../components/CompanyForm";
import { CompanyJobs } from "../components/CompanyJobs";
import { ErrorMessage } from "../components/ErrorMessage";
import { Timeline } from "../components/Timeline";
import { COMPANY_SIZE } from "../labels";

/** One company. Render it with key={id} so edit state never carries over. */
export function CompanyDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const company = useQuery({
    queryKey: companyKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: id } } })),
  });
  const c = company.data;
  if (!c) return company.error ? <ErrorMessage error={company.error} /> : <p className="muted">Loading…</p>;

  return (
    <main>
      <a href="#/companies">← Companies</a>
      {company.error && (
        <ErrorMessage error={company.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      {permissions.includes("companies:update") && !editing && (
        <div className="row spread"><span /><button type="button" onClick={() => { setEditing(true); }}>Edit company</button></div>
      )}
      <CompanyBanner company={c} canEdit={permissions.includes("companies:update")} />
      {editing ? (
        <CompanyForm company={c} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel" aria-label="Company details">
          <dl>
            <dt>Website</dt>
            <dd>{c.website ? <a href={c.website} target="_blank" rel="noreferrer">{c.website}</a> : "—"}</dd>
            <dt>LinkedIn</dt>
            <dd>{c.linkedinUrl ? <a href={c.linkedinUrl} target="_blank" rel="noreferrer">{c.linkedinUrl.replace(/^https:\/\/www\./, "")}</a> : "—"}</dd>
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
      <CompanyTerms companyId={c.id} permissions={permissions} />
      <CompanyBenefits companyId={c.id} companyName={c.name} permissions={permissions} />
      {/* Notes (and the timeline) right under the details, as on a candidate's page. */}
      <Timeline subjectType="company" subjectId={c.id} permissions={permissions} />
      {permissions.includes("companies:update") && !editing && <CompanyAutofill companyId={c.id} website={c.website} />}
      <CompanyJobs company={c} canCreate={permissions.includes("jobs:create")} />
      <CompanyContacts company={c} canCreate={permissions.includes("contacts:create")} canEmail={permissions.includes("emails:send")} />
    </main>
  );
}
