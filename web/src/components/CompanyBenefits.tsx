// A client's benefits and its steps before an offer, recorded from the
// agency's template (Hyppies' Benefits & Pre-Offer Process to start with,
// changed in Settings). Read at a glance; filled in as one form, item by
// item, with what to write under each; downloaded as Word or PDF.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Permission, unwrap } from "../api/client";
import { downloadFrom } from "../api/files";
import { companyBenefitsKey, fieldSettingsKey } from "../api/keys";
import { formatWhen } from "../labels";
import { type Benefit, blankBenefit, blankStep, filled, type Row, rowsFor, type Step } from "./benefitRows";
import { CompanyDocuments } from "./CompanyDocuments";
import { ErrorMessage } from "./ErrorMessage";

const TYPES = { docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", pdf: "application/pdf" } as const;
const APPLIES: { value: boolean | null; label: string }[] = [{ value: true, label: "Yes" }, { value: false, label: "No" }, { value: null, label: "Not known" }];
const appliesLabel = (a: boolean | null) => APPLIES.find((x) => x.value === a)?.label ?? "Not known";
const textOrNull = (s: string) => (s.trim() === "" ? null : s);

export function CompanyBenefits({ companyId, companyName, permissions }: { companyId: string; companyName: string; permissions: Permission[] }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [downloadError, setDownloadError] = useState<unknown>(null);
  const saved = useQuery({
    queryKey: companyBenefitsKey(companyId),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/benefits", { params: { path: { companyId } } })),
  });
  const settings = useQuery({ queryKey: fieldSettingsKey, queryFn: async () => unwrap(await api.GET("/v1/settings/fields")), staleTime: 60_000 });
  const list = (name: "company_benefits" | "pre_offer_process") => settings.data?.lists.find((l) => l.name === name);
  const data = saved.data;
  if (!data || !settings.data) {
    return <section className="panel" aria-label="Benefits"><h3>Benefits &amp; pre-offer process</h3><ErrorMessage error={saved.error ?? settings.error} /></section>;
  }
  const benefits = rowsFor(list("company_benefits"), data.benefits, blankBenefit);
  const steps = rowsFor(list("pre_offer_process"), data.preOffer, blankStep);
  const canEdit = permissions.includes("companies:update");
  const done = benefits.filter((r) => filled(r.item)).length;
  const get = (format: keyof typeof TYPES) => {
    setDownloadError(null);
    downloadFrom(`/v1/companies/${companyId}/benefits/file?format=${format}`, `${companyName} - Benefits.${format}`, TYPES[format])
      .catch((e: unknown) => { setDownloadError(e); });
  };

  return (
    <section className="panel benefits" aria-label="Benefits">
      <div className="row spread">
        <div>
          <h3>Benefits &amp; pre-offer process</h3>
          <p className="muted small">
            {data.version === 0 ? "Nothing recorded yet." : `${String(done)} of ${String(benefits.length)} benefits recorded · saved ${data.updatedAt ? formatWhen(data.updatedAt) : ""}`}
          </p>
        </div>
        {!editing && (
          <div className="row">
            {data.version > 0 && <><button type="button" className="link" onClick={() => { get("docx"); }}>Word</button>
              <button type="button" className="link" onClick={() => { get("pdf"); }}>PDF</button></>}
            {permissions.includes("settings:manage") && <a className="small" href="#/settings">Change the template</a>}
            {canEdit && <button type="button" onClick={() => { setEditing(true); }}>{data.version === 0 ? "Fill in" : "Edit"}</button>}
          </div>
        )}
      </div>
      <ErrorMessage error={downloadError} />
      {editing ? (
        <BenefitsForm companyId={companyId} version={data.version} benefits={benefits} steps={steps}
          onDone={() => { setEditing(false); void queryClient.invalidateQueries({ queryKey: companyBenefitsKey(companyId) }); }} />
      ) : (
        <BenefitsView benefits={benefits} steps={steps} empty={data.version === 0} />
      )}
      <CompanyDocuments companyId={companyId} section="benefits" permissions={permissions} />
    </section>
  );
}

function BenefitsView({ benefits, steps, empty }: { benefits: Row<Benefit>[]; steps: Row<Step>[]; empty: boolean }) {
  if (empty) return <p className="muted">Record this client's leave, medical, bonuses and more, and what it checks before an offer.</p>;
  const missing = benefits.filter((r) => !filled(r.item));
  return (
    <div className="benefits-columns">
      <div>
        <h4>Benefits</h4>
        <dl className="benefit-list">
          {benefits.filter((r) => filled(r.item)).map(({ item, retired }) => (
            <div key={item.key}>
              <dt>{item.label}{retired && <span className="muted"> (not in the template)</span>}</dt>
              <dd>
                {item.value && <span className="benefit-value pre">{item.value}</span>}
                {item.remarks && <span className="benefit-remarks pre">{item.remarks}</span>}
              </dd>
            </div>
          ))}
        </dl>
        {missing.length > 0 && <p className="muted small">Not recorded yet: {missing.map((r) => r.item.label).join(" · ")}</p>}
      </div>
      <div>
        <h4>Pre-offer process</h4>
        <ul className="pre-offer-list">
          {steps.map(({ item }) => (
            <li key={item.key}>
              <span className={`applies ${item.applies === true ? "yes" : item.applies === false ? "no" : "unknown"}`}>{appliesLabel(item.applies)}</span>
              <span>
                <strong>{item.label}</strong>
                {item.remarks && <span className="benefit-remarks pre">{item.remarks}</span>}
              </span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function BenefitsForm({ companyId, version, benefits, steps, onDone }: {
  companyId: string; version: number; benefits: Row<Benefit>[]; steps: Row<Step>[]; onDone: () => void;
}) {
  const [b, setB] = useState(benefits.map((r) => ({ ...r.item, value: r.item.value ?? "", remarks: r.item.remarks ?? "" })));
  const [s, setS] = useState(steps.map((r) => ({ ...r.item, remarks: r.item.remarks ?? "" })));
  const save = useMutation({
    mutationFn: async () => unwrap(await api.PUT("/v1/companies/{companyId}/benefits", {
      params: { path: { companyId } },
      body: {
        version,
        benefits: b.map((x) => ({ key: x.key, label: x.label, value: textOrNull(x.value), remarks: textOrNull(x.remarks) })),
        preOffer: s.map((x) => ({ key: x.key, label: x.label, applies: x.applies, remarks: textOrNull(x.remarks) })),
      },
    })),
    onSuccess: onDone,
  });
  const setBenefit = (i: number, change: Partial<(typeof b)[number]>) => { setB(b.map((x, j) => (j === i ? { ...x, ...change } : x))); };
  const setStep = (i: number, change: Partial<(typeof s)[number]>) => { setS(s.map((x, j) => (j === i ? { ...x, ...change } : x))); };
  return (
    <form aria-label="Benefits form" onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <h4>Benefits</h4>
      <div className="benefit-grid" role="group" aria-label="Benefits">
        <span className="benefit-head">Item</span><span className="benefit-head">Value</span><span className="benefit-head">Remarks or clauses</span>
        {b.map((x, i) => (
          <BenefitLine key={x.key} label={x.label} hint={benefits[i]?.hint ?? ""}>
            <input aria-label={`${x.label}: value`} value={x.value} maxLength={1000} onChange={(e) => { setBenefit(i, { value: e.target.value }); }} />
            <textarea aria-label={`${x.label}: remarks`} rows={1} value={x.remarks} maxLength={2000} onChange={(e) => { setBenefit(i, { remarks: e.target.value }); }} />
          </BenefitLine>
        ))}
      </div>
      <h4>Pre-offer process</h4>
      <div className="benefit-grid" role="group" aria-label="Pre-offer process">
        <span className="benefit-head">Item</span><span className="benefit-head">Applies</span><span className="benefit-head">Remarks</span>
        {s.map((x, i) => (
          <BenefitLine key={x.key} label={x.label} hint={steps[i]?.hint ?? ""}>
            <span className="segmented" role="radiogroup" aria-label={`${x.label}: applies`}>
              {APPLIES.map((o) => (
                <button key={o.label} type="button" role="radio" aria-checked={x.applies === o.value} className={x.applies === o.value ? "on" : undefined}
                  onClick={() => { setStep(i, { applies: o.value }); }}>{o.label}</button>
              ))}
            </span>
            <textarea aria-label={`${x.label}: remarks`} rows={1} value={x.remarks} maxLength={2000} onChange={(e) => { setStep(i, { remarks: e.target.value }); }} />
          </BenefitLine>
        ))}
      </div>
      <div className="row">
        <button type="submit" disabled={save.isPending}>Save</button>
        <button type="button" className="link" onClick={onDone}>Cancel</button>
      </div>
      <ErrorMessage error={save.error} />
    </form>
  );
}

function BenefitLine({ label, hint, children }: { label: string; hint: string; children: React.ReactNode }) {
  return (
    <>
      <span className="benefit-item">{label}{hint && <small className="muted">{hint}</small>}</span>
      {children}
    </>
  );
}
