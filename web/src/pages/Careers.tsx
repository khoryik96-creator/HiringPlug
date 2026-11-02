import { useMutation, useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { AdText } from "../components/AdText";
import { ErrorMessage } from "../components/ErrorMessage";
import { SaveError } from "../components/SaveError";
import { WORK_ARRANGEMENT, WORK_TYPE } from "../labels";

type Ad = Schemas["PublicJobAd"];

const meta = (ad: Ad) =>
  [ad.location, ad.workType && WORK_TYPE[ad.workType], ad.workArrangement && WORK_ARRANGEMENT[ad.workArrangement], ad.salaryText]
    .filter(Boolean)
    .join(" · ");

/**
 * A company's public careers page (signed out): its open ads, one ad with the
 * apply form. Reached at #/careers/<address> and #/careers/<address>/<ad>.
 */
export function Careers({ slug, adId }: { slug: string; adId: string | undefined }) {
  const page = useQuery({
    queryKey: ["careers", slug],
    queryFn: async () => unwrap(await api.GET("/v1/public/careers/{slug}", { params: { path: { slug } } })),
  });
  if (page.error) return <main className="careers"><ErrorMessage error={page.error} /></main>;
  if (!page.data) return <main className="careers"><p className="muted">Loading…</p></main>;
  const ad = adId ? page.data.ads.find((a) => a.id === adId) : undefined;
  return (
    <main className="careers">
      <header>
        <h1><a href={`#/careers/${slug}`}>{page.data.companyName}</a></h1>
        <p className="muted">Careers</p>
      </header>
      {adId ? (
        ad ? <AdPage slug={slug} ad={ad} company={page.data.companyName} /> : <p>This job is no longer open. <a href={`#/careers/${slug}`}>See open jobs</a></p>
      ) : page.data.ads.length === 0 ? (
        <p>There are no open jobs right now. Please check back later.</p>
      ) : (
        <ul className="careers-list">
          {page.data.ads.map((a) => (
            <li key={a.id}>
              <a href={`#/careers/${slug}/${a.id}`}><strong>{a.title}</strong></a>
              <div className="muted">{meta(a)}</div>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}

function AdPage({ slug, ad, company }: { slug: string; ad: Ad; company: string }) {
  return (
    <article aria-label={ad.title}>
      <a href={`#/careers/${slug}`}>← All jobs</a>
      <h2>{ad.title}</h2>
      <p className="muted">{meta(ad)}{ad.closesOn && ` · apply by ${ad.closesOn}`}</p>
      <AdText text={ad.description} />
      <ApplyForm slug={slug} adId={ad.id} company={company} />
    </article>
  );
}

function ApplyForm({ slug, adId, company }: { slug: string; adId: string; company: string }) {
  const [form, setForm] = useState({ firstName: "", lastName: "", email: "", phone: "", consent: false, website: "" });
  const [file, setFile] = useState<File | null>(null);
  const apply = useMutation({
    mutationFn: async () => {
      const body = new FormData();
      body.append("firstName", form.firstName);
      body.append("lastName", form.lastName);
      body.append("email", form.email);
      body.append("phone", form.phone);
      if (form.consent) body.append("consent", "yes");
      body.append("website", form.website);
      if (file) body.append("file", file);
      const res = await fetch(`/v1/public/careers/${slug}/ads/${adId}/applications`, { method: "POST", body });
      if (!res.ok) unwrap({ error: await res.json(), response: res }); // throws the API's error
    },
  });
  if (apply.isSuccess) {
    return <p className="panel" role="status">Thank you. Your application has been received.</p>;
  }
  const text = (name: "firstName" | "lastName" | "email" | "phone", label: string, type = "text") => (
    <label className="field">{label}
      <input type={type} value={form[name]} autoComplete={name === "phone" ? "tel" : name === "email" ? "email" : name === "firstName" ? "given-name" : "family-name"}
        onChange={(e) => { setForm({ ...form, [name]: e.target.value }); }} />
    </label>
  );
  return (
    <form className="panel form-grid" aria-label="Apply" onSubmit={(e) => { e.preventDefault(); apply.mutate(); }}>
      <h3 className="full">Apply</h3>
      {text("firstName", "First name")}
      {text("lastName", "Last name")}
      {text("email", "Email", "email")}
      {text("phone", "Phone", "tel")}
      <label className="field full">CV (PDF or Word)
        <input type="file" accept=".pdf,.docx,.doc" onChange={(e) => { setFile(e.target.files?.[0] ?? null); }} />
      </label>
      {/* A trap for robots: hidden from people (and screen readers), so it stays empty. */}
      <label className="trap" aria-hidden="true">Website
        <input tabIndex={-1} autoComplete="off" value={form.website} onChange={(e) => { setForm({ ...form, website: e.target.value }); }} />
      </label>
      <label className="row full">
        <input type="checkbox" checked={form.consent} onChange={(e) => { setForm({ ...form, consent: e.target.checked }); }} />{" "}
        I agree that {company} keeps my details to handle my application.
      </label>
      <div className="full">
        <button type="submit" disabled={apply.isPending || !file || !form.consent}>Send application</button>
      </div>
      <SaveError error={apply.error} shown={[]} onReload={() => { apply.reset(); }} />
    </form>
  );
}
