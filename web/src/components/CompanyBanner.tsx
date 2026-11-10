// The company page's header, like a candidate's card: a banner in the
// colour of the company's category (Settings › Company categories), the
// category down its right edge, picked here and saved straight away.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ApiError, api, type Schemas, unwrap } from "../api/client";
import { companiesKey, companyKey, jobsKey } from "../api/keys";
import { COMPANY_SIZE, COMPANY_TYPE, countryName } from "../labels";
import { companyTermsKey } from "./CompanyTerms";
import { tint, UNKNOWN_CATEGORY } from "./companyCategory";
import { ErrorMessage } from "./ErrorMessage";
import { bySalary, guaranteeLabel } from "./termsTiers";
import { useFieldSettings } from "./useFieldSettings";

type Company = Schemas["Company"];

/** "18%", or "18–25%" when the fee depends on the salary. */
function feeText(tiers: Schemas["CompanyTermsTier"][]): string | null {
  const fees = tiers.map((t) => Number(t.feePercent)).filter((n) => n > 0);
  if (fees.length === 0) return null;
  const [lo, hi] = [Math.min(...fees), Math.max(...fees)];
  return lo === hi ? `${String(lo)}%` : `${String(lo)}–${String(hi)}%`;
}

const range = (values: (number | null)[], words: (n: number) => string): string | null => {
  const ns = values.filter((n): n is number => n !== null);
  if (ns.length === 0) return null;
  const [lo, hi] = [Math.min(...ns), Math.max(...ns)];
  return lo === hi ? words(lo) : `${words(lo)} – ${words(hi)}`;
};

export function CompanyBanner({ company: c, canEdit }: { company: Company; canEdit: boolean }) {
  const queryClient = useQueryClient();
  const fs = useFieldSettings();
  const terms = useQuery({
    queryKey: companyTermsKey(c.id),
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/terms", { params: { path: { companyId: c.id } } })),
  });
  const jobs = useQuery({
    queryKey: [...jobsKey, "company", c.id],
    // The same list as the page's Jobs section (one request for both).
    queryFn: async () => unwrap(await api.GET("/v1/jobs", { params: { query: { companyId: c.id, limit: 100 } } })).items,
  });
  const save = useMutation({
    mutationFn: async (category: string | null) =>
      unwrap(await api.PATCH("/v1/companies/{companyId}", { params: { path: { companyId: c.id } }, body: { version: c.version, category } })),
    onSuccess: (updated) => {
      queryClient.setQueryData(companyKey(c.id), updated);
      void queryClient.invalidateQueries({ queryKey: companiesKey });
    },
    onError: (e) => {
      if (e instanceof ApiError && e.code === "CONFLICT_STALE_VERSION") void queryClient.invalidateQueries({ queryKey: companyKey(c.id) });
    },
  });

  const category = c.category ?? UNKNOWN_CATEGORY;
  const color = fs.color("company_category", category);
  const place = [c.city, c.country ? countryName(c.country) : null].filter(Boolean).join(", ");
  const tiers = terms.data?.tiers ?? [];
  const open = jobs.data?.filter((j) => j.status === "open").length;

  return (
    <section className="banner company-banner" aria-label="Company" style={{ background: `linear-gradient(120deg, ${tint(color, 0.72)}, ${tint(color, 0.9)})` }}>
      <div className="banner-body">
        <div className="banner-top">
          <div className="banner-who">
            <div className="row">
              <h2 className="banner-name">{c.name}</h2>
              <span className="banner-status" style={{ color: color === "#6b7280" ? undefined : color }}>{fs.label("company_category", category)}</span>
            </div>
            <p className="banner-job">{[COMPANY_TYPE[c.type], c.industry].filter(Boolean).join(" · ")}</p>
          </div>
          <dl className="banner-facts">
            <div><dt>Based in</dt><dd>{place || "—"}</dd></div>
            <div><dt>Size</dt><dd>{c.size ? COMPANY_SIZE[c.size] : "—"}</dd></div>
            {c.linkedinUrl && <div><dt>LinkedIn</dt><dd><a href={c.linkedinUrl} target="_blank" rel="noreferrer">{c.linkedinUrl.replace(/^https:\/\/www\.linkedin\.com\//, "")}</a></dd></div>}
            <div><dt>Website</dt><dd>{c.website ? <a href={c.website} target="_blank" rel="noreferrer">{c.website.replace(/^https?:\/\/(www\.)?/, "")}</a> : "—"}</dd></div>
          </dl>
        </div>

        {canEdit && (
          <div className="row" role="group" aria-label="Category">
            {fs.options("company_category", c.category).map((o) => (
              <button key={o.key} type="button" className="banner-pill" aria-pressed={category === o.key} disabled={save.isPending}
                onClick={() => { save.mutate(o.key === UNKNOWN_CATEGORY ? null : o.key); }}>
                <span className="dot" style={{ background: fs.color("company_category", o.key), borderColor: fs.color("company_category", o.key) }} />
                {o.label}
              </button>
            ))}
            <a className="small" href="#/settings#company-categories">Change the categories</a>
          </div>
        )}
        <ErrorMessage error={save.error} />

        <div className="banner-strip">
          <div>
            <span className="label">Fee</span>
            <span className="figure">{feeText(tiers) ?? "—"}</span>
            {bySalary(tiers) && <span className="note">by salary band</span>}
          </div>
          <div>
            <span className="label">Guarantee · payment</span>
            <span className="figure small">
              {[range(tiers.map((t) => t.guaranteeDays), guaranteeLabel), range(tiers.map((t) => t.paymentTermsDays), (n) => `${String(n)} days`)]
                .filter(Boolean).join(" · ") || "—"}
            </span>
          </div>
          <div>
            <span className="label">Open jobs</span>
            <span className="figure">{open ?? "—"}</span>
            {jobs.data && <span className="note">{jobs.data.length} in all</span>}
          </div>
        </div>
      </div>
      <div className="banner-column" style={{ background: color }}><span>{fs.label("company_category", category)}</span></div>
    </section>
  );
}
