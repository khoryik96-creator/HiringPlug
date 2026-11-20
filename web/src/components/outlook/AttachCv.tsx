// A candidate's CV attached to an Outlook message: chosen from the CVs on
// their record, fetched and added to the message's attachments.
import { useMutation, useQuery } from "@tanstack/react-query";
import { useRef } from "react";

import { api, unwrap } from "../../api/client";
import { getToken } from "../../auth";
import { ErrorMessage } from "../ErrorMessage";

export function AttachCv({ candidateId, name, onAttach }: { candidateId: string; name: string; onAttach: (file: File) => void }) {
  const docs = useQuery({
    queryKey: ["documents", candidateId],
    queryFn: async () => unwrap(await api.GET("/v1/candidates/{candidateId}/documents", { params: { path: { candidateId } } })).items,
  });
  const select = useRef<HTMLSelectElement>(null);
  const fetchFile = useMutation({
    mutationFn: async (id: string) => {
      const doc = docs.data?.find((d) => d.id === id);
      const res = await fetch(`/v1/documents/${id}/file`, { headers: { Authorization: `Bearer ${getToken() ?? ""}` } });
      if (!res.ok || !doc) throw new Error("The CV could not be fetched.");
      return new File([await res.blob()], doc.fileName, { type: doc.contentType });
    },
    onSuccess: (file) => { onAttach(file); if (select.current) select.current.value = ""; },
  });
  if (docs.isPending) return <span className="muted small">…</span>;
  if (!docs.data?.length) return <span className="muted small">No CV on record</span>;
  return <span className="outlook-attach-cv">
    <select ref={select} aria-label={`Attach a CV of ${name}`} defaultValue="" disabled={fetchFile.isPending}
      onChange={(e) => { if (e.target.value) fetchFile.mutate(e.target.value); }}>
      <option value="">{fetchFile.isPending ? "Attaching…" : "📎 Attach CV…"}</option>
      {docs.data.map((d) => <option key={d.id} value={d.id}>{d.fileName}</option>)}
    </select>
    {fetchFile.isSuccess && <span className="muted small" role="status">Attached {fetchFile.data.name}</span>}
    <ErrorMessage error={fetchFile.error} />
  </span>;
}
