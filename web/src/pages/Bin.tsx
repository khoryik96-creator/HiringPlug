// Settings › Bin (admins): what was deleted, by whom, when it is deleted for
// good, and Restore.
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api, unwrap } from "../api/client";
import { ErrorMessage } from "../components/ErrorMessage";

const KIND = { candidate: "Candidate", contact: "Contact", company: "Company", document: "CV" } as const;
const day = new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", year: "numeric" });

export function Bin() {
  const queryClient = useQueryClient();
  const bin = useQuery({ queryKey: ["bin"], queryFn: async () => unwrap(await api.GET("/v1/bin")).items });
  const restore = useMutation({
    mutationFn: async (entryId: string) => { unwrap(await api.POST("/v1/bin/{entryId}/restore", { params: { path: { entryId } } })); },
    onSuccess: async () => { await queryClient.invalidateQueries(); },
  });
  const items = bin.data ?? [];
  return (
    <main>
      <h2>Bin</h2>
      <p className="muted">Deleted candidates, contacts, companies and replaced CVs wait here 30 days, then are deleted for good.</p>
      <ErrorMessage error={bin.error ?? restore.error} />
      {bin.data && items.length === 0 && <p className="muted">The Bin is empty.</p>}
      {items.length > 0 && (
        <table className="bin-table">
          <thead><tr><th>Deleted</th><th>What</th><th>By</th><th>Deleted for good</th><th><span className="visually-hidden">Restore</span></th></tr></thead>
          <tbody>
            {items.map((e) => (
              <tr key={e.id}>
                <td className="nowrap">{day.format(new Date(e.binnedAt))}</td>
                <td>
                  <strong>{e.label}</strong> <span className="muted">{KIND[e.kind]}</span>
                  {(e.contacts > 0 || e.jobs > 0) && <div className="muted small">with {[e.contacts > 0 && `${String(e.contacts)} contact${e.contacts === 1 ? "" : "s"}`, e.jobs > 0 && `${String(e.jobs)} job${e.jobs === 1 ? "" : "s"}`].filter(Boolean).join(" and ")}</div>}
                </td>
                <td>{e.binnedByName}</td>
                <td className="nowrap muted">{day.format(new Date(e.purgeAfter))}</td>
                <td><button type="button" className="doc-btn" disabled={restore.isPending} onClick={() => { restore.mutate(e.id); }}>Restore</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}
