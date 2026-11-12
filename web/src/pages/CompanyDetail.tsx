import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { IndustrySummary } from "../components/IndustryPicker";
import { companyKey } from "../api/keys";
import { CompanyAutofill } from "../components/CompanyAutofill";
import { CompanyBanner } from "../components/CompanyBanner";
import { CompanyBenefits } from "../components/CompanyBenefits";
import { CompanyTerms } from "../components/CompanyTerms";
import { CompanyContacts } from "../components/CompanyContacts";
import { CompanyForm } from "../components/CompanyForm";
import { CompanyJobs } from "../components/CompanyJobs";
import { ErrorMessage } from "../components/ErrorMessage";
import { type Section, SectionTabs } from "../components/SectionTabs";
import { Timeline } from "../components/Timeline";
import { COMPANY_SIZE } from "../labels";

type Tab = "details" | "terms" | "jobs" | "contacts" | "activity";

/**
 * One company, laid out like a candidate: its card, a contact column and its
 * sections as tabs. Render it with key={id} so edit state never carries over.
 */
export function CompanyDetail({ id, permissions }: { id: string; permissions: Permission[] }) {
  const [editing, setEditing] = useState(false);
  const [active, setActive] = useState<Tab>("details");
  const company = useQuery({
    queryKey: companyKey(id),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}", { params: { path: { companyId: id } } })),
  });
  const c = company.data;
  if (!c) return company.error ? <ErrorMessage error={company.error} /> : <p className="muted">Loading…</p>;
  const canEdit = permissions.includes("companies:update");

  const sections: Section<Tab>[] = [
    { id: "details", label: "Details", content: <>
      {editing ? (
        <CompanyForm company={c} onDone={() => { setEditing(false); }} />
      ) : (
        <section className="panel candidate-profile" aria-label="Company details">
          <dl>
            <dt>Industries</dt><dd><IndustrySummary value={c.industrySelections} legacy={c.industry} /></dd>
            <dt>Size</dt><dd>{c.size ? COMPANY_SIZE[c.size] : "—"}</dd>
            <dt>Registration no.</dt><dd>{c.registrationNumber ?? "—"}</dd>
            <dt>Website</dt><dd>{c.website ? <a href={c.website} target="_blank" rel="noreferrer">{c.website.replace(/^https?:\/\/(www\.)?/, "")}</a> : "—"}</dd>
            <dt>Address</dt><dd className="pre">{c.address ?? "—"}</dd>
            <dt>About</dt><dd className="pre">{c.about ?? "—"}</dd>
          </dl>
        </section>
      )}
      {canEdit && !editing && <CompanyAutofill companyId={c.id} website={c.website} />}
    </> },
    { id: "terms", label: "Terms & benefits", content: <>
      <CompanyTerms companyId={c.id} permissions={permissions} />
      <CompanyBenefits companyId={c.id} companyName={c.name} permissions={permissions} />
    </> },
    { id: "jobs", label: "Jobs", content: <CompanyJobs company={c} canCreate={permissions.includes("jobs:create")} /> },
    { id: "contacts", label: "Contacts", content:
      <CompanyContacts company={c} canCreate={permissions.includes("contacts:create")} canEmail={permissions.includes("emails:send")} /> },
    { id: "activity", label: "Activity", content: <Timeline compact subjectType="company" subjectId={c.id} permissions={permissions} /> },
  ];

  return (
    <main className="roomy">
      <a href="#/companies">← Companies</a>
      {company.error && (
        <ErrorMessage error={company.error}>
          <p className="muted">Showing the version loaded earlier.</p>
        </ErrorMessage>
      )}
      {canEdit && !editing && (
        <div className="row spread"><span /><button type="button" onClick={() => { setEditing(true); setActive("details"); }}>Edit company</button></div>
      )}
      <CompanyBanner company={c} canEdit={canEdit} permissions={permissions} />
      <div className="candidate-workspace">
        <aside className="candidate-contact panel" aria-label="Company contact">
          <h3>Contact</h3>
          <dl>
            <dt>Phone</dt><dd>{c.phone ? <a href={`tel:${c.phone}`}>{c.phone}</a> : "—"}</dd>
            <dt>Website</dt><dd>{c.website ? <a href={c.website} target="_blank" rel="noreferrer">{c.website.replace(/^https?:\/\/(www\.)?/, "")}</a> : "—"}</dd>
            <dt>LinkedIn</dt><dd>{c.linkedinUrl ? <a href={c.linkedinUrl} target="_blank" rel="noreferrer">LinkedIn page</a> : "—"}</dd>
            <dt>Address</dt><dd className="pre">{c.address ?? "—"}</dd>
          </dl>
        </aside>
        <SectionTabs name="company" label="Company sections" sections={sections} active={active} onChange={setActive} />
      </div>
    </main>
  );
}
