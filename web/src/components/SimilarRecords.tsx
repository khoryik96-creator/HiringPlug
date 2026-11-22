import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { api, unwrap } from "../api/client";
import { companyKey, contactKey } from "../api/keys";
import { ErrorMessage } from "./ErrorMessage";

/** Why two records may be one, in words. */
export const DUPLICATE_REASON: Record<string, string> = {
  same_website: "same website",
  same_linkedin: "same LinkedIn",
  similar_name: "similar name",
  same_name: "same name",
  same_phone: "same mobile",
};
export const reasonsText = (reasons: readonly string[]) => reasons.map((r) => DUPLICATE_REASON[r] ?? r).join(", ");

type Lookalike = { id: string; name: string; href: string; detail: string; reasons: string[]; cannot?: string };

/**
 * A warning listing a record's possible duplicates (nothing if none). With
 * the merge permission, one can be merged into this record.
 */
function DuplicateLine({ keepName, items, canMerge, moves, merge }: {
  keepName: string; items: Lookalike[]; canMerge: boolean; moves: string;
  merge: { mutate: (id: string) => void; isPending: boolean; error: unknown };
}) {
  const [open, setOpen] = useState(false);
  if (items.length === 0) return null;
  const n = items.length;
  const why = [...new Set(items.flatMap((s) => s.reasons))].map((r) => DUPLICATE_REASON[r] ?? r).join(" or ");
  return (
    <section className="dupe-line" aria-label="Possible duplicates">
      <div className="dupe-head">
        <span className="dupe-icon" aria-hidden="true">!</span>
        <span className="dupe-text"><strong>{n === 1 ? "1 possible duplicate" : `${String(n)} possible duplicates`}</strong> <span className="muted">· {why}</span></span>
        <button type="button" className="small-button" aria-expanded={open} onClick={() => { setOpen(!open); }}>
          {open ? "Hide" : canMerge ? "Review and merge" : "Review"}
        </button>
      </div>
      {open && <ul>
        {items.map((s) => (
          <li key={s.id}>
            <a href={s.href}>{s.name}</a>{" "}
            <span className="muted">({reasonsText(s.reasons)}{s.detail ? `; ${s.detail}` : ""})</span>{" "}
            {canMerge && (s.cannot ? <span className="muted small">· {s.cannot}</span> : (
              <button type="button" className="link" disabled={merge.isPending}
                onClick={() => {
                  if (window.confirm(`Merge ${s.name} into ${keepName}? ${moves} Empty fields here are filled from theirs, and ${s.name} is deleted.`)) {
                    merge.mutate(s.id);
                  }
                }}>
                Merge into this one
              </button>
            ))}
          </li>
        ))}
      </ul>}
      <ErrorMessage error={merge.error} />
    </section>
  );
}

/** A company's possible duplicates (companies:merge merges one in). */
export function SimilarCompanies({ company, canMerge }: { company: { id: string; name: string; version: number }; canMerge: boolean }) {
  const queryClient = useQueryClient();
  const similar = useQuery({
    queryKey: ["company", company.id, "similar"],
    queryFn: async () => unwrap(await api.GET("/v1/companies/{companyId}/similar", { params: { path: { companyId: company.id } } })).items,
  });
  const merge = useMutation({
    mutationFn: async (duplicateId: string) => unwrap(await api.POST("/v1/companies/{companyId}/merge", {
      params: { path: { companyId: company.id } }, body: { duplicateId, version: company.version },
    })),
    onSuccess: (merged) => {
      queryClient.setQueryData(companyKey(company.id), merged);
      void queryClient.invalidateQueries(); // contacts, jobs, notes and lists all changed
    },
  });
  const items = (similar.data ?? []).map((s) => ({
    id: s.id, name: s.name, href: `#/companies/${s.id}`, reasons: s.reasons,
    detail: [s.website, s.city].filter(Boolean).join(", "),
  }));
  return <DuplicateLine keepName={company.name} items={items} canMerge={canMerge} merge={merge}
    moves="Their contacts, jobs, notes, documents and leads move here (their terms and benefits too, if this company has none)." />;
}

/** A contact's possible duplicates (contacts:merge merges one at the same company in). */
export function SimilarContacts({ contact, canMerge }: { contact: { id: string; companyId: string; firstName: string; lastName: string; version: number }; canMerge: boolean }) {
  const queryClient = useQueryClient();
  const similar = useQuery({
    queryKey: ["contact", contact.id, "similar"],
    queryFn: async () => unwrap(await api.GET("/v1/contacts/{contactId}/similar", { params: { path: { contactId: contact.id } } })).items,
  });
  const merge = useMutation({
    mutationFn: async (duplicateId: string) => unwrap(await api.POST("/v1/contacts/{contactId}/merge", {
      params: { path: { contactId: contact.id } }, body: { duplicateId, version: contact.version },
    })),
    onSuccess: (merged) => {
      queryClient.setQueryData(contactKey(contact.id), merged);
      void queryClient.invalidateQueries();
    },
  });
  const items = (similar.data ?? []).map((s) => ({
    id: s.id, name: s.fullName, href: `#/contacts/${s.id}`, reasons: s.reasons,
    detail: [s.title, s.companyName, s.email].filter(Boolean).join(", "),
    cannot: s.companyId === contact.companyId ? undefined : "at another company (they may have moved)",
  }));
  return <DuplicateLine keepName={`${contact.firstName} ${contact.lastName}`.trim()} items={items} canMerge={canMerge} merge={merge}
    moves="Their emails, jobs, invoices and notes move here." />;
}
