// Companies › Possible duplicates and Contacts › Possible duplicates: groups
// of records that look like one company or person. Open the one to keep and
// merge the others into it there.
import { useQuery } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";
import { reasonsText } from "../components/SimilarRecords";

type Row = { id: string; name: string; detail: string };
type Group = { reasons: string[]; rows: Row[] };

export function Duplicates({ kind }: { kind: "companies" | "contacts" }) {
  const groups = useQuery({
    queryKey: [kind, "duplicates"],
    queryFn: async (): Promise<Group[]> => {
      if (kind === "companies") {
        const res = unwrap(await api.GET("/v1/companies/duplicates"));
        return res.items.map((g) => ({ reasons: g.reasons, rows: g.companies.map((c) => ({ id: c.id, name: c.name, detail: [c.website, c.city, c.industry].filter(Boolean).join(" · ") })) }));
      }
      const res = unwrap(await api.GET("/v1/contacts/duplicates"));
      return res.items.map((g) => ({ reasons: g.reasons, rows: g.contacts.map((c) => ({ id: c.id, name: c.fullName, detail: [c.title, c.companyName, c.email].filter(Boolean).join(" · ") })) }));
    },
  });
  const noun = kind === "companies" ? "companies" : "contacts";
  return (
    <main className="roomy">
      <p><a href={`#/${kind}`}>← {kind === "companies" ? "Companies" : "Contacts"}</a></p>
      <h2>Possible duplicates</h2>
      <p className="muted">
        {kind === "companies"
          ? "Companies with the same website or LinkedIn page, or the same name without Sdn Bhd, Berhad and the like."
          : "Contacts with the same LinkedIn profile, or the same name (in any order, titles aside) at the same company."}
        {" "}Open the one to keep, then choose <strong>Merge into this one</strong> beside the others.
      </p>
      <ErrorMessage error={groups.error} />
      {groups.isPending && <p className="muted">Looking…</p>}
      {groups.data?.length === 0 && <p className="muted">No possible duplicates among your {noun}.</p>}
      {groups.data && groups.data.length > 0 && (
        <ul className="dupe-groups">
          {groups.data.map((g) => (
            <li key={g.rows.map((r) => r.id).join(",")} className="panel dupe-group">
              <span className="dupe-why">{reasonsText(g.reasons)}</span>
              <ul>
                {g.rows.map((r) => (
                  <li key={r.id}><a href={`#/${kind}/${r.id}`}>{r.name}</a>{r.detail && <span className="muted"> · {r.detail}</span>}</li>
                ))}
              </ul>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
