import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, type Schemas, unwrap } from "../api/client";
import { companiesKey } from "../api/keys";
import { CompanyForm } from "../components/CompanyForm";
import { chipStyle, UNKNOWN_CATEGORY } from "../components/companyCategory";
import { ErrorMessage } from "../components/ErrorMessage";
import { useFieldSettings } from "../components/useFieldSettings";
import { COMPANY_TYPE, keysOf } from "../labels";
import { hashParam } from "../hashQuery";
import { PinnedScroll } from "../components/StickyScrollbar";

type TypeFilter = "" | Schemas["CompanyType"];

/** Search, filter and add companies. */
export function CompanyList({ canCreate }: { canCreate: boolean }) {
  const [q, setQ] = useState(() => hashParam("q")); // a search from the dashboard
  const [type, setType] = useState<TypeFilter>("");
  const [category, setCategory] = useState("");
  const fs = useFieldSettings();
  const [adding, setAdding] = useState(false);
  const list = useQuery({
    queryKey: [...companiesKey, q, type, category],
    queryFn: async () => {
      const query: { q?: string; type?: Schemas["CompanyType"]; category?: string } = {};
      if (q.trim()) query.q = q.trim();
      if (type) query.type = type;
      if (category) query.category = category;
      return unwrap(await api.GET("/v1/companies", { params: { query } }));
    },
  });

  return (
    <main>
      <div className="row spread">
        <h2>Companies</h2>
        <div className="row">
          <input aria-label="Search companies" placeholder="Search" value={q} onChange={(e) => { setQ(e.target.value); }} />
          <select aria-label="Type" value={type} onChange={(e) => { setType(e.target.value as TypeFilter); }}>
            <option value="">All types</option>
            {keysOf(COMPANY_TYPE).map((t) => <option key={t} value={t}>{COMPANY_TYPE[t]}</option>)}
          </select>
          <select aria-label="Category" value={category} onChange={(e) => { setCategory(e.target.value); }}>
            <option value="">All categories</option>
            {fs.options("company_category").filter((o) => o.key !== UNKNOWN_CATEGORY).map((o) => <option key={o.key} value={o.key}>{o.label}</option>)}
          </select>
          {canCreate && !adding && <button type="button" onClick={() => { setAdding(true); }}>Add company</button>}
        </div>
      </div>
      {adding && (
        <CompanyForm
          company={null}
          onDone={() => { setAdding(false); }}
          onSaved={(c) => { window.location.hash = `#/companies/${c.id}`; }}
        />
      )}
      <ErrorMessage error={list.error} />
      <PinnedScroll>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Category</th>
            <th>Type</th>
            <th>Industry</th>
            <th>City</th>
            <th>Website</th>
            <th>LinkedIn</th>
          </tr>
        </thead>
        <tbody>
          {list.data?.items.map((c) => (
            <tr key={c.id}>
              <td><a href={`#/companies/${c.id}`}>{c.name}</a></td>
              <td>
                <span className="category-chip" style={chipStyle(fs.color("company_category", c.category ?? UNKNOWN_CATEGORY))}>
                  {fs.label("company_category", c.category ?? UNKNOWN_CATEGORY)}
                </span>
              </td>
              <td>{COMPANY_TYPE[c.type]}</td>
              <td>{c.industry}</td>
              <td>{c.city}</td>
              <td>{c.website && <a href={c.website} target="_blank" rel="noreferrer">{c.website.replace(/^https?:\/\//, "")}</a>}</td>
              <td>{c.linkedinUrl && <a href={c.linkedinUrl} target="_blank" rel="noreferrer" aria-label={`${c.name} on LinkedIn`}>{c.linkedinUrl.replace(/^https:\/\/www\.linkedin\.com\/(company\/)?/, "")}</a>}</td>
            </tr>
          ))}
          {list.data?.items.length === 0 && (
            <tr><td colSpan={7} className="muted">No companies yet{q || type || category ? " match" : ""}.</td></tr>
          )}
        </tbody>
      </table>
      </PinnedScroll>
    </main>
  );
}
