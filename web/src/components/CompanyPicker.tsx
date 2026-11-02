import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { companiesKey } from "../api/keys";

export interface PickedCompany {
  id: string;
  name: string;
}

/**
 * Choose a company by typing part of its name. Shows the choice with a
 * "Change" button once picked. `id` labels the search input (for Field).
 */
export function CompanyPicker({ id, value, onChange, ...aria }: {
  id?: string;
  value: PickedCompany | null;
  onChange: (company: PickedCompany | null) => void;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  const [q, setQ] = useState("");
  const search = q.trim();
  const results = useQuery({
    queryKey: [...companiesKey, "picker", search],
    queryFn: async () => unwrap(await api.GET("/v1/companies", { params: { query: { q: search, limit: 8 } } })).items,
    enabled: value === null && search !== "",
  });

  if (value) {
    return (
      <div className="row">
        <span id={id}>{value.name}</span>
        <button type="button" className="link" onClick={() => { onChange(null); setQ(""); }}>Change</button>
      </div>
    );
  }
  return (
    <div>
      <input id={id} {...aria} placeholder="Type to search companies" value={q} onChange={(e) => { setQ(e.target.value); }} />
      {search !== "" && (
        <div className="picker-results">
          {results.data?.map((c) => (
            <button key={c.id} type="button" className="link" onClick={() => { onChange({ id: c.id, name: c.name }); }}>
              {c.name}
            </button>
          ))}
          {results.data?.length === 0 && <span className="muted">No company matches. Add it under Companies first.</span>}
        </div>
      )}
    </div>
  );
}
