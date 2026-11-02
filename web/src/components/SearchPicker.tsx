import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

export interface Picked {
  id: string;
  name: string;
}

/**
 * Choose a record by typing part of its name: shows up to `search` results as
 * buttons, then the choice with a "Change" button. `id` labels the search
 * input (for Field). `queryKey` identifies the search in the cache.
 */
export function SearchPicker({ id, value, onChange, search, queryKey, placeholder, noMatch, ...aria }: {
  id?: string;
  value: Picked | null;
  onChange: (picked: Picked | null) => void;
  search: (text: string) => Promise<(Picked & { detail?: string | null })[]>;
  queryKey: readonly unknown[];
  placeholder: string;
  noMatch: string;
  "aria-invalid"?: boolean;
  "aria-describedby"?: string;
}) {
  const [q, setQ] = useState("");
  const text = q.trim();
  const results = useQuery({
    queryKey: [...queryKey, "picker", text],
    queryFn: () => search(text),
    enabled: value === null && text !== "",
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
      <input id={id} {...aria} placeholder={placeholder} value={q} onChange={(e) => { setQ(e.target.value); }} />
      {text !== "" && (
        <div className="picker-results">
          {results.data?.map((r) => (
            <button key={r.id} type="button" className="link" onClick={() => { setQ(""); onChange({ id: r.id, name: r.name }); }}>
              {r.name}{r.detail ? <span className="muted"> · {r.detail}</span> : null}
            </button>
          ))}
          {results.data?.length === 0 && <span className="muted">{noMatch}</span>}
        </div>
      )}
    </div>
  );
}
